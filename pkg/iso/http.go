package iso

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
)

type HttpDownloader struct {
	imagePath   string
	imageSha256 string
}

func NewHttpDownloader(imagePath string, imageSha256 string) *HttpDownloader {
	return &HttpDownloader{imagePath: imagePath, imageSha256: imageSha256}
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
	mw := newProgressWriter(newCtx, dst, totalSize)
	imageHasher := newImageHasher(mw)

	_, err = io.Copy(imageHasher, resp.Body)
	if err != nil {
		return err
	}

	computedSum := imageHasher.Sum()
	if h.imageSha256 != computedSum {
		return fmt.Errorf("failed to download the image. wanted %q received %q", h.imageSha256, computedSum)
	}

	return nil
}

func (h *HttpDownloader) Type() string {
	return "http"
}
