// Package iam provides a client for the Red Hat User Service, used to resolve a
// user's organization id (org_id) from their username (SSO login/principal).
package iam

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"
)

// Client resolves user and organization information from the User Service.
type Client interface {
	// FindUser returns user info including org_id and personal details by username.
	FindUser(ctx context.Context, username string) (*UserInfo, error)

	// FindOrg retrieves organization details by org_id.
	FindOrg(ctx context.Context, orgID string) (*OrgInfo, error)
}

// HTTPClient talks to the User Service over an mTLS-authenticated HTTPS
// connection.
type HTTPClient struct {
	findUserURL    string
	findAccountURL string
	client         *http.Client
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
		findUserURL:    parsedURL.JoinPath(findUserPath).String(),
		findAccountURL: parsedURL.JoinPath(findAccountPath).String(),
		client:         &http.Client{Transport: transport, Timeout: 30 * time.Second},
	}, nil
}

func (c *HTTPClient) FindUser(ctx context.Context, username string) (*UserInfo, error) {
	if username == "" {
		return nil, fmt.Errorf("username must not be empty")
	}

	body, err := json.Marshal(findUserRequest{
		By: findUserBy{
			Authentication: findUserAuthentication{
				Principal: username,
				Provider:  authProvider,
			},
		},
		Include: findUserInclude{AllOf: []string{"relationship_summary", "personal_information"}},
	})
	if err != nil {
		return nil, fmt.Errorf("building find user request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.findUserURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("building find user request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling user service: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrOrgNotFound
	}
	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("user service returned status %d: %s", resp.StatusCode, string(msg))
	}

	var parsed findUserResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("decoding user service response: %w", err)
	}

	var orgID string
	for _, rel := range parsed.AccountRelationships {
		if rel.AccountID != "" {
			orgID = rel.AccountID
			break
		}
	}
	if orgID == "" {
		return nil, ErrOrgNotFound
	}

	return &UserInfo{
		OrgID:     orgID,
		FirstName: parsed.PersonalInformation.FirstName,
		LastName:  parsed.PersonalInformation.LastNames,
	}, nil
}

func (c *HTTPClient) FindOrg(ctx context.Context, orgID string) (*OrgInfo, error) {
	if orgID == "" {
		return nil, fmt.Errorf("orgID must not be empty")
	}

	body, err := json.Marshal(findAccountRequest{
		By: findAccountBy{
			ID: orgID,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("building find account request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.findAccountURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("building find account request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling user service: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrAccountNotFound
	}
	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("user service returned status %d: %s", resp.StatusCode, string(msg))
	}

	var parsed findAccountResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("decoding user service response: %w", err)
	}

	if parsed.Type != accountTypeOrganization {
		return nil, fmt.Errorf("account type is %q, expected %q", parsed.Type, accountTypeOrganization)
	}

	return &OrgInfo{
		ID:               parsed.ID,
		Name:             parsed.Name,
		EBSAccountNumber: parsed.EBSAccountNumber,
		Status:           parsed.Status,
		Type:             parsed.Type,
	}, nil
}
