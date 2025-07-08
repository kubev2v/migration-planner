package service

import (
	"fmt"

	"github.com/kubev2v/migration-planner/internal/service/report"
	"github.com/kubev2v/migration-planner/internal/service/report/csv"
	"github.com/kubev2v/migration-planner/internal/service/report/html"
	"github.com/kubev2v/migration-planner/internal/service/report/types"
	"github.com/kubev2v/migration-planner/internal/store/model"
)

type ReportRenderer = types.ReportRenderer
type InventoryProcessor = types.InventoryProcessor
type ReportFormat = types.ReportFormat
type ReportType = types.ReportType
type ReportOptions = types.ReportOptions
type ReportData = types.ReportData
type ProcessedMetrics = types.ProcessedMetrics
type ExecutiveMetrics = types.ExecutiveMetrics
type ResourceMetrics = types.ResourceMetrics
type ResourceDetail = types.ResourceDetail
type DistributionBucket = types.DistributionBucket
type OSDetail = types.OSDetail
type WarningDetail = types.WarningDetail
type StorageDetail = types.StorageDetail
type NetworkDetail = types.NetworkDetail
type HostDetail = types.HostDetail
type InfrastructureMetrics = types.InfrastructureMetrics
type ReportTimestamps = types.ReportTimestamps
type ReportTemplateData = types.ReportTemplateData
type EmptyReportTemplateData = types.EmptyReportTemplateData

const (
	ReportFormatCSV  = types.ReportFormatCSV
	ReportFormatHTML = types.ReportFormatHTML
)

const (
	ReportTypeSummary = types.ReportTypeSummary
)

type ReportService struct {
	processor types.InventoryProcessor
	renderers map[types.ReportFormat]types.ReportRenderer
}

func NewReportService() *ReportService {
	service := &ReportService{
		processor: report.NewStandardInventoryProcessor(),
		renderers: make(map[types.ReportFormat]types.ReportRenderer),
	}

	csvRenderer := csv.NewRenderer()
	htmlRenderer := html.NewRenderer()

	service.renderers[csvRenderer.SupportedFormat()] = csvRenderer
	service.renderers[htmlRenderer.SupportedFormat()] = htmlRenderer

	return service
}

func (r *ReportService) GenerateReport(source *model.Source, options types.ReportOptions) (string, error) {
	reportData, err := r.processor.ProcessInventory(source)
	if err != nil {
		return "", fmt.Errorf("failed to process inventory: %w", err)
	}

	reportData.Options = options

	renderer, exists := r.renderers[options.Format]
	if !exists {
		return "", fmt.Errorf("unsupported report format: %s", options.Format)
	}

	return renderer.Render(reportData)
}
