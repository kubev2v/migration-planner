package e2e_utils

import (
	"fmt"
	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/cli"
	. "github.com/kubev2v/migration-planner/test/e2e"
	"os"
)

// GetToken retrieves the private key from the specified path, parses it, and then generates a token
// for the given credentials using the private key. Returns the token or an error.
func GetToken(credentials *auth.User) (string, error) {
	privateKeyString, err := os.ReadFile(PrivateKeyPath)
	if err != nil {
		return "", fmt.Errorf("error, unable to read the private key: %v", err)
	}

	privateKey, err := cli.ParsePrivateKey(string(privateKeyString))
	if err != nil {
		return "", fmt.Errorf("error with parsing the private key: %v", err)
	}

	token, err := cli.GenerateToken(credentials.Username, credentials.Organization, privateKey)
	if err != nil {
		return "",
			fmt.Errorf("error, unable to generate token: %v for user: %s, org: %s",
				err, credentials.Username, credentials.Organization)
	}

	return token, nil
}

// UserAuth returns an auth.User object with the provided username and organization.
func UserAuth(user string, org string) *auth.User {
	return &auth.User{
		Username:     user,
		Organization: org,
	}
}

// DefaultUserAuth returns an auth.User object with the default username and organization.
func DefaultUserAuth() *auth.User {
	return UserAuth(DefaultUsername, DefaultOrganization)
}
