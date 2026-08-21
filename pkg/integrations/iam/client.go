// Package iam provides a client for the Red Hat User Service, used to resolve a
// user's organization id (org_id) from their username (SSO login/principal).
package iam

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"
)

// findUserPath is the User Service endpoint that resolves a single user.
const findUserPath = "/v2/findUser"

// authProvider is the authentication provider used when looking a user up by
// login/principal. Console SSO logins are issued by "Red Hat".
const authProvider = "Red Hat"

// ErrOrgNotFound is returned when the User Service has no organization
// (account relationship) for the requested username.
var ErrOrgNotFound = errors.New("no org_id found for username")

// Client resolves organization ids from the User Service.
type Client interface {
	// OrgIDByUsername returns the org_id for the given username, or
	// ErrOrgNotFound if the user has no associated organization.
	OrgIDByUsername(ctx context.Context, username string) (string, error)
}

// NewIAMClient builds the User interface client.
// When the IAM URL or client certificate/key are unset, it returns an unimplemented client
func NewIAMClient(URL, ClientCert, ClientKey string) (Client, error) {
	if URL == "" || ClientCert == "" || ClientKey == "" {
		return &UnimplementedClient{}, fmt.Errorf("IAM client missing url, cert or key")
	}

	iamClient, err := NewHTTPClient(URL, []byte(ClientCert), []byte(ClientKey))
	if err != nil {
		return &UnimplementedClient{}, fmt.Errorf("unable to create IAM client: %w", err)
	}

	return iamClient, nil
}

// HTTPClient talks to the User Service over an mTLS-authenticated HTTPS
// connection.
type HTTPClient struct {
	url    string
	client *http.Client
}

// NewHTTPClient builds an HTTPClient that authenticates with the User Service
// using the given PEM-encoded client certificate and private key. baseURL is
// the service root (e.g. https://user.stage.api.redhat.com).
func NewHTTPClient(baseURL string, certPEM, keyPEM []byte) (*HTTPClient, error) {
	parsedURL, err := url.ParseRequestURI(baseURL)
	if err != nil {
		return nil, fmt.Errorf("parsing user service URL: %w", err)
	}
	if parsedURL.Scheme != "https" {
		return nil, fmt.Errorf("user service URL must use HTTPS scheme, got %q", parsedURL.Scheme)
	}
	if parsedURL.Host == "" {
		return nil, fmt.Errorf("user service URL must specify a host")
	}

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("loading user service client certificate: %w", err)
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

	return &HTTPClient{
		url:    parsedURL.JoinPath(findUserPath).String(),
		client: &http.Client{Transport: transport, Timeout: 30 * time.Second},
	}, nil
}

func (c *HTTPClient) OrgIDByUsername(ctx context.Context, username string) (string, error) {
	if username == "" {
		return "", fmt.Errorf("username must not be empty")
	}

	body, err := json.Marshal(findUserRequest{
		By: findUserBy{
			Authentication: findUserAuthentication{
				Principal: username,
				Provider:  authProvider,
			},
		},
		Include: findUserInclude{AllOf: []string{"relationship_summary"}},
	})
	if err != nil {
		return "", fmt.Errorf("building find user request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("building find user request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("calling user service: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode == http.StatusNotFound {
		return "", ErrOrgNotFound
	}
	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return "", fmt.Errorf("user service returned status %d: %s", resp.StatusCode, string(msg))
	}

	var parsed findUserResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return "", fmt.Errorf("decoding user service response: %w", err)
	}

	for _, rel := range parsed.AccountRelationships {
		if rel.AccountID != "" {
			return rel.AccountID, nil
		}
	}
	return "", ErrOrgNotFound
}

type UnimplementedClient struct{}

func (c *UnimplementedClient) OrgIDByUsername(_ context.Context, _ string) (string, error) {
	return "", ErrOrgNotFound
}

// findUserRequest is the POST /v2/findUser body. We look the user up by their
// SSO login/principal and ask only for the relationship summary, which carries
// the accountId we treat as the org_id.
type findUserRequest struct {
	By      findUserBy      `json:"by"`
	Include findUserInclude `json:"include"`
}

type findUserBy struct {
	Authentication findUserAuthentication `json:"authentication"`
}

type findUserAuthentication struct {
	Principal string `json:"principal"`
	Provider  string `json:"provider"`
}

type findUserInclude struct {
	AllOf []string `json:"allOf"`
}

// findUserResponse is the subset of the /v2/findUser response we consume.
// accountRelationships[0].accountId is the org_id.
type findUserResponse struct {
	AccountRelationships []accountRelationship `json:"accountRelationships"`
}

type accountRelationship struct {
	AccountID string `json:"accountId"`
}
