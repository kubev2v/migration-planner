// The HTTPWriter was written according to the guide in the link below:
// https://inscope.corp.redhat.com/docs/default/component/notifications-app/user-guide/send-notification/
// Email templates can be found here: https://github.com/RedHatInsights/notifications-backend/tree/master/common-template/src/main/resources/templates/email/Oma

package notification

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"
)

// Writer sends an already-built notification payload (see Build) to the
// notification service.
type Writer interface {
	Write(ctx context.Context, data []byte) error
}

// HTTPWriter sends notification payloads to the notification service over
// an mTLS-authenticated HTTPS connection.
type HTTPWriter struct {
	url    string
	client *http.Client
}

// NewHTTPWriter builds an HTTPWriter that authenticates with the notification
// service using the given PEM-encoded client certificate and private key.
func NewHTTPWriter(notificationURL string, certPEM, keyPEM []byte) (*HTTPWriter, error) {
	parsedURL, err := url.ParseRequestURI(notificationURL)
	if err != nil {
		return nil, fmt.Errorf("parsing notification URL: %w", err)
	}
	if parsedURL.Scheme != "https" {
		return nil, fmt.Errorf("notification URL must use HTTPS scheme, got %q", parsedURL.Scheme)
	}
	if parsedURL.Host == "" {
		return nil, fmt.Errorf("notification URL must specify a host")
	}

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("loading notification service client certificate: %w", err)
	}

	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSClientConfig: &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS12,
		},
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	return &HTTPWriter{
		url:    notificationURL,
		client: &http.Client{Transport: transport, Timeout: 30 * time.Second},
	}, nil
}

func (w *HTTPWriter) Write(ctx context.Context, data []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("building notification request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := w.client.Do(req)
	if err != nil {
		return fmt.Errorf("sending notification: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("notification service returned status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

// NoopWriter discards notifications. It is used when notifications are
// disabled via configuration.
type NoopWriter struct{}

func NewNoopWriter() *NoopWriter { return &NoopWriter{} }

func (w *NoopWriter) Write(_ context.Context, _ []byte) error { return nil }

// StdoutWriter prints notifications to stdout. It is used as a fallback
// when the notification service's mTLS client certificate is not configured.
type StdoutWriter struct{}

func NewStdoutWriter() *StdoutWriter { return &StdoutWriter{} }

func (w *StdoutWriter) Write(_ context.Context, data []byte) error {
	fmt.Println(string(data))
	return nil
}
