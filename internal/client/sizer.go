package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// SizerClient is an HTTP client for the sizer service
type SizerClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewSizerClient(baseURL string, timeout time.Duration) *SizerClient {
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	return &SizerClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

type SizerRequest struct {
	Platform    string       `json:"platform"`
	MachineSets []MachineSet `json:"machineSets"`
	Workloads   []Workload   `json:"workloads"`
	Detailed    bool         `json:"detailed,omitempty"`
}

type MachineSet struct {
	Name                    string                `json:"name"`
	CPU                     int                   `json:"cpu"`
	Memory                  int                   `json:"memory"`
	InstanceName            string                `json:"instanceName"`
	NumberOfDisks           int                   `json:"numberOfDisks"`
	OnlyFor                 []string              `json:"onlyFor,omitempty"`
	Label                   string                `json:"label,omitempty"`
	AllowWorkloadScheduling *bool                 `json:"allowWorkloadScheduling,omitempty"`
	ControlPlaneReserved    *ControlPlaneReserved `json:"controlPlaneReserved,omitempty"`
}

type ControlPlaneReserved struct {
	CPU    float64 `json:"cpu"`
	Memory float64 `json:"memory"`
}

type Workload struct {
	Name         string              `json:"name"`
	Count        int                 `json:"count"`
	UsesMachines []string            `json:"usesMachines"`
	Services     []ServiceDescriptor `json:"services"`
}

type ServiceDescriptor struct {
	Name           string   `json:"name"`
	RequiredCPU    float64  `json:"requiredCPU"`
	RequiredMemory float64  `json:"requiredMemory"`
	LimitCPU       float64  `json:"limitCPU,omitempty"`
	LimitMemory    float64  `json:"limitMemory,omitempty"`
	Zones          int      `json:"zones"`
	RunsWith       []string `json:"runsWith,omitempty"`
	Avoid          []string `json:"avoid,omitempty"`
}

type SizerResponse struct {
	Success bool      `json:"success"`
	Data    SizerData `json:"data"`
	Error   string    `json:"error,omitempty"`
}

type SizerData struct {
	NodeCount           int                 `json:"nodeCount"`
	Zones               int                 `json:"zones"`
	TotalCPU            int                 `json:"totalCPU"`
	TotalMemory         int                 `json:"totalMemory"`
	ResourceConsumption ResourceConsumption `json:"resourceConsumption"`
	Advanced            []Zone              `json:"advanced,omitempty"`
}

type ResourceConsumption struct {
	CPU             float64          `json:"cpu"`
	Memory          float64          `json:"memory"`
	Limits          *ResourceLimits  `json:"limits,omitempty"`
	OverCommitRatio *OverCommitRatio `json:"overCommitRatio,omitempty"`
}

type ResourceLimits struct {
	CPU    float64 `json:"cpu"`
	Memory float64 `json:"memory"`
}

type OverCommitRatio struct {
	CPU    float64 `json:"cpu"`
	Memory float64 `json:"memory"`
}

type Zone struct {
	Zone  string `json:"zone"`
	Nodes []Node `json:"nodes"`
}

type Node struct {
	Node           string        `json:"node"`
	MachineSet     string        `json:"machineSet"`
	IsControlPlane bool          `json:"isControlPlane"`
	Resources      NodeResources `json:"resources"`
	Services       []string      `json:"services"`
}

type NodeResources struct {
	CPU    CPUResources    `json:"cpu"`
	Memory MemoryResources `json:"memory"`
	Disks  DiskResources   `json:"disks"`
}

type CPUResources struct {
	Requested       float64 `json:"requested"`
	Total           int     `json:"total"`
	Limits          float64 `json:"limits,omitempty"`
	OverCommitRatio float64 `json:"overCommitRatio,omitempty"`
}

type MemoryResources struct {
	Requested       float64 `json:"requested"`
	Total           int     `json:"total"`
	Limits          float64 `json:"limits,omitempty"`
	OverCommitRatio float64 `json:"overCommitRatio,omitempty"`
}

type DiskResources struct {
	Used  int `json:"used"`
	Total int `json:"total"`
}

func (c *SizerClient) CalculateSizing(ctx context.Context, req *SizerRequest) (*SizerResponse, error) {
	url := fmt.Sprintf("%s/api/v1/size/custom", c.baseURL)

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to call sizer service: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sizer service returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var sizerResp SizerResponse
	if err := json.Unmarshal(bodyBytes, &sizerResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if !sizerResp.Success {
		return nil, fmt.Errorf("sizer service returned error: %s", sizerResp.Error)
	}

	return &sizerResp, nil
}

func (c *SizerClient) HealthCheck(ctx context.Context) error {
	url := fmt.Sprintf("%s/health", c.baseURL)

	httpReq, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("failed to call sizer service: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	// Drain body to enable connection reuse
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("sizer service health check returned status %d", resp.StatusCode)
	}

	return nil
}
