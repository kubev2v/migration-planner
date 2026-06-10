package auth

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNoneAgentAuthenticator_SourceIDExtraction(t *testing.T) {
	tests := []struct {
		name             string
		method           string
		path             string
		body             map[string]interface{}
		expectedSourceID string
		description      string
	}{
		{
			name:   "Extract from new PUT /sources/{id} endpoint",
			method: http.MethodPut,
			path:   "/api/v1/sources/693a5630-664f-4415-b503-dbdec31fbf36",
			body: map[string]interface{}{
				"inventory": map[string]interface{}{
					"vcenter_id": "test",
				},
			},
			expectedSourceID: "693a5630-664f-4415-b503-dbdec31fbf36",
			description:      "New main inventory endpoint without trailing slash should extract source ID from URL path",
		},
		{
			name:   "Extract from old PUT /sources/{id}/status endpoint",
			method: http.MethodPut,
			path:   "/api/v1/sources/693a5630-664f-4415-b503-dbdec31fbf36/status",
			body: map[string]interface{}{
				"agentId": "agent-123",
				"inventory": map[string]interface{}{
					"vcenter_id": "test",
				},
			},
			expectedSourceID: "693a5630-664f-4415-b503-dbdec31fbf36",
			description:      "Old status endpoint with trailing /status should extract source ID from URL path",
		},
		{
			name:   "Extract from PUT /sources/{id}/subset/{subsetId} endpoint",
			method: http.MethodPut,
			path:   "/api/v1/sources/693a5630-664f-4415-b503-dbdec31fbf36/subset/abc-123",
			body: map[string]interface{}{
				"name": "test-subset",
				"inventory": map[string]interface{}{
					"vcenter_id": "test",
				},
			},
			expectedSourceID: "693a5630-664f-4415-b503-dbdec31fbf36",
			description:      "Subset endpoint should extract source ID from URL path",
		},
		{
			name:             "Extract from DELETE /sources/{id}/subset/{subsetId} endpoint",
			method:           http.MethodDelete,
			path:             "/api/v1/sources/693a5630-664f-4415-b503-dbdec31fbf36/subset/abc-123",
			body:             nil,
			expectedSourceID: "693a5630-664f-4415-b503-dbdec31fbf36",
			description:      "DELETE subset endpoint should extract source ID from URL path",
		},
		{
			name:   "Extract from body sourceId field if present",
			method: http.MethodPut,
			path:   "/api/v1/agents/agent-123/status",
			body: map[string]interface{}{
				"sourceId": "source-from-body-456",
				"status":   "active",
			},
			expectedSourceID: "source-from-body-456",
			description:      "Should prefer sourceId from request body when present",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create request body
			var bodyReader io.Reader
			if tt.body != nil {
				bodyBytes, err := json.Marshal(tt.body)
				require.NoError(t, err)
				bodyReader = bytes.NewReader(bodyBytes)
			} else {
				bodyReader = bytes.NewReader([]byte{})
			}

			// Create test request with the specified HTTP method
			req := httptest.NewRequest(tt.method, tt.path, bodyReader)
			rec := httptest.NewRecorder()

			// Create authenticator
			auth := NewNoneAgentAuthenticator()

			// Create a test handler that extracts the AgentJWT from context
			var capturedSourceID string
			testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				agentJWT, found := AgentFromContext(r.Context())
				if found {
					capturedSourceID = agentJWT.SourceID
				}
				w.WriteHeader(http.StatusOK)
			})

			// Execute
			auth.Authenticator(testHandler).ServeHTTP(rec, req)

			// Assert
			assert.Equal(t, http.StatusOK, rec.Code, "Handler should succeed")
			assert.Equal(t, tt.expectedSourceID, capturedSourceID, tt.description)
		})
	}
}

func TestSourcePathRegex(t *testing.T) {
	tests := []struct {
		path            string
		shouldMatch     bool
		expectedCapture string
	}{
		{
			path:            "/api/v1/sources/693a5630-664f-4415-b503-dbdec31fbf36",
			shouldMatch:     true,
			expectedCapture: "693a5630-664f-4415-b503-dbdec31fbf36",
		},
		{
			path:            "/api/v1/sources/693a5630-664f-4415-b503-dbdec31fbf36/status",
			shouldMatch:     true,
			expectedCapture: "693a5630-664f-4415-b503-dbdec31fbf36",
		},
		{
			path:            "/api/v1/sources/693a5630-664f-4415-b503-dbdec31fbf36/subset/abc",
			shouldMatch:     true,
			expectedCapture: "693a5630-664f-4415-b503-dbdec31fbf36",
		},
		{
			path:        "/api/v1/agents/123",
			shouldMatch: false,
		},
		{
			path:        "/api/v1/sources",
			shouldMatch: false,
		},
		{
			path:        "/api/v1/sources/",
			shouldMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			matches := sourcePathRe.FindStringSubmatch(tt.path)

			if tt.shouldMatch {
				require.True(t, len(matches) > 1, "Expected regex to match path: %s", tt.path)
				assert.Equal(t, tt.expectedCapture, matches[1], "Captured source ID should match")
			} else {
				assert.True(t, len(matches) <= 1, "Expected regex NOT to match path: %s", tt.path)
			}
		})
	}
}
