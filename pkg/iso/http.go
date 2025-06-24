package iso

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/kubev2v/migration-planner/internal/util"
)

const (
	rhcosUrl string = "https://mirror.openshift.com/pub/openshift-v4/dependencies/rhcos/latest/rhcos-live-iso.x86_64.iso"
)

type HttpDownloader struct {
	imagePath string
}

func NewRHCOSHttpDownloader() *HttpDownloader {
	return NewHttpDownloader(util.GetEnv("MIGRATION_PLANNER_ISO_URL", rhcosUrl))
}

func NewHttpDownloader(imagePath string) *HttpDownloader {
	return &HttpDownloader{imagePath: imagePath}
}

func (h *HttpDownloader) Get(ctx context.Context, dst io.Writer) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.imagePath, nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download ISO %q, status code: %d", h.imagePath, resp.StatusCode)
	}

	totalSize := int64(0)
	n, err := strconv.ParseInt(resp.Header.Get("Content-Length"), 10, 64)
	if err == nil {
		totalSize = n
	}

	newCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	mw := newWrapper(newCtx, dst, totalSize)

	_, err = io.Copy(mw, resp.Body)
	if err != nil {
		return err
	}

	if mw.total > 0 && (mw.total != mw.downloadedBytes) {
		return fmt.Errorf("failed to download the entire image. expected bytes %d received %d", mw.total, mw.downloadedBytes)
	}

	return nil
}

func (h *HttpDownloader) Type() string {
	return "http"
}
