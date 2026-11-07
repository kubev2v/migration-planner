package apiserver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	api "github.com/kubev2v/migration-planner/api/v1alpha1"
)

func TestRequestErrorHandlerOutOfRangeManualCounts(t *testing.T) {
	tests := []struct {
		name      string
		container string
		field     string
		label     string
	}{
		{
			name:      "perpetual licenses",
			container: "vmwareVersionCounts",
			field:     "perpetualLicensesCount",
			label:     "Number of perpetual licenses",
		},
		{
			name:      "cores licensing",
			container: "vmwareVersionCounts",
			field:     "coresLicensingCount",
			label:     "Number of cores licensing",
		},
		{
			name:      "environments",
			container: "vmwareVersionCounts",
			field:     "environmentCount",
			label:     "Number of environments",
		},
		{
			name:      "other nodes",
			container: "vmwareVersionCounts",
			field:     "otherNodesCount",
			label:     "Number of other nodes",
		},
		{
			name:      "physical locations",
			container: "customerDetails",
			field:     "physicalLocationsCount",
			label:     "Number of physical locations/datacenters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := []byte(fmt.Sprintf(`{"%s":{"%s":1.1111111111111113e+21}}`, tt.container, tt.field))
			var requestBody api.SaveAssessmentEnhancementDataJSONRequestBody
			decodeErr := json.Unmarshal(body, &requestBody)
			if decodeErr == nil {
				t.Fatal("expected an out-of-range count to fail decoding")
			}

			request := httptest.NewRequest(http.MethodPost, "/api/v1/assessments/assessment-id/enhancement-data", nil)
			response := httptest.NewRecorder()
			requestErrorHandler(response, request, fmt.Errorf("can't decode JSON body: %w", decodeErr))

			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
			}
			if contentType := response.Header().Get("Content-Type"); contentType != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", contentType)
			}

			var got api.Error
			if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
				t.Fatalf("response body is not JSON: %v", err)
			}
			wantMessage := fmt.Sprintf("%s must be a whole number within the supported range", tt.label)
			if got.Message != wantMessage {
				t.Errorf("message = %q, want %q", got.Message, wantMessage)
			}
			if strings.Contains(response.Body.String(), "json: cannot unmarshal") {
				t.Errorf("response exposed raw JSON decoder detail: %s", response.Body.String())
			}
		})
	}
}

func TestParseStandaloneValidationError(t *testing.T) {
	tests := []struct {
		name           string
		errorMessage   string
		expectedOutput string
		expectedOK     bool
	}{
		{
			name:           "number prefix with long schema path",
			errorMessage:   `request body has an error: doesn't match schema #/components/schemas/StandaloneClusterRequirementsRequest: Error at "/totalVMs": number must be at most 10000`,
			expectedOutput: "Total VMs must be at most 10000",
			expectedOK:     true,
		},
		{
			name:           "value prefix for enum constraint",
			errorMessage:   `Error at "/cpuOverCommitRatio": value is not one of the allowed values`,
			expectedOutput: "CPU over-commit ratio is not one of the allowed values",
			expectedOK:     true,
		},
		{
			name:           "control plane enum validation",
			errorMessage:   `Error at "/controlPlaneNodeCount": value must be one of: 1, 3`,
			expectedOutput: "Control plane node count must be one of: 1, 3",
			expectedOK:     true,
		},
		{
			name:           "non-standalone field",
			errorMessage:   `Error at "/name": string does not match pattern`,
			expectedOutput: "",
			expectedOK:     false,
		},
		{
			name:           "generic API error",
			errorMessage:   "Internal server error",
			expectedOutput: "",
			expectedOK:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, ok := parseStandaloneValidationError(tt.errorMessage)

			if ok != tt.expectedOK {
				t.Errorf("parseStandaloneValidationError() ok = %v, want %v", ok, tt.expectedOK)
			}

			if output != tt.expectedOutput {
				t.Errorf("parseStandaloneValidationError() output = %q, want %q", output, tt.expectedOutput)
			}
		})
	}
}

func TestOapiErrorHandler(t *testing.T) {
	tests := []struct {
		name               string
		errorMessage       string
		statusCode         int
		expectedBody       string
		expectedStatusCode int
	}{
		{
			name:               "recognized standalone validation error",
			errorMessage:       `Error at "/totalVMs": number must be at most 10000`,
			statusCode:         http.StatusBadRequest,
			expectedBody:       "Total VMs must be at most 10000\n",
			expectedStatusCode: http.StatusBadRequest,
		},
		{
			name:               "unrecognized error falls back to API Error prefix",
			errorMessage:       "Internal server error",
			statusCode:         http.StatusInternalServerError,
			expectedBody:       "API Error: Internal server error\n",
			expectedStatusCode: http.StatusInternalServerError,
		},
		{
			name:               "non-standalone field error falls back",
			errorMessage:       `Error at "/name": string does not match pattern`,
			statusCode:         http.StatusBadRequest,
			expectedBody:       "API Error: Error at \"/name\": string does not match pattern\n",
			expectedStatusCode: http.StatusBadRequest,
		},
		{
			name:               "recognized enum constraint error",
			errorMessage:       `Error at "/cpuOverCommitRatio": value is not one of the allowed values`,
			statusCode:         http.StatusBadRequest,
			expectedBody:       "CPU over-commit ratio is not one of the allowed values\n",
			expectedStatusCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()

			oapiErrorHandler(w, tt.errorMessage, tt.statusCode)

			if w.Code != tt.expectedStatusCode {
				t.Errorf("oapiErrorHandler() status = %v, want %v", w.Code, tt.expectedStatusCode)
			}

			body := w.Body.String()
			if !strings.Contains(body, strings.TrimSpace(tt.expectedBody)) {
				t.Errorf("oapiErrorHandler() body = %q, want to contain %q", body, tt.expectedBody)
			}
		})
	}
}
