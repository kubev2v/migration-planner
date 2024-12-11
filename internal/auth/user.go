package auth

import (
	"context"

	"github.com/golang-jwt/jwt/v5"
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

func newContext(ctx context.Context, u User) context.Context {
	return context.WithValue(ctx, usernameKey, u)
}

type User struct {
	Username     string
	Organization string
	Token        *jwt.Token
}
