package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kubev2v/migration-planner/internal/service"
	"github.com/kubev2v/migration-planner/internal/service/mappers"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/internal/store/model"
	"github.com/kubev2v/migration-planner/pkg/duckdb_parser"
	"github.com/kubev2v/migration-planner/pkg/log"
)

var riverQueueRegex = regexp.MustCompile(`^[a-z0-9]+([_-]?[a-z0-9]+)*$`)

type recordingJobStore struct {
	store.Job
	metadata model.RVToolsJobMetadata
}

func (s *recordingJobStore) UpdateMetadata(_ context.Context, _ int64, data []byte) error {
	return json.Unmarshal(data, &s.metadata)
}

type workerStore struct {
	store.Store
	jobs *recordingJobStore
}

func (s *workerStore) Job() store.Job { return s.jobs }

func TestRVToolsWorkerReadableErrors(t *testing.T) {
	for _, tc := range []struct {
		name          string
		content       []byte
		sheet         string
		truncate      bool
		missingColumn bool
		row           []string
		want          string
	}{
		{name: "JSON", content: []byte(`{"VM":"vm-1"}`), want: "The file is corrupted or not a valid Excel file"},
		{name: "text", content: []byte("not an Excel workbook"), want: "The file is corrupted or not a valid Excel file"},
		{name: "empty", content: []byte{}, want: "The file is corrupted or not a valid Excel file"},
		{name: "truncated", truncate: true, want: "The file is corrupted or not a valid Excel file"},
		{name: "missing sheet", sheet: "vHost", want: "File is not a valid RVTools export (missing required 'vInfo' sheet)"},
		{name: "no VMs", want: "No VMs found in vInfo sheet - cannot build inventory without VM data"},
		{name: "missing column", missingColumn: true, want: duckdb_parser.RVToolsFailureMessage},
		{
			name:          "readable and technical validation errors",
			missingColumn: true,
			row:           []string{"", "cluster-1", "uuid"},
			want:          duckdb_parser.RVToolsFailureMessage,
		},
		{
			name: "multiple validation errors",
			row:  []string{"", "", "cluster-1", "550e8400-e29b-41d4-a716-446655440000"},
			want: "'VM ID' column is missing or empty in the vInfo sheet - VMs cannot be identified\n'VM' column is missing or empty in the vInfo sheet - VMs cannot be identified",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "rvtools.xlsx")
			book := excelize.NewFile()
			defer func() { require.NoError(t, book.Close()) }()
			sheet := tc.sheet
			if sheet == "" {
				sheet = "vInfo"
			}
			require.NoError(t, book.SetSheetName("Sheet1", sheet))
			headers := []string{"VM ID", "VM", "Cluster", "VI SDK UUID"}
			if tc.missingColumn {
				headers = headers[1:]
			}
			require.NoError(t, book.SetSheetRow(sheet, "A1", &headers))
			if tc.missingColumn {
				row := []string{"vm-1", "cluster-1", "uuid"}
				require.NoError(t, book.SetSheetRow(sheet, "A2", &row))
			}
			if tc.row != nil {
				require.NoError(t, book.SetSheetRow(sheet, "A2", &tc.row))
			}
			require.NoError(t, book.SaveAs(path))
			if tc.content != nil {
				require.NoError(t, os.WriteFile(path, tc.content, 0600))
			}
			if tc.truncate {
				data, err := os.ReadFile(path)
				require.NoError(t, err)
				require.NoError(t, os.Truncate(path, int64(len(data)/2)))
			}
			jobs := &recordingJobStore{}
			worker := NewRVToolsWorker(&workerStore{jobs: jobs}, nil, nil)
			err := worker.Work(context.Background(), &river.Job[model.RVToolsJobArgs]{
				JobRow: &rivertype.JobRow{ID: 1},
				Args:   model.RVToolsJobArgs{Name: "assessment: with a colon", File: path},
			})
			require.Error(t, err)
			var processing *duckdb_parser.RVToolsProcessingError
			require.ErrorAs(t, err, &processing)
			require.Equal(t, duckdb_parser.RVToolsStageParsing, processing.Stage)
			require.Equal(t, path, processing.Filename)

			require.Equal(t, model.JobStatusFailed, jobs.metadata.Status)
			require.Equal(t, tc.want, jobs.metadata.Error)
			form := mappers.JobForm{Metadata: jobs.metadata}
			apiJob := form.ToAPIJob()
			require.NotNil(t, apiJob.Error)
			require.Equal(t, jobs.metadata.Error, *apiJob.Error)
			_, err = os.Stat(path)
			require.ErrorIs(t, err, os.ErrNotExist, "worker must remove rejected uploads")
		})
	}
}

func TestSanitizeQueueName(t *testing.T) {
	tests := []struct {
		hostname string
		want     string
	}{
		{"simple-pod", "rvtools-simple-pod"},
		{"pod.namespace.svc.cluster.local", "rvtools-pod-namespace-svc-cluster-local"},
		{"MyHost.Example.COM", "rvtools-myhost-example-com"},
		{"host_name", "rvtools-host-name"},
		{"host:8080", "rvtools-host-8080"},
		{"pod@node!#1", "rvtools-pod-node-1"},
		{".leading.dot", "rvtools-leading-dot"},
		{"trailing.", "rvtools-trailing"},
		{"host-.example.com", "rvtools-host-example-com"},
		{"...", "default"},
		{"@#$", "default"},
	}

	for _, tt := range tests {
		t.Run(tt.hostname, func(t *testing.T) {
			got := sanitizeQueueName(tt.hostname)
			if got != tt.want {
				t.Errorf("sanitizeQueueName(%q) = %q, want %q", tt.hostname, got, tt.want)
			}
			if !riverQueueRegex.MatchString(got) {
				t.Errorf("sanitizeQueueName(%q) = %q, does not match River queue regex", tt.hostname, got)
			}
		})
	}
}

func TestRVToolsWorkerLogsCauseAndStoresSafeMessage(t *testing.T) {
	core, logs := observer.New(zap.ErrorLevel)
	restore := zap.ReplaceGlobals(zap.New(core))
	defer restore()
	jobs := &recordingJobStore{}
	worker := NewRVToolsWorker(&workerStore{jobs: jobs}, nil, nil)
	cause := errors.New("SELECT private_column FROM /private/inventory.duckdb")
	original := fmt.Errorf("saving assessment: %w", cause)
	logger := log.NewDebugLogger("test").Operation("fail").Build()
	err := worker.failJob(context.Background(), logger, 1, "save", original, duckdb_parser.RVToolsFailureMessage)
	require.ErrorIs(t, err, cause)
	require.Equal(t, duckdb_parser.RVToolsFailureMessage, jobs.metadata.Error)
	require.NotContains(t, jobs.metadata.Error, "/private")
	require.NotEmpty(t, logs.All())
	require.Contains(t, fmt.Sprint(logs.All()), "private_column")
	initializationErr := &duckdb_parser.RVToolsProcessingError{Stage: duckdb_parser.RVToolsStageInitialization, Cause: original}
	err = worker.failJob(context.Background(), logger, 1, "create_parser", initializationErr, "Could not start processing the RVTools file")
	require.ErrorIs(t, err, cause)
	require.Equal(t, "Could not start processing the RVTools file", jobs.metadata.Error)
	cleanupErr := &duckdb_parser.RVToolsProcessingError{Stage: duckdb_parser.RVToolsStageCleanup, Filename: "/private/upload.xlsx", Cause: original}
	err = worker.failJob(context.Background(), logger, 1, "cleanup", cleanupErr, duckdb_parser.RVToolsFailureMessage)
	require.ErrorIs(t, err, cause)
	require.Equal(t, duckdb_parser.RVToolsFailureMessage, jobs.metadata.Error)

	workbook := &duckdb_parser.XLSXError{Cause: fmt.Errorf("query /private/upload.xlsx: No xl/workbook.xml found")}
	parseErr := &duckdb_parser.RVToolsProcessingError{Stage: duckdb_parser.RVToolsStageParsing, Cause: workbook}
	require.ErrorIs(t, worker.failJob(context.Background(), logger, 1, "parse_files", parseErr, duckdb_parser.RVToolsFailureMessage), workbook)
	require.Equal(t, "The file is corrupted or not a valid Excel file", jobs.metadata.Error)
	require.Contains(t, fmt.Sprint(logs.All()), "query /private/upload.xlsx")

	cleanupErr.Cause = workbook
	require.ErrorIs(t, worker.failJob(context.Background(), logger, 1, "cleanup", cleanupErr, duckdb_parser.RVToolsFailureMessage), workbook)
	require.Equal(t, duckdb_parser.RVToolsFailureMessage, jobs.metadata.Error)

	validation := (duckdb_parser.ValidationResult{Errors: []duckdb_parser.ValidationIssue{
		{Code: duckdb_parser.CodeColumnValidationFailed, Message: "SELECT secret_column FROM private_table"},
	}}).Error()
	require.ErrorIs(t, worker.failJob(context.Background(), logger, 1, "parse_files", validation, duckdb_parser.RVToolsFailureMessage), validation)
	require.Equal(t, duckdb_parser.RVToolsFailureMessage, jobs.metadata.Error)
	require.Contains(t, fmt.Sprint(logs.All()), "secret_column")
}

func TestRVToolsWorkerDuplicateAssessmentName(t *testing.T) {
	jobs := &recordingJobStore{}
	worker := NewRVToolsWorker(&workerStore{jobs: jobs}, nil, nil)
	var cause error = service.NewErrAssessmentDuplicateName("assessment-1")
	original := fmt.Errorf("saving /private/inventory.duckdb: %w", cause)
	logger := log.NewDebugLogger("test").Operation("create_assessment").Build()
	err := worker.failJob(context.Background(), logger, 1, "create_assessment", original, "failed to create assessment")
	require.ErrorIs(t, err, cause)
	require.Equal(t, `assessment with name "assessment-1" already exists`, jobs.metadata.Error)
	form := mappers.JobForm{Metadata: jobs.metadata}
	require.Equal(t, &jobs.metadata.Error, form.ToAPIJob().Error)
}
