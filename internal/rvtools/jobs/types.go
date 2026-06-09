package jobs

import (
	"github.com/riverqueue/river"
)

type RVToolsJobArgs struct {
	Name      string `json:"name"`
	FilePath  string `json:"file_path"`
	OrgID     string `json:"org_id"`
	Username  string `json:"username"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}

func (RVToolsJobArgs) Kind() string {
	return "rvtools_assessment"
}

func (RVToolsJobArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue:       "default",
		MaxAttempts: 1,
	}
}
