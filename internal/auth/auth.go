package auth

import (
	"net/http"

	"github.com/kubev2v/migration-planner/internal/config"
	"go.uber.org/zap"
)

type Authenticator interface {
	Authenticator(next http.Handler) http.Handler
}

const (
	RHSSOAuthentication string = "rhsso"
	NoneAuthentication  string = "none"
)

func NewAuthenticator(authConfig config.Auth) (Authenticator, error) {
	zap.S().Named("auth").Infof("authentication: '%s'", authConfig.AuthenticationType)

	switch authConfig.AuthenticationType {
	case RHSSOAuthentication:
		return NewRHSSOAuthenticator(authConfig.JwtCertUrl)
	default:
		return NewNoneAuthenticator()
	}
}
