package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

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
	var user User
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

	if user, err = rh.parseToken(t); err != nil {
		zap.S().Errorw("failed to parse or the token is invalid", "token", token, "error", err)
		return User{}, fmt.Errorf("failed to authenticate token: %w", err)
	}

	return user, nil
}

func (rh *RHSSOAuthenticator) parseToken(userToken *jwt.Token) (user User, err error) {
	claims, ok := userToken.Claims.(jwt.MapClaims)
	if !ok {
		return User{}, errors.New("failed to parse jwt token claims")
	}

	// recover from panic if any of the claims are missing
	defer func() {
		if r := recover(); r != nil {
			user = User{}
			err = fmt.Errorf("failed to parse token: %v", claims)
		}
	}()

	orgID, err := rh.getOrgID(claims)
	if err != nil {
		return User{}, err
	}

	username := claims["username"].(string)
	domain := ""
	if email, ok := claims["email"].(string); ok {
		if email != "" {
			parts := strings.Split(email, "@")
			if len(parts) == 2 {
				domain = parts[1]
			}
		}
	}

	return User{
		Username:     username,
		Organization: orgID,
		EmailDomain:  domain,
		Token:        userToken,
	}, nil
}

func (rh *RHSSOAuthenticator) Authenticator(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		accessToken := r.Header.Get("X-Authorization")
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

func (rh *RHSSOAuthenticator) getOrgID(claims jwt.MapClaims) (string, error) {
	if v, found := claims["org_id"]; found {
		if orgID, ok := v.(string); ok {
			return orgID, nil
		}
	}

	if v, found := claims["organization"]; found {
		if orgMap, ok := v.(map[string]any); ok {
			if orgID, found := orgMap["id"]; found {
				if orgIDStr, ok := orgID.(string); ok {
					return orgIDStr, nil
				}
			}
		}
	}

	// get orgID from username if possible
	username := claims["username"].(string)
	if strings.Contains(username, "@") {
		orgID := strings.Split(username, "@")[1]
		if strings.TrimSpace(orgID) == "" {
			return "", fmt.Errorf("preferred_username %q is malformatted", username)
		}

		return orgID, nil
	}

	return "", fmt.Errorf("organization ID not found in claims")
}
