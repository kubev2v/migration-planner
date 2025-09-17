package iso

import (
	"context"
	"errors"
	"fmt"
	"io"

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

func (m *Manager) Download(ctx context.Context, dst io.WriteSeeker) error {
	for i := 0; i < len(m.downloaders); i++ {
		downloader := m.downloaders[i]

		zap.S().Infow("downloading iso image", "downloader_type", downloader.Type())

		if err := downloader.Get(ctx, dst); err != nil {
			zap.S().Errorw("failed to download image", "error", err, "downloader_type", downloader.Type())

			// rewind buffer
			if _, err := dst.Seek(0, io.SeekStart); err != nil {
				return fmt.Errorf("failed to rewind the buffer to 0 again: %w", err)
			}

			continue
		}

		return nil
	}

	return errors.New("failed to download image. All downloaders failed")
}
