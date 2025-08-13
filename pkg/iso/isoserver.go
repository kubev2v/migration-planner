package iso

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/kubev2v/migration-planner/internal/api_server/isoserver"
)

type ServerDownloader struct {
	isoEndpointUrl string
	imageSha256    string
	client         *http.Client
}

func NewIsoServerDownloader(baseServerURL, imageSha256 string) (*ServerDownloader, error) {
	u, err := url.Parse(baseServerURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}
	fullURL := u.ResolveReference(&url.URL{Path: isoserver.IsoEndpoint})

	return &ServerDownloader{
		isoEndpointUrl: fullURL.String(),
		imageSha256:    imageSha256,
		client:         &http.Client{Timeout: time.Minute},
	}, nil
}

func (i *ServerDownloader) Get(ctx context.Context, dst io.Writer) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, i.isoEndpointUrl, nil)
	if err != nil {
		return fmt.Errorf("failed to create request for ISO server: %w", err)
	}

	resp, err := i.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch ISO from server %q: %w", i.isoEndpointUrl, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ISO server returned status %d for %q", resp.StatusCode, i.isoEndpointUrl)
	}

	totalSize := int64(0)
	if contentLength := resp.Header.Get("Content-Length"); contentLength != "" {
		if n, err := strconv.ParseInt(contentLength, 10, 64); err == nil {
			totalSize = n
		}
	}

	return DownloadWithValidation(ctx, resp.Body, dst, i.imageSha256, totalSize)
}

// HealthCheck verifies if the ISO server is available and has the required ISO
func (i *ServerDownloader) HealthCheck(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, i.isoEndpointUrl, nil)
	if err != nil {
		return fmt.Errorf("failed to create health check request: %w", err)
	}

	resp, err := i.client.Do(req)
	if err != nil {
		return fmt.Errorf("ISO server health check failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ISO server health check returned status %d", resp.StatusCode)
	}

	return nil
}

func (i *ServerDownloader) Type() string {
	return "iso-server"
}
