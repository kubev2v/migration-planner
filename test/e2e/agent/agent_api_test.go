package agent

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPutCredentials(t *testing.T) {
	var gotMethod string
	var gotPath string
	var gotBody CredentialsRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	api := NewAgentApi(server.URL+"/", server.Client())
	statusCode, err := api.PutCredentials("https://vcenter.example.com", "admin", "secret")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotMethod != http.MethodPut {
		t.Errorf("expected method PUT, got %s", gotMethod)
	}
	if gotPath != "/credentials" {
		t.Errorf("expected path /credentials, got %s", gotPath)
	}
	if statusCode != http.StatusNoContent {
		t.Errorf("expected status %d, got %d", http.StatusNoContent, statusCode)
	}
	if gotBody.URL != "https://vcenter.example.com" {
		t.Errorf("expected URL %q, got %q", "https://vcenter.example.com", gotBody.URL)
	}
	if gotBody.Username != "admin" {
		t.Errorf("expected username %q, got %q", "admin", gotBody.Username)
	}
	if gotBody.Password != "secret" {
		t.Errorf("expected password %q, got %q", "secret", gotBody.Password)
	}
}

func TestStartCollector(t *testing.T) {
	var gotMethod string
	var gotPath string
	var gotBodyBytes []byte

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotBodyBytes, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"collected"}`))
	}))
	defer server.Close()

	api := NewAgentApi(server.URL+"/", server.Client())
	status, statusCode, err := api.StartCollector()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Errorf("expected method POST, got %s", gotMethod)
	}
	if gotPath != "/collector" {
		t.Errorf("expected path /collector, got %s", gotPath)
	}
	if len(gotBodyBytes) != 0 {
		t.Errorf("expected empty request body, got %q", string(gotBodyBytes))
	}
	if statusCode != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, statusCode)
	}
	if status.Status != "collected" {
		t.Errorf("expected status %q, got %q", "collected", status.Status)
	}
}
