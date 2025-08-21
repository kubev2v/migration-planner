package iso

import (
	"context"
	"crypto/sha256"
	"fmt"
	"hash"
	"io"
	"time"

	"go.uber.org/zap"
)

// progressWriter is a progressWriter around the io.Writer to get metrics about download progress.
type progressWriter struct {
	downloadedBytes int64
	total           int64
	w               io.Writer
}

func newProgressWriter(ctx context.Context, w io.Writer, totalBytesToDownload int64) *progressWriter {
	mw := &progressWriter{w: w, total: totalBytesToDownload}
	go mw.start(ctx)

	return mw
}

func (m *progressWriter) start(ctx context.Context) {
	oldValue := int64(0)
	ticker := time.NewTicker(10 * time.Second)
	for {
		select {
		case <-ctx.Done():
			ticker.Stop()
			return
		case <-ticker.C:
			if m.total == 0 {
				progress := fmt.Sprintf("%.2f Mb", float32(m.downloadedBytes)/(1024*1024))
				zap.S().Debugw("iso downloading", "progress", progress)
				continue
			}

			progress := fmt.Sprintf("%.2f%%", 100*(float32(m.downloadedBytes)/float32(m.total)))
			rate := fmt.Sprintf("%.2f MB/s", (float32(m.downloadedBytes)-float32(oldValue))/(1024*1024*10))
			zap.S().Debugw("iso downloading", "progress", progress, "rate", rate)
			oldValue = m.downloadedBytes
		}
	}
}

func (m *progressWriter) Write(p []byte) (n int, err error) {
	n, err = m.w.Write(p)
	if err == nil {
		m.downloadedBytes += int64(n)
	}
	return
}

type imageHasher struct {
	w      io.Writer
	hasher hash.Hash
}

func newImageHasher(w io.Writer) *imageHasher {
	return &imageHasher{
		w:      w,
		hasher: sha256.New(),
	}
}

func (ih *imageHasher) Write(p []byte) (n int, err error) {
	n, err = ih.w.Write(p)
	if err != nil {
		return
	}
	_, _ = ih.hasher.Write(p)
	return
}

func (ih *imageHasher) Sum() string {
	return fmt.Sprintf("%x", ih.hasher.Sum(nil))
}

// DownloadWithValidation downloads data from src to dst with progress tracking and SHA256 validation
func DownloadWithValidation(ctx context.Context, src io.Reader, dst io.Writer, expectedSha256 string, totalSize int64) error {
	newCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Create progress tracking writer
	mw := newProgressWriter(newCtx, dst, totalSize)

	// Create hash validation writer
	imageHasher := newImageHasher(mw)

	// Copy data through the validation chain
	_, err := io.Copy(imageHasher, src)
	if err != nil {
		return fmt.Errorf("failed to copy data: %w", err)
	}

	// Verify SHA256 hash
	computedSum := imageHasher.Sum()
	if expectedSha256 != computedSum {
		return fmt.Errorf("SHA256 hash mismatch. expected %q, got %q", expectedSha256, computedSum)
	}

	return nil
}
