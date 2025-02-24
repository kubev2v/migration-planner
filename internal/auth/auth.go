package auth

import (
	"go.uber.org/zap"
	"net/http"
)

type AuthType string

const (
	AuthTypeEmpty AuthType = ""
	AuthTypeRHSSO AuthType = "rhsso"
	AuthTypeNone  AuthType = "none"
)

type Config struct {
	AuthType   AuthType `envconfig:"MIGRATION_PLANNER_AUTH" default:""`
	JwtCertUrl string   `envconfig:"MIGRATION_PLANNER_JWK_URL" default:""`
}

type Authenticator interface {
	Authenticator(next http.Handler) http.Handler
}

func NewAuthenticator(cfg Config) (Authenticator, error) {
	zap.S().Named("auth").Infof("authentication: '%s'", cfg.AuthType)

	switch cfg.AuthType {
	case AuthTypeRHSSO:
		return NewRHSSOAuthenticator(cfg.JwtCertUrl)
	default:
		return NewNoneAuthenticator()
	}
}
