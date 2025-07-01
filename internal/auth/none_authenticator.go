package auth

import (
	"net/http"

	"github.com/golang-jwt/jwt/v5"
)

type NoneAuthenticator struct{}

func NewNoneAuthenticator() (*NoneAuthenticator, error) {
	return &NoneAuthenticator{}, nil
}

func (n *NoneAuthenticator) Authenticator(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		user := User{
			Username:     "admin",
			Organization: "internal",
		}
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			"org_id": "internal",
			"sub":    "test-user",
		})
		token.Raw = "fake-raw-token"
		user.Token = token

		ctx := NewTokenContext(r.Context(), user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
