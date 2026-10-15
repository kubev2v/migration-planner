package service

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/kubev2v/migration-planner/internal/config"
	"github.com/kubev2v/migration-planner/internal/image"
	"go.uber.org/zap"
)

// ApplianceImageCache manages the cached appliance container image and modified ISO
type ApplianceImageCache struct {
	cfg               *config.ApplianceImage
	logger            *zap.Logger
	cacheMutex        sync.RWMutex
	refreshMutex      sync.Mutex
	imageTarPath      string
	modifiedISOPath   string
	modifiedISODigest string
	baseISOPath       string
	lastPullTime      time.Time
	worker            sync.WaitGroup
	started           bool
	ctx               context.Context
	cancel            context.CancelFunc
}

// NewApplianceImageCache creates a new appliance image cache service
func NewApplianceImageCache(cfg *config.ApplianceImage, baseISOPath string, logger *zap.Logger) *ApplianceImageCache {
	ctx, cancel := context.WithCancel(context.Background())
	return &ApplianceImageCache{
		cfg:         cfg,
		logger:      logger,
		baseISOPath: baseISOPath,
		ctx:         ctx,
		cancel:      cancel,
	}
}

// Start initializes the cache and begins periodic refresh
func (c *ApplianceImageCache) Start() error {
	c.logger.Info("Starting appliance image cache service",
		zap.String("image", c.cfg.Image),
		zap.String("tag", c.cfg.Tag),
		zap.String("pullSpec", c.cfg.GetPullSpec()),
		zap.String("cachePath", c.cfg.CachePath),
		zap.String("refreshInterval", c.cfg.RefreshInterval))

	refreshInterval, err := time.ParseDuration(c.cfg.RefreshInterval)
	if err != nil || refreshInterval <= 0 {
		return fmt.Errorf("refresh interval must be a positive duration, got %q", c.cfg.RefreshInterval)
	}

	// Ensure cache directory exists
	if err := os.MkdirAll(c.cfg.CachePath, 0755); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	// Start the initial build in the background so HTTP health checks remain
	// available while the registry is unavailable or an ISO is being built.
	c.cacheMutex.Lock()
	if err := c.ctx.Err(); err != nil {
		c.cacheMutex.Unlock()
		return err
	}
	if c.started {
		c.cacheMutex.Unlock()
		return fmt.Errorf("appliance image cache already started")
	}
	c.started = true
	c.worker.Add(1)
	go c.refreshLoop(refreshInterval)
	c.cacheMutex.Unlock()

	c.logger.Info("Appliance image cache service started successfully")
	return nil
}

// Stop gracefully shuts down the cache service
func (c *ApplianceImageCache) Stop() {
	c.logger.Info("Stopping appliance image cache service")
	// Synchronize cancellation with Start before waiting, so Start cannot
	// add a worker once Stop begins waiting.
	c.cacheMutex.Lock()
	c.cancel()
	c.cacheMutex.Unlock()
	c.worker.Wait()
}

// GetModifiedISOPath returns the path to the ISO with embedded appliance image
func (c *ApplianceImageCache) GetModifiedISOPath() (string, error) {
	path, _, err := c.GetModifiedISO()
	return path, err
}

// GetModifiedISO returns an immutable generation's path and content digest
// together, so a concurrent refresh cannot pair bytes with another validator.
func (c *ApplianceImageCache) GetModifiedISO() (string, string, error) {
	c.cacheMutex.RLock()
	defer c.cacheMutex.RUnlock()

	if _, err := os.Stat(c.modifiedISOPath); err != nil {
		return "", "", fmt.Errorf("cached ISO not found: %w", err)
	}

	return c.modifiedISOPath, c.modifiedISODigest, nil
}

// RefreshNow forces an immediate refresh of the cached image and ISO
func (c *ApplianceImageCache) RefreshNow() error {
	c.logger.Info("Manual refresh requested")
	return c.pullAndBuildISO()
}

// HealthCheck verifies that the cached image exists and is valid
func (c *ApplianceImageCache) HealthCheck() error {
	c.cacheMutex.RLock()
	defer c.cacheMutex.RUnlock()

	if _, err := os.Stat(c.modifiedISOPath); err != nil {
		return fmt.Errorf("cached ISO file not found: %w", err)
	}

	return nil
}

// refreshLoop builds immediately, retries failures with bounded backoff, and
// waits the configured interval after a successful publication.
func (c *ApplianceImageCache) refreshLoop(refreshInterval time.Duration) {
	defer c.worker.Done()
	timer := time.NewTimer(0)
	defer timer.Stop()
	retryDelay := time.Second
	maxRetryDelay := min(30*time.Second, refreshInterval)
	for {
		select {
		case <-c.ctx.Done():
			c.logger.Info("Refresh loop stopped")
			return
		case <-timer.C:
			if c.ctx.Err() != nil {
				return
			}
			if err := c.pullAndBuildISO(); err != nil {
				if c.ctx.Err() != nil {
					return
				}
				delay := min(retryDelay, maxRetryDelay)
				c.logger.Error("Appliance image refresh failed", zap.Error(err), zap.Duration("retryDelay", delay))
				timer.Reset(delay)
				retryDelay = min(2*retryDelay, maxRetryDelay)
			} else {
				retryDelay = time.Second
				timer.Reset(refreshInterval)
			}
		}
	}
}

// pullAndBuildISO pulls the appliance image and builds a modified ISO with it embedded
func (c *ApplianceImageCache) pullAndBuildISO() error {
	c.refreshMutex.Lock()
	defer c.refreshMutex.Unlock()

	// Build in a private generation. Readers never see partially refreshed files
	// and an already-open download continues to read its original ISO.
	versionDir, err := os.MkdirTemp(c.cfg.CachePath, "version-")
	if err != nil {
		return err
	}
	candidate := &ApplianceImageCache{
		cfg: c.cfg, logger: c.logger, ctx: c.ctx, baseISOPath: c.baseISOPath,
		imageTarPath:    filepath.Join(versionDir, "appliance-image.tar"),
		modifiedISOPath: filepath.Join(versionDir, "rhcos-with-appliance.iso"),
	}
	published := false
	defer func() {
		if !published {
			if err := os.RemoveAll(versionDir); err != nil {
				c.logger.Warn("Failed to remove unsuccessful cache generation", zap.Error(err))
			}
		}
	}()
	if err := candidate.pullImage(); err != nil {
		return fmt.Errorf("failed to pull appliance image: %w", err)
	}
	if err := candidate.buildModifiedISO(); err != nil {
		return fmt.Errorf("failed to build modified ISO: %w", err)
	}
	iso, err := os.Open(candidate.modifiedISOPath)
	if err != nil {
		return fmt.Errorf("failed to open ISO for hashing: %w", err)
	}
	digest, hashErr := image.ContentETag(c.ctx, iso)
	closeErr := iso.Close()
	if hashErr != nil {
		return fmt.Errorf("failed to hash ISO generation: %w", hashErr)
	}
	if closeErr != nil {
		return fmt.Errorf("failed to close ISO generation: %w", closeErr)
	}
	c.cacheMutex.Lock()
	if err := c.ctx.Err(); err != nil {
		c.cacheMutex.Unlock()
		return err
	}
	c.modifiedISODigest = digest
	c.imageTarPath = candidate.imageTarPath
	c.modifiedISOPath = candidate.modifiedISOPath
	c.lastPullTime = candidate.lastPullTime
	c.cacheMutex.Unlock()
	published = true
	// Compatibility aliases are for inspection only, never for serving.
	for _, name := range []string{"appliance-image.tar", "rhcos-with-appliance.iso"} {
		if err := c.publishAlias(versionDir, name); err != nil {
			c.logger.Warn("Failed to update cache inspection alias", zap.String("name", name), zap.Error(err))
		}
	}
	c.removeExpiredVersions(versionDir)
	return nil
}

// publishAlias atomically updates a compatibility path without touching a
// published generation. A unique temporary path also tolerates stale files.
func (c *ApplianceImageCache) publishAlias(versionDir, name string) error {
	alias := filepath.Join(c.cfg.CachePath, name)
	temp := filepath.Join(versionDir, name+".link")
	target, err := filepath.Abs(filepath.Join(versionDir, name))
	if err != nil {
		return err
	}
	if err := os.Symlink(target, temp); err != nil {
		return err
	}
	defer func() { _ = os.Remove(temp) }()
	return os.Rename(temp, alias)
}

// Retain generations for at least the download URL lifetime. Unlinking an old
// generation does not interrupt readers that already have its ISO open.
func (c *ApplianceImageCache) removeExpiredVersions(current string) {
	entries, err := os.ReadDir(c.cfg.CachePath)
	if err != nil {
		c.logger.Warn("Failed to list old cache generations", zap.Error(err))
		return
	}
	for _, entry := range entries {
		path := filepath.Join(c.cfg.CachePath, entry.Name())
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "version-") || path == current {
			continue
		}
		info, err := entry.Info()
		if err != nil || time.Since(info.ModTime()) < image.ImageExpirationTime {
			continue
		}
		if err := os.RemoveAll(path); err != nil {
			c.logger.Warn("Failed to remove old cache generation", zap.String("path", path), zap.Error(err))
		}
	}
}

// buildModifiedISO creates an ISO with the appliance image embedded
func (c *ApplianceImageCache) buildModifiedISO() error {
	c.logger.Info("Building modified ISO with appliance image",
		zap.String("baseISO", c.baseISOPath),
		zap.String("applianceImage", c.imageTarPath),
		zap.String("outputISO", c.modifiedISOPath))

	tempDir := filepath.Join(filepath.Dir(c.modifiedISOPath), "iso-build")
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	// A previous failed extraction may have left read-only ISO files behind.
	if err := makeISOFilesWritable(tempDir); err != nil {
		return fmt.Errorf("failed to prepare temp directory: %w", err)
	}
	defer func() {
		// Extraction can fail after restoring read-only directory permissions.
		if err := makeISOFilesWritable(tempDir); err != nil {
			c.logger.Warn("Failed to make temporary ISO files writable", zap.Error(err))
		}
		if err := os.RemoveAll(tempDir); err != nil {
			c.logger.Warn("Failed to remove temporary ISO build directory",
				zap.String("path", tempDir), zap.Error(err))
		}
	}()

	workDir := filepath.Join(tempDir, "work")
	mountDir := filepath.Join(tempDir, "mount")
	if err := os.MkdirAll(workDir, 0755); err != nil {
		return fmt.Errorf("failed to create work directory: %w", err)
	}
	if err := os.MkdirAll(mountDir, 0755); err != nil {
		return fmt.Errorf("failed to create mount directory: %w", err)
	}

	// Extract ISO contents using osirrox
	c.logger.Info("Extracting ISO contents")
	cmd := exec.CommandContext(c.ctx, "osirrox", "-indev", c.baseISOPath, "-extract", "/", mountDir)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to extract ISO: %w, output: %s", err, string(output))
	}
	// ISO permissions are read-only; rsync -a would preserve them in workDir.
	if err := makeISOFilesWritable(mountDir); err != nil {
		return fmt.Errorf("failed to make extracted ISO files writable: %w", err)
	}

	// Copy extracted contents to work directory
	c.logger.Info("Copying ISO contents to work directory")
	cmd = exec.CommandContext(c.ctx, "rsync", "-aH", mountDir+"/", workDir+"/")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to copy ISO contents: %w, output: %s", err, string(output))
	}

	// Create images directory and copy appliance tar
	imagesDir := filepath.Join(workDir, "images")
	if err := os.MkdirAll(imagesDir, 0755); err != nil {
		return fmt.Errorf("failed to create images directory: %w", err)
	}

	applianceTarDest := filepath.Join(imagesDir, "migration-planner-agent.tar")
	c.logger.Info("Copying appliance image to ISO", zap.String("dest", applianceTarDest))
	cmd = exec.CommandContext(c.ctx, "cp", c.imageTarPath, applianceTarDest)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to copy appliance tar: %w, output: %s", err, string(output))
	}

	// Extract volume label from original ISO
	cmd = exec.CommandContext(c.ctx, "xorriso", "-indev", c.baseISOPath, "-toc")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to read ISO volume label: %w, output: %s", err, output)
	}
	volumeLabel, err := parseISOVolumeLabel(string(output))
	if err != nil {
		return err
	}

	// Get boot image info
	bootImagePath := filepath.Join(workDir, "images", "efiboot.img")
	stat, err := os.Stat(bootImagePath)
	if err != nil {
		return fmt.Errorf("failed to stat boot image: %w", err)
	}
	bootLoadSize := (stat.Size() + 2047) / 2048

	// Create modified ISO
	tempISOPath := c.modifiedISOPath + ".tmp"
	c.logger.Info("Creating modified ISO with xorriso")
	cmd = exec.CommandContext(c.ctx, "xorriso", "-as", "mkisofs",
		"-o", tempISOPath,
		"-J", "-R", "-V", volumeLabel,
		"-b", "isolinux/isolinux.bin",
		"-c", "isolinux/boot.cat",
		"-no-emul-boot", "-boot-load-size", "4", "-boot-info-table",
		"-eltorito-alt-boot",
		"-e", "images/efiboot.img",
		"-no-emul-boot", "-boot-load-size", fmt.Sprintf("%d", bootLoadSize),
		workDir)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to create ISO: %w, output: %s", err, string(output))
	}

	// Make ISO UEFI bootable
	c.logger.Info("Making ISO UEFI bootable")
	cmd = exec.CommandContext(c.ctx, "isohybrid", "--uefi", tempISOPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to make ISO bootable: %w, output: %s", err, string(output))
	}

	// Atomically move temp ISO to final location
	if err := os.Rename(tempISOPath, c.modifiedISOPath); err != nil {
		return fmt.Errorf("failed to move temp ISO to cache: %w", err)
	}

	isoStat, err := os.Stat(c.modifiedISOPath)
	if err != nil {
		return fmt.Errorf("failed to stat modified ISO: %w", err)
	}
	c.logger.Info("Successfully built modified ISO",
		zap.String("path", c.modifiedISOPath),
		zap.Int64("size", isoStat.Size()))

	return nil
}

// parseISOVolumeLabel reads xorriso's quoted volume ID, preserving spaces and
// punctuation inside the label. Never substitute a label that boot arguments
// in the source ISO do not reference.
func parseISOVolumeLabel(output string) (string, error) {
	for _, line := range strings.Split(output, "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok || strings.TrimSpace(key) != "Volume id" {
			continue
		}
		value = strings.TrimSpace(value)
		if len(value) > 2 && strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'") {
			return value[1 : len(value)-1], nil
		}
	}
	return "", fmt.Errorf("source ISO volume label is missing or malformed")
}

// makeISOFilesWritable grants the owner access to edit and remove an extracted
// ISO tree without following symbolic links or removing existing permissions.
func makeISOFilesWritable(root string) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return os.Chmod(path, info.Mode().Perm()|0700)
		}
		if info.Mode().IsRegular() {
			return os.Chmod(path, info.Mode().Perm()|0600)
		}
		return nil
	})
}

// pullImage pulls the appliance image from the registry and saves it as a tar
func (c *ApplianceImageCache) pullImage() error {
	pullSpec := c.cfg.GetPullSpec()
	c.logger.Info("Pulling appliance image", zap.String("pullSpec", pullSpec))

	tempTarPath := c.imageTarPath + ".tmp"

	// Build skopeo command
	args := []string{
		"--override-os", "linux", "--override-arch", "amd64",
		"copy",
		"-q", // quiet mode
	}

	if !c.cfg.TLSVerify {
		args = append(args, "--tls-verify=false")
	}

	args = append(args,
		fmt.Sprintf("docker://%s", pullSpec),
		fmt.Sprintf("oci-archive:%s", tempTarPath),
	)

	cmd := exec.CommandContext(c.ctx, "skopeo", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("skopeo copy failed: %w, output: %s", err, string(output))
	}

	// Atomically move temp file to final location
	if err := os.Rename(tempTarPath, c.imageTarPath); err != nil {
		return fmt.Errorf("failed to move temp file to cache: %w", err)
	}

	c.lastPullTime = time.Now()

	c.logger.Info("Successfully pulled and cached appliance image",
		zap.String("path", c.imageTarPath),
		zap.Time("pullTime", c.lastPullTime))

	return nil
}

// GetLastPullTime returns the timestamp of the last successful image pull
func (c *ApplianceImageCache) GetLastPullTime() time.Time {
	c.cacheMutex.RLock()
	defer c.cacheMutex.RUnlock()
	return c.lastPullTime
}
