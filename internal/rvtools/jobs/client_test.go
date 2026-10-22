package jobs

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"

	"github.com/kubev2v/migration-planner/internal/service/mappers"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/internal/store/model"
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
		technical     bool
	}{
		{name: "JSON", content: []byte(`{"VM":"vm-1"}`), want: "zip: not a valid zip file", technical: true},
		{name: "text", content: []byte("not an Excel workbook"), want: "zip: not a valid zip file", technical: true},
		{name: "empty", content: []byte{}, want: "zip: not a valid zip file", technical: true},
		{name: "truncated", truncate: true, want: "zip: not a valid zip file", technical: true},
		{name: "missing sheet", sheet: "vHost", want: "reading vInfo headers: sheet vInfo does not exist", technical: true},
		{name: "no VMs", want: "No VMs found in vInfo sheet - cannot build inventory without VM data"},
		{name: "missing column", missingColumn: true, want: `could not validate column "VM ID":`, technical: true},
		{
			name:          "readable and technical validation errors",
			missingColumn: true,
			row:           []string{"", "cluster-1", "uuid"},
			technical:     true,
			want:          "'VM' column is missing or empty in the vInfo sheet - VMs cannot be identified",
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
			if tc.technical {
				require.ErrorContains(t, err, tc.want)
				if tc.missingColumn {
					require.ErrorContains(t, err, `could not validate column "VM ID":`)
				}
			} else {
				require.EqualError(t, err, tc.want)
			}
			require.Equal(t, model.JobStatusFailed, jobs.metadata.Status)
			require.Equal(t, err.Error(), jobs.metadata.Error)
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
