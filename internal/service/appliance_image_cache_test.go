package service

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kubev2v/migration-planner/internal/config"
	"github.com/kubev2v/migration-planner/internal/image"
	"github.com/onsi/ginkgo/v2"
	"go.uber.org/zap"
)

// These specs register with the existing Service Suite so make test executes
// them alongside the database-backed Ginkgo specs.
type cacheTestingT interface {
	Helper()
	TempDir() string
	Setenv(string, string)
	Cleanup(func())
	Fatal(...interface{})
	Fatalf(string, ...interface{})
	Error(...interface{})
	Errorf(string, ...interface{})
}

var _ = ginkgo.Describe("Appliance image cache", func() {
	ginkgo.It("serves health checks while the first build is pending", func() { testApplianceCacheStartsInBackground(ginkgo.GinkgoT()) })
	ginkgo.It("retries initial pull and build failures", func() { testApplianceCacheRetriesInitialFailures(ginkgo.GinkgoT()) })
	ginkgo.It("cancels and joins the initial build on stop", func() { testApplianceCacheStopsInitialBuild(ginkgo.GinkgoT()) })
	ginkgo.It("preserves the source ISO volume label", func() { testApplianceCachePreservesVolumeLabel(ginkgo.GinkgoT()) })
	ginkgo.It("makes extracted ISO files writable without following symlinks", func() { testMakeISOFilesWritable(ginkgo.GinkgoT()) })
	ginkgo.It("keeps the last good generation when pulling or building fails", func() { testApplianceRefreshKeepsLastGoodGeneration(ginkgo.GinkgoT()) })
	ginkgo.It("keeps downloads available during a refresh", func() { testApplianceRefreshDoesNotBlockDownloads(ginkgo.GinkgoT()) })
	ginkgo.It("rejects invalid refresh intervals", func() { testApplianceCacheRejectsInvalidIntervals(ginkgo.GinkgoT()) })
	ginkgo.It("preserves open readers when expired generations are removed", func() { testExpiredCacheGenerationDoesNotInterruptOpenReader(ginkgo.GinkgoT()) })
})

func testMakeISOFilesWritable(t cacheTestingT) {
	root := t.TempDir()
	images := filepath.Join(root, "images")
	if err := os.Mkdir(images, 0755); err != nil {
		t.Fatal(err)
	}
	image := filepath.Join(images, "migration-planner-agent.tar")
	if err := os.WriteFile(image, []byte("old image"), 0444); err != nil {
		t.Fatal(err)
	}
	boot := filepath.Join(root, "boot")
	if err := os.WriteFile(boot, []byte("boot image"), 0555); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(outside, []byte("outside tree"), 0444); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(images, "link")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(root, 0755)
		_ = os.Chmod(images, 0755)
	})
	if err := os.Chmod(images, 0555); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0555); err != nil {
		t.Fatal(err)
	}

	if err := makeISOFilesWritable(root); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(image, []byte("new image"), 0644); err != nil {
		t.Fatalf("cannot replace existing appliance image: %v", err)
	}
	if err := os.WriteFile(filepath.Join(images, "new.tar"), nil, 0644); err != nil {
		t.Fatalf("cannot add appliance image: %v", err)
	}
	info, err := os.Stat(boot)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0755 {
		t.Errorf("boot permissions = %o, want 755", info.Mode().Perm())
	}
	info, err = os.Stat(outside)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0444 {
		t.Errorf("symlink target permissions changed to %o", info.Mode().Perm())
	}
	if err := os.RemoveAll(root); err != nil {
		t.Fatalf("cannot clean up extracted ISO: %v", err)
	}
}

// Fake command-line tools exercise the real refresh/publication pipeline without
// downloading a registry image or building a multi-gigabyte ISO in unit tests.
func newTestApplianceCache(t cacheTestingT) (*ApplianceImageCache, string) {
	t.Helper()
	cache, root := newUnstartedTestApplianceCache(t)
	if err := cache.Start(); err != nil {
		t.Fatal(err)
	}
	waitForCacheTest(t, func() bool { return cache.HealthCheck() == nil })
	return cache, root
}

func newUnstartedTestApplianceCache(t cacheTestingT) (*ApplianceImageCache, string) {
	t.Helper()
	root := t.TempDir()
	tools := filepath.Join(root, "tools")
	if err := os.Mkdir(tools, 0755); err != nil {
		t.Fatal(err)
	}
	commands := map[string]string{
		"skopeo": `
touch "$CACHE_TEST_ROOT/pull-attempted"
if [ -f "$CACHE_TEST_ROOT/fail-pull" ]; then touch "$CACHE_TEST_ROOT/failed-pull"; exit 1; fi
if [ -f "$CACHE_TEST_ROOT/block" ]; then
 touch "$CACHE_TEST_ROOT/pulling"
 while [ -f "$CACHE_TEST_ROOT/block" ]; do sleep 0.05; done
fi
for arg do target="$arg"; done
cat "$CACHE_TEST_ROOT/image" > "${target#oci-archive:}"
`,
		"osirrox": `
for arg do target="$arg"; done
mkdir -p "$target/images" "$target/isolinux"
printf boot > "$target/images/efiboot.img"
chmod 555 "$target" "$target/images" "$target/isolinux"
`,
		"rsync": `cp -R "$2". "$3"`,
		"xorriso": `
if [ "$3" = "-toc" ]; then
 if [ -f "$CACHE_TEST_ROOT/missing-label" ]; then exit 0; fi
 printf "Volume id    : 'rhcos-test-label'\n" >&2
 exit 0
fi
previous=""
for arg do
 if [ "$previous" = "-V" ]; then printf '%s' "$arg" > "$CACHE_TEST_ROOT/volume-label"; fi
 previous="$arg"
done
if [ -f "$CACHE_TEST_ROOT/fail-build" ]; then touch "$CACHE_TEST_ROOT/failed-build"; exit 1; fi
for arg do work="$arg"; done
cat "$work/images/migration-planner-agent.tar" > "$4"
`,
		"isohybrid": `exit 0`,
	}
	for name, script := range commands {
		if err := os.WriteFile(filepath.Join(tools, name), []byte("#!/bin/sh\nset -eu\n"+script), 0755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", tools+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("CACHE_TEST_ROOT", root)
	if err := os.WriteFile(filepath.Join(root, "image"), []byte("image one"), 0644); err != nil {
		t.Fatal(err)
	}
	cache := NewApplianceImageCache(&config.ApplianceImage{Image: "test/appliance", Tag: "latest", CachePath: filepath.Join(root, "cache"), RefreshInterval: "24h", TLSVerify: true}, "base.iso", zap.NewNop())
	t.Cleanup(cache.Stop)
	return cache, root
}

func testApplianceRefreshKeepsLastGoodGeneration(t cacheTestingT) {
	cache, root := newTestApplianceCache(t)
	oldPath, oldDigest, err := cache.GetModifiedISO()
	if err != nil {
		t.Fatal(err)
	}
	oldTime := cache.GetLastPullTime()
	for _, failure := range []string{"fail-pull", "fail-build"} {
		if err := os.WriteFile(filepath.Join(root, failure), nil, 0644); err != nil {
			t.Fatal(err)
		}
		if err := cache.RefreshNow(); err == nil {
			t.Fatalf("%s unexpectedly succeeded", failure)
		}
		if err := cache.HealthCheck(); err != nil {
			t.Fatalf("last good image became unhealthy: %v", err)
		}
		path, digest, err := cache.GetModifiedISO()
		if err != nil || path != oldPath || digest != oldDigest || !cache.GetLastPullTime().Equal(oldTime) {
			t.Fatalf("failed refresh changed published generation: %s, %v", path, err)
		}
		if err := os.Remove(filepath.Join(root, failure)); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := os.ReadDir(cache.cfg.CachePath)
	if err != nil {
		t.Fatal(err)
	}
	versions := 0
	for _, entry := range entries {
		if entry.IsDir() {
			versions++
		}
	}
	if versions != 1 {
		t.Fatalf("failed generations leaked: %d directories", versions)
	}
	if err := os.WriteFile(filepath.Join(root, "image"), []byte("image two"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := cache.RefreshNow(); err != nil {
		t.Fatal(err)
	}
	newPath, newDigest, err := cache.GetModifiedISO()
	if err != nil || newPath == oldPath || newDigest == oldDigest {
		t.Fatalf("new generation not published: %s, %v", newPath, err)
	}
	for path, want := range map[string]string{oldPath: "image one", newPath: "image two", filepath.Join(cache.cfg.CachePath, "appliance-image.tar"): "image two"} {
		content, err := os.ReadFile(path)
		if err != nil || string(content) != want {
			t.Fatalf("%s = %q, %v; want %q", path, content, err, want)
		}
	}
}

func testApplianceRefreshDoesNotBlockDownloads(t cacheTestingT) {
	cache, root := newTestApplianceCache(t)
	if err := os.WriteFile(filepath.Join(root, "block"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cache.RefreshNow() }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(root, "pulling")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("refresh did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	available := make(chan error, 1)
	go func() { _, err := cache.GetModifiedISOPath(); available <- err }()
	select {
	case err := <-available:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Error("refresh blocked download readers")
	}
	if err := os.Remove(filepath.Join(root, "block")); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("refresh did not finish")
	}
}

func testApplianceCacheRejectsInvalidIntervals(t cacheTestingT) {
	for _, interval := range []string{"invalid", "0s", "-1h"} {
		cache := NewApplianceImageCache(&config.ApplianceImage{RefreshInterval: interval}, "base.iso", zap.NewNop())
		if err := cache.Start(); err == nil {
			t.Fatalf("accepted interval %q", interval)
		}
		cache.Stop()
	}
}

func testExpiredCacheGenerationDoesNotInterruptOpenReader(t cacheTestingT) {
	cache, root := newTestApplianceCache(t)
	oldPath, err := cache.GetModifiedISOPath()
	if err != nil {
		t.Fatal(err)
	}
	reader, err := os.Open(oldPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Close() }()
	expired := time.Now().Add(-5 * time.Hour)
	if err := os.Chtimes(filepath.Dir(oldPath), expired, expired); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "image"), []byte("new image"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := cache.RefreshNow(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("expired generation remains: %v", err)
	}
	content, err := io.ReadAll(reader)
	if err != nil || string(content) != "image one" {
		t.Fatalf("active reader interrupted by cleanup: %q, %v", content, err)
	}
}

func waitForCacheTest(t cacheTestingT, condition func() bool) {
	t.Helper()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for !condition() {
		select {
		case <-deadline.C:
			t.Fatal("timed out waiting for cache condition")
		case <-tick.C:
		}
	}
}

func testApplianceCacheStartsInBackground(t cacheTestingT) {
	cache, root := newUnstartedTestApplianceCache(t)
	if err := os.WriteFile(filepath.Join(root, "block"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	started := make(chan error, 1)
	go func() { started <- cache.Start() }()
	select {
	case err := <-started:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("Start blocked on the initial pull")
	}
	waitForCacheTest(t, func() bool {
		_, err := os.Stat(filepath.Join(root, "pulling"))
		return err == nil
	})
	if err := cache.HealthCheck(); err == nil {
		t.Fatal("unpublished cache reported ready")
	}
	if err := os.Remove(filepath.Join(root, "block")); err != nil {
		t.Fatal(err)
	}
	waitForCacheTest(t, func() bool { return cache.HealthCheck() == nil })
	path, digest, err := cache.GetModifiedISO()
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	want, err := image.ContentETag(context.Background(), file)
	if err != nil || digest == "" || digest != want {
		t.Fatalf("published ISO digest = %q, want %q, error %v", digest, want, err)
	}
}

func testApplianceCacheRetriesInitialFailures(t cacheTestingT) {
	for _, failure := range []string{"fail-pull", "fail-build"} {
		cache, root := newUnstartedTestApplianceCache(t)
		cache.cfg.RefreshInterval = "10ms"
		marker := filepath.Join(root, failure)
		if err := os.WriteFile(marker, nil, 0644); err != nil {
			t.Fatal(err)
		}
		if err := cache.Start(); err != nil {
			t.Fatalf("transient %s failed startup: %v", failure, err)
		}
		waitForCacheTest(t, func() bool {
			_, err := os.Stat(filepath.Join(root, strings.Replace(failure, "fail-", "failed-", 1)))
			return err == nil
		})
		if err := cache.HealthCheck(); err == nil {
			t.Fatalf("cache ready despite %s", failure)
		}
		if err := os.Remove(marker); err != nil {
			t.Fatal(err)
		}
		waitForCacheTest(t, func() bool { return cache.HealthCheck() == nil })
		cache.Stop()
	}
}

func testApplianceCacheStopsInitialBuild(t cacheTestingT) {
	cache, root := newUnstartedTestApplianceCache(t)
	if err := os.WriteFile(filepath.Join(root, "block"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	if err := cache.Start(); err != nil {
		t.Fatal(err)
	}
	waitForCacheTest(t, func() bool {
		_, err := os.Stat(filepath.Join(root, "pulling"))
		return err == nil
	})
	stopped := make(chan struct{})
	go func() { cache.Stop(); close(stopped) }()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("Stop did not cancel and join the initial build")
	}
	if err := cache.HealthCheck(); err == nil {
		t.Fatal("cancelled build was published")
	}
	if err := cache.Start(); err == nil {
		t.Fatal("restarted a stopped cache")
	}
}

func testApplianceCachePreservesVolumeLabel(t cacheTestingT) {
	for _, label := range []string{"rhcos-9.6.20250826-1", "label with spaces", "label's punctuation"} {
		got, err := parseISOVolumeLabel("diagnostic\nVolume id    : '" + label + "'\n")
		if err != nil || got != label {
			t.Fatalf("volume label = %q, want %q, error %v", got, label, err)
		}
	}
	for _, output := range []string{"", "Volume id : ''", "Volume id : unquoted"} {
		if _, err := parseISOVolumeLabel(output); err == nil {
			t.Fatalf("accepted missing/malformed label: %q", output)
		}
	}
	cache, root := newTestApplianceCache(t)
	label, err := os.ReadFile(filepath.Join(root, "volume-label"))
	if err != nil || string(label) != "rhcos-test-label" {
		t.Fatalf("mkisofs volume label = %q, error %v", label, err)
	}
	oldPath, oldDigest, err := cache.GetModifiedISO()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "missing-label"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	if err := cache.RefreshNow(); err == nil || !strings.Contains(err.Error(), "volume label") {
		t.Fatalf("missing label refresh error = %v", err)
	}
	path, digest, err := cache.GetModifiedISO()
	if err != nil || path != oldPath || digest != oldDigest {
		t.Fatalf("failed build changed generation: %q, %q, %v", path, digest, err)
	}
}
