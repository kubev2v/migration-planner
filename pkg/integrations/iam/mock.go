package iam

import "context"

// MockClient is a mock implementation of Client for testing.
type MockClient struct {
}

func (m *MockClient) FindUser(_ context.Context, username string) (*UserInfo, error) {
	// return a fake user with org_id
	return &UserInfo{
		OrgID:     "test-org",
		FirstName: "Test",
		LastName:  "User",
	}, nil
}

func (m *MockClient) FindAccount(ctx context.Context, accountID string) (*AccountInfo, error) {
	// return a fake account
	return &AccountInfo{
		ID:               accountID,
		Name:             "Test Organization",
		EBSAccountNumber: "12345",
		Status:           "enabled",
		Type:             AccountTypeOrganization,
	}, nil
}

// UnimplementedClient is a no-op client that always returns errors.
type UnimplementedClient struct{}

func (c *UnimplementedClient) FindUser(_ context.Context, _ string) (*UserInfo, error) {
	return nil, ErrOrgNotFound
}

func (c *UnimplementedClient) FindAccount(_ context.Context, _ string) (*AccountInfo, error) {
	return nil, ErrAccountNotFound
}
