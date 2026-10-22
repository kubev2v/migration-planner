package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/kubev2v/migration-planner/internal/config"
	"go.uber.org/zap"
)

const (
	// imageTarName is the well-known filename for the appliance image archive
	// inside the data directory. Both the registry-pull path and a future
	// upload endpoint write to this same path.
	imageTarName = "appliance.tar"

	// modifiedISOName is the filename for the rebuilt ISO.
	modifiedISOName = "modified.iso"
)

// ApplianceImageCache manages a cached copy of the appliance OCI image and the
// modified ISO that results from injecting it into the base RHCOS ISO.
//
// Design note — the rebuild logic is decoupled from the image source:
//
//   - refreshLoop periodically polls the registry, pulls when a new version
//     is detected, and writes the archive to the stable data-dir path.
//   - RebuildFromImage takes any OCI-archive tarball and re-creates the ISO.
//     A future upload endpoint can call this method directly.
type ApplianceImageCache struct {
	cfg            *config.ApplianceImage
	baseISOPath    string
	dataDir        string
	modifiedISO    string
	imageTarPath   string
	currentVersion string
	lastErr        error // last build error (nil while building for the first time)
	hasBuiltOnce   bool  // true after the first successful or failed build attempt
	logger         *zap.SugaredLogger
	mu             sync.RWMutex
	ctx            context.Context
	cancel         context.CancelFunc
	wg             sync.WaitGroup
}

// NewApplianceImageCache creates a new cache instance. Call Start() to begin
// the background refresh loop.
func NewApplianceImageCache(cfg *config.ApplianceImage, baseISOPath string) *ApplianceImageCache {
	return &ApplianceImageCache{
		cfg:          cfg,
		baseISOPath:  baseISOPath,
		dataDir:      cfg.DataDir,
		modifiedISO:  filepath.Join(cfg.DataDir, modifiedISOName),
		imageTarPath: filepath.Join(cfg.DataDir, imageTarName),
		logger:       zap.S().Named("appliance_cache"),
	}
}

// Start launches the background goroutine that periodically checks the
// registry for a newer appliance image.
func (c *ApplianceImageCache) Start() {
	c.ctx, c.cancel = context.WithCancel(context.Background())

	// Ensure the data directory exists.
	if err := os.MkdirAll(c.dataDir, 0o755); err != nil {
		c.logger.Errorw("failed to create appliance data directory", "dir", c.dataDir, "error", err)
	}

	c.wg.Add(1)
	go c.refreshLoop()
}

// Stop cancels the background goroutine and waits for it to finish.
func (c *ApplianceImageCache) Stop() {
	if c.cancel != nil {
		c.cancel()
	}
	c.wg.Wait()
}

// HealthCheck returns nil when the cache is healthy. During the initial
// build (before any attempt has completed) it returns nil — only an
// active build failure is reported as an error. This prevents the liveness
// probe from killing the pod while the first ISO is still being assembled.
func (c *ApplianceImageCache) HealthCheck() error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.hasBuiltOnce {
		// Still building for the first time — not an error.
		return nil
	}
	return c.lastErr
}

// IsReady reports whether a modified ISO file exists and is available for
// serving. The readiness probe should gate on this so that traffic is only
// sent to pods that can actually serve the ISO.
func (c *ApplianceImageCache) IsReady() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if _, err := os.Stat(c.modifiedISO); err != nil {
		return false
	}
	return true
}

// GetModifiedISO returns the path and SHA-256 digest of the current modified
// ISO, or an error if no ISO has been built yet.
func (c *ApplianceImageCache) GetModifiedISO() (path string, digest string, err error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if _, err := os.Stat(c.modifiedISO); err != nil {
		return "", "", fmt.Errorf("modified ISO not available: %w", err)
	}

	d, err := fileSHA256(c.modifiedISO)
	if err != nil {
		return "", "", fmt.Errorf("failed to compute ISO digest: %w", err)
	}
	return c.modifiedISO, d, nil
}

// RebuildFromImage takes the path to an OCI-archive tarball and rebuilds
// the ISO by injecting the appliance image into the base RHCOS ISO.
//
// This is the shared rebuild method — it does not care how the image tar
// arrived at the path (registry pull, upload endpoint, etc.).
func (c *ApplianceImageCache) RebuildFromImage(imageTarPath string) error {
	c.logger.Infow("rebuilding ISO from image", "imageTar", imageTarPath)

	if _, err := os.Stat(c.baseISOPath); err != nil {
		return fmt.Errorf("base ISO not found at %s: %w", c.baseISOPath, err)
	}

	if _, err := os.Stat(imageTarPath); err != nil {
		return fmt.Errorf("image tar not found at %s: %w", imageTarPath, err)
	}

	tmpISO := c.modifiedISO + ".tmp"
	defer os.Remove(tmpISO) //nolint:errcheck

	if err := c.buildModifiedISO(imageTarPath, tmpISO); err != nil {
		return fmt.Errorf("ISO build failed: %w", err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Atomic rename into the serving path.
	if err := os.Rename(tmpISO, c.modifiedISO); err != nil {
		return fmt.Errorf("failed to move ISO into place: %w", err)
	}

	c.lastErr = nil
	c.hasBuiltOnce = true
	c.logger.Infow("ISO rebuild complete", "path", c.modifiedISO)
	return nil
}

// refreshLoop runs in the background and periodically checks the registry
// for a newer version of the appliance image.
func (c *ApplianceImageCache) refreshLoop() {
	defer c.wg.Done()

	// Perform an initial check immediately on startup.
	c.pullAndUpdate()

	ticker := time.NewTicker(c.cfg.RefreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			c.pullAndUpdate()
		}
	}
}

// pullAndUpdate checks the registry for a new version, pulls the image if
// changed, writes it to the stable data-dir path, then calls the shared
// RebuildFromImage method.
func (c *ApplianceImageCache) pullAndUpdate() {
	pullSpec := c.cfg.GetPullSpec()

	remoteVersion, err := c.getRemoteVersion(pullSpec)
	if err != nil {
		c.logger.Warnw("failed to inspect remote image", "pullSpec", pullSpec, "error", err)
		// If we've never built, try pulling anyway — the version label might
		// simply not exist yet, and we still want an initial ISO.
		if c.hasBuiltOnce {
			return
		}
	}

	c.mu.RLock()
	sameVersion := remoteVersion != "" && remoteVersion == c.currentVersion
	c.mu.RUnlock()

	if sameVersion {
		c.logger.Debugw("remote version unchanged, skipping pull", "version", remoteVersion)
		return
	}

	c.logger.Infow("new appliance version detected", "remote", remoteVersion, "current", c.currentVersion)

	if err := c.pullImage(pullSpec); err != nil {
		c.logger.Errorw("failed to pull image", "pullSpec", pullSpec, "error", err)
		c.mu.Lock()
		c.lastErr = fmt.Errorf("pull failed: %w", err)
		c.hasBuiltOnce = true
		c.mu.Unlock()
		return
	}

	if err := c.RebuildFromImage(c.imageTarPath); err != nil {
		c.logger.Errorw("failed to rebuild ISO", "error", err)
		c.mu.Lock()
		c.lastErr = fmt.Errorf("rebuild failed: %w", err)
		c.hasBuiltOnce = true
		c.mu.Unlock()
		return
	}

	c.mu.Lock()
	c.currentVersion = remoteVersion
	c.mu.Unlock()
}

// skopeoInspect is a JSON subset we care about from `skopeo inspect`.
type skopeoInspect struct {
	Labels map[string]string `json:"Labels"`
}

// getRemoteVersion uses `skopeo inspect` to read the
// org.opencontainers.image.version label from the remote image.
func (c *ApplianceImageCache) getRemoteVersion(pullSpec string) (string, error) {
	args := []string{"inspect"}
	if !c.cfg.TLSVerify {
		args = append(args, "--tls-verify=false")
	}
	args = append(args, "docker://"+pullSpec)

	out, err := exec.CommandContext(c.ctx, "skopeo", args...).Output()
	if err != nil {
		return "", fmt.Errorf("skopeo inspect failed: %w", err)
	}

	var info skopeoInspect
	if err := json.Unmarshal(out, &info); err != nil {
		return "", fmt.Errorf("failed to parse skopeo output: %w", err)
	}

	version := info.Labels["org.opencontainers.image.version"]
	return version, nil
}

// pullImage downloads the image from the registry to the stable archive path.
func (c *ApplianceImageCache) pullImage(pullSpec string) error {
	args := []string{"copy", "-q"}
	if !c.cfg.TLSVerify {
		args = append(args, "--tls-verify=false")
	}
	args = append(args, "docker://"+pullSpec, "oci-archive:"+c.imageTarPath)

	cmd := exec.CommandContext(c.ctx, "skopeo", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// buildModifiedISO performs the same steps as build-ove-image.sh:
// extract the base ISO, copy the agent tar into images/, repackage, and
// make the result UEFI-bootable with isohybrid.
func (c *ApplianceImageCache) buildModifiedISO(imageTarPath, outputISO string) error {
	workDir, err := os.MkdirTemp(c.dataDir, "iso-build-")
	if err != nil {
		return fmt.Errorf("failed to create work dir: %w", err)
	}
	defer os.RemoveAll(workDir) //nolint:errcheck

	extractDir := filepath.Join(workDir, "extract")
	if err := os.MkdirAll(extractDir, 0o755); err != nil {
		return err
	}

	// 1. Extract base ISO using osirrox (no mount required).
	c.logger.Infow("extracting base ISO", "iso", c.baseISOPath)
	if err := runCmd(c.ctx, "osirrox", "-indev", c.baseISOPath, "-extract", "/", extractDir); err != nil {
		return fmt.Errorf("osirrox extract failed: %w", err)
	}

	// 2. Copy the appliance image archive into the extracted tree.
	imagesDir := filepath.Join(extractDir, "images")
	if err := os.MkdirAll(imagesDir, 0o755); err != nil {
		return err
	}
	destTar := filepath.Join(imagesDir, "migration-planner-agent.tar")
	if err := copyFile(imageTarPath, destTar); err != nil {
		return fmt.Errorf("failed to copy image tar: %w", err)
	}

	// 3. Read the volume label from the original ISO.
	volumeLabel, err := c.getVolumeLabel()
	if err != nil {
		c.logger.Warnw("failed to read volume label, using default", "error", err)
		volumeLabel = "RHCOS"
	}

	// 4. Compute boot load size for EFI boot image.
	efiBootImg := filepath.Join(extractDir, "images", "efiboot.img")
	info, err := os.Stat(efiBootImg)
	if err != nil {
		return fmt.Errorf("efiboot.img not found: %w", err)
	}
	bootLoadSize := fmt.Sprintf("%d", (info.Size()+2047)/2048)

	// 5. Repackage as ISO with xorriso.
	c.logger.Infow("creating ISO with xorriso")
	if err := runCmd(c.ctx, "xorriso",
		"-as", "mkisofs",
		"-o", outputISO,
		"-J", "-R", "-V", volumeLabel,
		"-b", "isolinux/isolinux.bin",
		"-c", "isolinux/boot.cat",
		"-no-emul-boot", "-boot-load-size", "4", "-boot-info-table",
		"-eltorito-alt-boot",
		"-e", "images/efiboot.img",
		"-no-emul-boot", "-boot-load-size", bootLoadSize,
		extractDir,
	); err != nil {
		return fmt.Errorf("xorriso failed: %w", err)
	}

	// 6. Make ISO UEFI bootable with isohybrid.
	c.logger.Infow("running isohybrid")
	if err := runCmd(c.ctx, "isohybrid", "--uefi", outputISO); err != nil {
		return fmt.Errorf("isohybrid failed: %w", err)
	}

	return nil
}

// getVolumeLabel extracts the ISO volume label using xorriso.
func (c *ApplianceImageCache) getVolumeLabel() (string, error) {
	out, err := exec.CommandContext(c.ctx, "xorriso",
		"-indev", c.baseISOPath, "-toc").CombinedOutput()
	if err != nil {
		return "", err
	}

	// Parse the volume label from xorriso -toc output.
	// The label is in the 4th comma-separated field of the "ISO session" line.
	for _, line := range splitLines(string(out)) {
		if contains(line, "ISO session") {
			parts := splitComma(line)
			if len(parts) >= 4 {
				return trimSpace(parts[3]), nil
			}
		}
	}
	return "", fmt.Errorf("volume label not found in xorriso output")
}

// runCmd executes a command and returns an error with stderr details on failure.
func runCmd(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s failed: %w", name, err)
	}
	return nil
}

// copyFile copies src to dst.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

// fileSHA256 computes the SHA-256 hex digest of a file.
func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Simple string helpers to avoid importing heavy packages for trivial ops.

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func splitComma(s string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	parts = append(parts, s[start:])
	return parts
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstring(s, substr)
}

func searchSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func trimSpace(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}
