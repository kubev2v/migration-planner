package eventwrap

import (
	"github.com/kubev2v/migration-planner/internal/store/model"
)

// usersByOrgID groups member usernames by their org_id
func usersByOrgID(members []model.Member) map[string][]string {
	usersByOrg := make(map[string][]string)

	for _, m := range members {
		usersByOrg[m.OrgID] = append(usersByOrg[m.OrgID], m.Username)
	}

	return usersByOrg
}
