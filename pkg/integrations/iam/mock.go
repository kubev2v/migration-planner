package iam

import "context"

// MockClient is a mock implementation of Client for testing.
type MockClient struct {
	FindUserFunc func(ctx context.Context, username string) (*UserInfo, error)
	FindOrgFunc  func(ctx context.Context, orgID string) (*OrgInfo, error)
}

func (m *MockClient) FindUser(_ context.Context, username string) (*UserInfo, error) {
	// return a fake user with org_id
	return &UserInfo{
		OrgID:     "test-org",
		FirstName: "Test",
		LastName:  "User",
	}, nil
}

func (m *MockClient) FindOrg(_ context.Context, orgID string) (*OrgInfo, error) {
	// return a fake org
	return &OrgInfo{
		ID:               orgID,
		Name:             "Test Organization",
		EBSAccountNumber: "12345",
		Status:           "enabled",
		Type:             accountTypeOrganization,
	}, nil
}

// UnimplementedClient is a no-op client that always returns errors.
type UnimplementedClient struct{}

func (c *UnimplementedClient) FindUser(_ context.Context, _ string) (*UserInfo, error) {
	return nil, ErrOrgNotFound
}

func (c *UnimplementedClient) FindOrg(_ context.Context, _ string) (*OrgInfo, error) {
	return nil, ErrAccountNotFound
}
