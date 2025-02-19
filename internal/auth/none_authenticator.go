package auth

import (
	"net/http"
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
		ctx := NewUserContext(r.Context(), user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
