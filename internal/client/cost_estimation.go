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

// CostEstimationClient is an HTTP client for the cost-estimation service
type CostEstimationClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewCostEstimationClient(baseURL string, timeout time.Duration) *CostEstimationClient {
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	return &CostEstimationClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

type CostEstimationRequest struct {
	AssessmentID        string              `json:"assessmentId,omitempty"`
	ClusterID           string              `json:"clusterId,omitempty"`
	CustomerEnvironment CustomerEnvironment `json:"customerEnvironment"`
	Discounts           Discounts           `json:"discounts"`
}

type CustomerEnvironment struct {
	TotalEsxiHosts       int `json:"totalEsxiHosts"`
	SocketsPerHost       int `json:"socketsPerHost"`
	CoresPerSocket       int `json:"coresPerSocket"`
	TotalVirtualMachines int `json:"totalVirtualMachines"`
}

type Discounts struct {
	VcfDiscountPct    float64 `json:"vcfDiscountPct"`
	VvfDiscountPct    float64 `json:"vvfDiscountPct"`
	RedhatDiscountPct float64 `json:"redhatDiscountPct"`
	AapDiscountPct    float64 `json:"aapDiscountPct"`
}

type CostEstimationResponse struct {
	CalculatorVersion string                `json:"calculatorVersion"`
	Results           CostEstimationResults `json:"results"`
	Savings           CostEstimationSavings `json:"savings"`
}

type CostEstimationResults struct {
	VmwareVcf               CostEstimationScenario `json:"vmwareVcf"`
	VmwareVvf               CostEstimationScenario `json:"vmwareVvf"`
	OpenshiftVirtualization CostEstimationScenario `json:"openshiftVirtualization"`
}

type CostEstimationSavings struct {
	VsVcf *SavingsVsReference `json:"vsVcf,omitempty"`
	VsVvf *SavingsVsReference `json:"vsVvf,omitempty"`
}

type CostEstimationScenario struct {
	TotalThreeYearCostEstimation float64                 `json:"totalThreeYearCostEstimation"`
	Breakdown                    CostEstimationBreakdown `json:"breakdown"`
}

type CostEstimationBreakdown struct {
	SoftwareSubscriptions       float64 `json:"softwareSubscriptions"`
	AnsibleAutomationPlatform   float64 `json:"ansibleAutomationPlatform"`
	MigrationConsultingServices float64 `json:"migrationConsultingServices"`
	SwingHardwareUpgrades       float64 `json:"swingHardwareUpgrades"`
	AdditionalStorageCosts      float64 `json:"additionalStorageCosts"`
	ThirdPartyIsvCosts          float64 `json:"thirdPartyIsvCosts"`
}

type SavingsVsReference struct {
	AbsoluteThreeYearUsd float64 `json:"absoluteThreeYearUsd"`
	Percentage           float64 `json:"percentage"`
}

func (c *CostEstimationClient) CalculateCostEstimation(ctx context.Context, req *CostEstimationRequest) (*CostEstimationResponse, error) {
	url := fmt.Sprintf("%s/v1/cost-estimation", c.baseURL)

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
		return nil, fmt.Errorf("failed to call cost-estimation service: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("cost-estimation service returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var costResp CostEstimationResponse
	if err := json.Unmarshal(bodyBytes, &costResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &costResp, nil
}
