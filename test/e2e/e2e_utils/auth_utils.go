package e2e_utils

import (
	"fmt"
	"github.com/golang-jwt/jwt/v5"
	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/cli"
	. "github.com/kubev2v/migration-planner/test/e2e"
	"os"
)

func getToken(username string, organization string) (string, error) {
	privateKeyString, err := os.ReadFile(PrivateKeyPath)
	if err != nil {
		return "", fmt.Errorf("error, unable to read the private key: %v", err)
	}

	privateKey, err := cli.ParsePrivateKey(string(privateKeyString))
	if err != nil {
		return "", fmt.Errorf("error with parsing the private key: %v", err)
	}

	token, err := cli.GenerateToken(username, organization, privateKey)
	if err != nil {
		return "", fmt.Errorf("error, unable to generate token: %v", err)
	}

	return token, nil
}

func DefaultUserAuth() (*auth.User, error) {
	tokenVal, err := getToken(DefaultUsername, DefaultOrganization)
	if err != nil {
		return nil, fmt.Errorf("unable to create user: %v", err)
	}
	token := jwt.New(jwt.SigningMethodRS256)
	token.Raw = tokenVal
	return &auth.User{
		Username:     DefaultUsername,
		Organization: DefaultOrganization,
		Token:        token,
	}, nil
}
