package jobs

import (
	"github.com/riverqueue/river"
)

// RVToolsJobArgs contains the arguments for an RVTools assessment job.
// This is stored in river_job.args as JSON.
type RVToolsJobArgs struct {
	Name        string `json:"name"`
	FileContent []byte `json:"file_content"`
	OrgID       string `json:"org_id"`
	Username    string `json:"username"`
	FirstName   string `json:"first_name"`
	LastName    string `json:"last_name"`
}

// Kind returns the job kind for River registration.
func (RVToolsJobArgs) Kind() string {
	return "rvtools_assessment"
}

// InsertOpts returns the default insert options for this job type.
func (RVToolsJobArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue:       "default",
		MaxAttempts: 1,
	}
}
