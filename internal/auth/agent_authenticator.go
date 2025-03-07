package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/internal/store/model"
	"go.uber.org/zap"
)

const (
	defaultExpirationPeriod = 3 * 30 * 24 // 3 months
)

func GenerateAgentJWTAndKey(source *model.Source) (*model.Key, string, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, "", fmt.Errorf("failed to generate agent private key: %s", err)
	}

	kid := uuid.NewString()
	key := &model.Key{
		ID:         kid,
		OrgID:      source.OrgID,
		PrivateKey: privateKey,
	}

	token, err := GenerateAgentJWT(key, source)
	if err != nil {
		return nil, "", err
	}

	return key, token, nil
}

func GenerateAgentJWT(signingKey *model.Key, source *model.Source) (string, error) {
	type jwtToken struct {
		SourceID string `json:"source_id"`
		jwt.RegisteredClaims
	}

	// Create claims with multiple fields populated
	claims := jwtToken{
		source.ID.String(),
		jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(defaultExpirationPeriod * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Issuer:    "assisted-migrations",
			Subject:   source.OrgID,
			ID:        "1",
			Audience:  []string{"assisted-migrations"},
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = signingKey.ID
	signedToken, err := token.SignedString(signingKey.PrivateKey)
	if err != nil {
		return "", fmt.Errorf("failed to signed agent token: %s", err)
	}

	return signedToken, nil
}

type AgentAuthenticator struct {
	store store.Store
}

func NewAgentAuthenticator(store store.Store) *AgentAuthenticator {
	return &AgentAuthenticator{store: store}
}

func (aa *AgentAuthenticator) Authenticate(token string) (AgentJWT, error) {
	parser := jwt.NewParser(jwt.WithValidMethods([]string{jwt.SigningMethodRS256.Name}), jwt.WithIssuedAt(), jwt.WithExpirationRequired())
	t, err := parser.Parse(token, func(t *jwt.Token) (interface{}, error) {
		kid, ok := t.Header["kid"].(string)
		if !ok {
			return nil, errors.New("kid not found")
		}

		publicKeys, err := aa.store.PrivateKey().GetPublicKeys(context.Background())
		if err != nil {
			return nil, err
		}

		pb, found := publicKeys[kid]
		if !found {
			return nil, fmt.Errorf("public key not found with id: %s", kid)
		}

		rsaPublicKey := pb.(rsa.PublicKey)
		return &rsaPublicKey, nil
	})
	if err != nil {
		zap.S().Errorw("failed to parse or the token is invalid", "token", token)
		return AgentJWT{}, fmt.Errorf("failed to authenticate token: %w", err)
	}

	if !t.Valid {
		zap.S().Errorw("failed to parse or the token is invalid", "token", token)
		return AgentJWT{}, fmt.Errorf("failed to parse or validate token")
	}

	return aa.parseToken(t)
}

func (aa *AgentAuthenticator) Authenticator(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		accessToken := r.Header.Get("Authorization")
		if accessToken == "" || len(accessToken) < len("Bearer ") {
			http.Error(w, "No token provided", http.StatusUnauthorized)
			return
		}

		accessToken = accessToken[len("Bearer "):]
		agentJwt, err := aa.Authenticate(accessToken)
		if err != nil {
			http.Error(w, "authentication failed", http.StatusUnauthorized)
			return
		}

		ctx := NewTokenContext(r.Context(), agentJwt)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (aa *AgentAuthenticator) parseToken(userToken *jwt.Token) (AgentJWT, error) {
	claims, ok := userToken.Claims.(jwt.MapClaims)
	if !ok {
		return AgentJWT{}, errors.New("failed to parse jwt token claims")
	}

	return AgentJWT{
		ExpireAt: time.Unix(int64(claims["exp"].(float64)), 0),
		IssueAt:  time.Unix(int64(claims["iat"].(float64)), 0),
		SourceID: claims["source_id"].(string),
		OrgID:    claims["sub"].(string),
		Issuer:   claims["iss"].(string),
	}, nil
}
