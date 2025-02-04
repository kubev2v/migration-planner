package auth

import (
	"context"

	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"
)

type usernameKeyType struct{}

var (
	usernameKey usernameKeyType
)

func UserFromContext(ctx context.Context) (User, bool) {
	val := ctx.Value(usernameKey)
	if val == nil {
		return User{}, false
	}
	return val.(User), true
}

func MustHaveUser(ctx context.Context) User {
	user, found := UserFromContext(ctx)
	if !found {
		zap.S().Named("auth").Panicf("failed to find user in context")
	}
	return user
}

func NewUserContext(ctx context.Context, u User) context.Context {
	return context.WithValue(ctx, usernameKey, u)
}

type User struct {
	Username     string
	Organization string
	Token        *jwt.Token
}
