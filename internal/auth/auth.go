package auth

import (
	"net/http"

	"github.com/kubev2v/migration-planner/internal/config"
)

type Authenticator interface {
	Authenticator(next http.Handler) http.Handler
}

const (
	RHSSOAuthentication string = "rhsso"
	NoneAuthentication  string = "none"
)

func NewAuthenticator(authConfig config.Auth) (Authenticator, error) {
	switch authConfig.AuthenticationType {
	case RHSSOAuthentication:
		return NewRHSSOAuthenticator(authConfig.JwtCertUrl)
	default:
		return NewNoneAuthenticator()
	}
}
