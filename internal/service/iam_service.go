package service

import (
	"context"

	"github.com/kubev2v/migration-planner/pkg/integrations/iam"
)

// IAMServicer provides user and organization information from the IAM service.
type IAMServicer interface {
	// GetUserInfo resolves user info including org_id and personal details by username.
	GetUserInfo(ctx context.Context, username string) (*iam.UserInfo, error)

	// GetOrgInfo retrieves organization details by org_id.
	GetOrgInfo(ctx context.Context, orgID string) (*iam.OrgInfo, error)
}

// IAMService wraps the IAM client and provides business logic for user and org resolution.
type IAMService struct {
	client iam.Client
}

// NewIAMService creates a new IAM service with the given client.
func NewIAMService(client iam.Client) *IAMService {
	return &IAMService{client: client}
}

func (s *IAMService) GetUserInfo(ctx context.Context, username string) (*iam.UserInfo, error) {
	return s.client.FindUser(ctx, username)
}

func (s *IAMService) GetOrgInfo(ctx context.Context, orgID string) (*iam.OrgInfo, error) {
	return s.client.FindOrg(ctx, orgID)
}
