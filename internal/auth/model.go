package auth

import (
	"context"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"
)

type tokenKeyType struct{}

var (
	tokenKey tokenKeyType
)

func UserFromContext(ctx context.Context) (User, bool) {
	val := ctx.Value(tokenKey)
	if val == nil {
		return User{}, false
	}
	return val.(User), true
}

func AgentFromContext(ctx context.Context) (AgentJWT, bool) {
	val := ctx.Value(tokenKey)
	if val == nil {
		return AgentJWT{}, false
	}
	return val.(AgentJWT), true
}

func MustHaveUser(ctx context.Context) User {
	user, found := UserFromContext(ctx)
	if !found {
		zap.S().Named("auth").Panic("failed to find user in context")
	}
	return user
}

func MustHaveAgent(ctx context.Context) AgentJWT {
	agentJwt, found := AgentFromContext(ctx)
	if !found {
		zap.S().Named("auth").Panic("failed to find agent jwt in context")
	}
	return agentJwt
}

func NewTokenContext(ctx context.Context, t any) context.Context {
	return context.WithValue(ctx, tokenKey, t)
}

type User struct {
	Username     string
	Organization string
	Token        *jwt.Token
}

type AgentJWT struct {
	ExpireAt time.Time `json:"exp"`
	IssueAt  time.Time `json:"iat"`
	Issuer   string    `json:"iss"`
	OrgID    string    `json:"sub"`
	SourceID string    `json:"sourceID"`
}
