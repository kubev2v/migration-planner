package service

import (
	"context"

	"github.com/kubev2v/migration-planner/pkg/integrations/iam"
	"github.com/kubev2v/migration-planner/pkg/metrics"
)

// IAMServicer provides user and account information from the IAM service.
type IAMServicer interface {
	// GetUserInfo resolves user info including org_id and personal details by username.
	GetUserInfo(ctx context.Context, username string) (*iam.UserInfo, error)

	// GetAccountInfo retrieves account details by account ID.
	GetAccountInfo(ctx context.Context, accountID string) (*iam.AccountInfo, error)
}

// IAMService wraps the IAM client and provides user and account resolution.
type IAMService struct {
	client iam.Client
}

// NewIAMService creates a new IAM service with the given client.
func NewIAMService(client iam.Client) *IAMService {
	return &IAMService{client: client}
}

func (s *IAMService) GetUserInfo(ctx context.Context, username string) (*iam.UserInfo, error) {
	info, err := s.client.FindUser(ctx, username)
	result := metrics.IntegrationResultSuccess
	if err != nil {
		result = metrics.IntegrationResultFailure
	}
	metrics.RecordIntegrationResult("iam", result)
	return info, err
}

func (s *IAMService) GetAccountInfo(ctx context.Context, accountID string) (*iam.AccountInfo, error) {
	info, err := s.client.FindAccount(ctx, accountID)
	result := metrics.IntegrationResultSuccess
	if err != nil {
		result = metrics.IntegrationResultFailure
	}
	metrics.RecordIntegrationResult("iam", result)
	return info, err
}
