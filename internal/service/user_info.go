package service

import (
	"context"
	"errors"

	"github.com/golang-jwt/jwt/v5"
)

type OcmClient interface {
	GetOrganization(context context.Context, authToken string, orgID string) (string, error)
}

type UserInformationService struct {
	client OcmClient
}

func NewUserInformationService(client OcmClient) *UserInformationService {
	return &UserInformationService{client: client}
}

// NewLocalUserInformationService returns a NewLocalUserInformationService instance that can be used in local dev
func NewLocalUserInformationService() *UserInformationService {
	return &UserInformationService{client: newLocalClient()}
}

func (s *UserInformationService) GetOrganization(ctx context.Context, token *jwt.Token) (string, error) {
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", errors.New("failed to parse jwt token claims")
	}

	orgID, ok := claims["org_id"].(string)
	if !ok {
		return "", errors.New("token is missing 'org_id' claim")
	}

	org, err := s.client.GetOrganization(ctx, token.Raw, orgID)
	if err != nil {
		return "", err
	}

	return org, nil
}

type localClient struct{} // used in local dev

func newLocalClient() *localClient {
	return &localClient{}
}

func (l *localClient) GetOrganization(ctx context.Context, authToken string, orgID string) (string, error) {
	return orgID, nil
}
