package auth

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/http"

	"github.com/golang-jwt/jwt/v5"
	"github.com/kubev2v/migration-planner/internal/config"
	"go.uber.org/zap"
)

type Authenticator interface {
	Authenticator(next http.Handler) http.Handler
}

const (
	RHSSOAuthentication string = "rhsso"
	LocalAuthentication string = "local"
	NoneAuthentication  string = "none"
)

func NewAuthenticator(authConfig config.Auth) (Authenticator, error) {
	zap.S().Named("auth").Infow("creating authenticator", "type", authConfig.AuthenticationType)

	switch authConfig.AuthenticationType {
	case RHSSOAuthentication:
		return NewRHSSOAuthenticator(context.Background(), authConfig.JwkCertURL)
	case LocalAuthentication:
		return NewRHSSOAuthenticatorWithKeyFn(func(t *jwt.Token) (any, error) {
			if authConfig.LocalPrivateKey == "" {
				return nil, fmt.Errorf("private key is empty")
			}
			block, _ := pem.Decode([]byte(authConfig.LocalPrivateKey))
			key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
			if err != nil {
				return nil, err
			}
			return key.Public(), nil
		})
	default:
		return NewNoneAuthenticator()
	}
}
