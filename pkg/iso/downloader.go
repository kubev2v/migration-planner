package iso

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"go.uber.org/zap"
)

type Downloader interface {
	Get(ctx context.Context, dst io.Writer) error
	Type() string
}

type Manager struct {
	downloaders map[int]Downloader // keep them in the order of the registration
}

func NewDownloaderManager() *Manager {
	return &Manager{
		downloaders: map[int]Downloader{},
	}
}

func (m *Manager) Register(downloader Downloader) *Manager {
	m.downloaders[len(m.downloaders)] = downloader
	return m
}

func (m *Manager) Download(ctx context.Context, dst io.Writer) error {
	for i := 0; i < len(m.downloaders); i++ {
		downloader := m.downloaders[i]

		zap.S().Infow("downloading iso image", "downloader_type", downloader.Type())

		if err := downloader.Get(ctx, dst); err != nil {
			zap.S().Errorw("failed to download image", "error", err, "downloader_type", downloader.Type())
			continue
		}

		return nil
	}

	return errors.New("failed to download image. All downloaders failed")
}

// wrapper is a wrapper around the io.Writer to get metrics about download progress.
type wrapper struct {
	downloadedBytes int64
	total           int64
	w               io.Writer
}

func newWrapper(ctx context.Context, w io.Writer, totalBytesToDownload int64) *wrapper {
	mw := &wrapper{w: w, total: totalBytesToDownload}
	go mw.start(ctx)

	return mw
}

func (m *wrapper) start(ctx context.Context) {
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

func (m *wrapper) Write(p []byte) (n int, err error) {
	n, err = m.w.Write(p)
	if err == nil {
		m.downloadedBytes += int64(n)
	}
	return
}
