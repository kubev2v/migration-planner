package eventwrap

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/internal/store/model"
	"github.com/kubev2v/migration-planner/pkg/integrations/iam"
)

// usersByOrgID groups member usernames by their org_id. Members whose org_id is
// empty are resolved on demand via the IAM service and the resolved value is
// persisted back to the member (backfill-on-demand). Members that cannot be
// resolved are skipped and logged so a single unknown org never blocks the rest
// of the notification.
func usersByOrgID(ctx context.Context, members []model.Member, iamClient iam.Client, s store.Store) map[string][]string {
	usersByOrg := make(map[string][]string)

	for _, m := range members {
		if strings.TrimSpace(m.Username) == "" {
			zap.S().Errorw("group member with empty username", "groupId", m.GroupID.String())
			continue
		}

		orgID := m.OrgID
		if orgID == "" {
			id, err := resolveAndPersistMemberOrgID(ctx, m, iamClient, s)
			if err != nil {
				zap.S().Warnw("skipping notification for member with unresolved org_id",
					"username", m.Username, "error", err)
				continue
			}
			orgID = id
		}

		usersByOrg[orgID] = append(usersByOrg[orgID], m.Username)
	}

	return usersByOrg
}

// resolveAndPersistMemberOrgID fetches the org_id from IAM and attempts a backfill to the DB.
func resolveAndPersistMemberOrgID(ctx context.Context, m model.Member, iamClient iam.Client, s store.Store) (string, error) {
	resolved, err := iamClient.OrgIDByUsername(ctx, m.Username)
	if err != nil {
		return "", fmt.Errorf("failed to resolve org_id by username: %w", err)
	}

	if resolved == "" {
		return "", fmt.Errorf("returned empty org_id")
	}

	// Persist the resolved org_id back to the member (backfill-on-demand)
	m.OrgID = resolved
	if _, err := s.Accounts().UpdateMember(ctx, m); err != nil {
		return "", fmt.Errorf("failed to persist resolved org_id for username: %s. err: %w", m.Username, err)
	}

	return resolved, nil
}

// resolveAndPersistCustomerOrgID fetches the org_id from IAM and attempts a backfill to the DB.
func resolveAndPersistCustomerOrgID(ctx context.Context, pc *model.PartnerCustomer, iamClient iam.Client, s store.Store) (string, error) {
	resolved, err := iamClient.OrgIDByUsername(ctx, pc.Username)
	if err != nil {
		return "", fmt.Errorf("failed to resolve org_id by username: %w", err)
	}

	if resolved == "" {
		return "", fmt.Errorf("returned empty org_id")
	}

	// Persist the resolved org_id back to the PartnerCustomer (backfill-on-demand)
	pc.UsernameOrgID = resolved
	if _, err := s.PartnerCustomer().Update(ctx, *pc); err != nil {
		return "", fmt.Errorf("failed to persist resolved org_id for username: %s. err: %w", pc.Username, err)
	}

	return resolved, nil
}
