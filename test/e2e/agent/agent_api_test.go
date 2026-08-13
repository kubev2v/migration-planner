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

func TestInventory(t *testing.T) {
	var gotMethod string
	var gotPath string

	// Response shape: inventory -> UpdateInventory -> inventory (v1alpha1.Inventory).
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"inventory":{"agentId":"11111111-1111-1111-1111-111111111111","inventory":{"vcenter_id":"vc-123","clusters":{"cluster-1":{}}}}}`))
	}))
	defer server.Close()

	api := NewAgentApi(server.URL+"/", server.Client())
	inventory, err := api.Inventory()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotMethod != http.MethodGet {
		t.Errorf("expected method GET, got %s", gotMethod)
	}
	if gotPath != "/inventory" {
		t.Errorf("expected path /inventory, got %s", gotPath)
	}
	if inventory.VcenterId != "vc-123" {
		t.Errorf("expected vcenter_id %q, got %q", "vc-123", inventory.VcenterId)
	}
	if _, ok := inventory.Clusters["cluster-1"]; !ok {
		t.Errorf("expected cluster %q in inventory clusters, got %v", "cluster-1", inventory.Clusters)
	}
}

func TestInventoryMissingEnvelope(t *testing.T) {
	// An HTTP 200 with an empty object (or explicit null inventory) must be
	// rejected rather than returning a zero-valued inventory.
	for name, body := range map[string]string{
		"empty object":   `{}`,
		"null inventory": `{"inventory":null}`,
		"empty body":     ``,
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(body))
			}))
			defer server.Close()

			api := NewAgentApi(server.URL+"/", server.Client())
			if _, err := api.Inventory(); err == nil {
				t.Errorf("expected an error for %s response, got nil", name)
			}
		})
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
