package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/MicahParks/jwkset"
	keyfunc "github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"
)

type RHSSOAuthenticator struct {
	keyFn func(t *jwt.Token) (any, error)
}

func NewRHSSOAuthenticatorWithKeyFn(keyFn func(t *jwt.Token) (any, error)) (*RHSSOAuthenticator, error) {
	return &RHSSOAuthenticator{keyFn: keyFn}, nil
}

func NewRHSSOAuthenticator(ctx context.Context, jwkCertUrl string) (*RHSSOAuthenticator, error) {
	jwks, err := jwkset.NewDefaultHTTPClient([]string{jwkCertUrl})
	if err != nil {
		return nil, fmt.Errorf("failed to create client jwk set: %s", err)
	}

	k, err := keyfunc.New(keyfunc.Options{
		Storage: jwks,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get sso public keys: %s", err)
	}

	m, err := k.Storage().Marshal(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal keys: %s", err)
	}

	for _, k := range m.Keys {
		zap.S().Debugw("key read from sso:", "alg", k.ALG.String(), "kid", k.KID)
	}

	return &RHSSOAuthenticator{keyFn: k.Keyfunc}, nil
}

func (rh *RHSSOAuthenticator) Authenticate(token string) (User, error) {
	parser := jwt.NewParser(jwt.WithValidMethods([]string{jwt.SigningMethodRS256.Name}), jwt.WithIssuedAt(), jwt.WithExpirationRequired())
	t, err := parser.Parse(token, rh.keyFn)
	if err != nil {
		zap.S().Errorw("failed to parse or the token is invalid", "token", token, "error", err)
		return User{}, fmt.Errorf("failed to authenticate token: %w", err)
	}

	if !t.Valid {
		zap.S().Errorw("failed to parse or the token is invalid", "token", token, "error", err)
		return User{}, fmt.Errorf("failed to parse or validate token")
	}

	return rh.parseToken(t)
}

func (rh *RHSSOAuthenticator) parseToken(userToken *jwt.Token) (User, error) {
	claims, ok := userToken.Claims.(jwt.MapClaims)
	if !ok {
		return User{}, errors.New("failed to parse jwt token claims")
	}

	return User{
		Username:     claims["preffered_username"].(string),
		Organization: claims["org_id"].(string),
		Token:        userToken,
	}, nil
}

func (rh *RHSSOAuthenticator) Authenticator(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		accessToken := r.Header.Get("Authorization")
		if accessToken == "" || len(accessToken) < len("Bearer ") {
			http.Error(w, "No token provided", http.StatusUnauthorized)
			return
		}

		accessToken = accessToken[len("Bearer "):]
		user, err := rh.Authenticate(accessToken)
		if err != nil {
			http.Error(w, "authentication failed", http.StatusUnauthorized)
			return
		}

		ctx := NewTokenContext(r.Context(), user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
