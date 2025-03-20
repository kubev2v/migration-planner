package util

import (
	"net/http"
	"strings"
)

// This method rewrite remove /api/migration-assessment/ from path
// in case we get the request from gateway
func GatewayApiRewrite(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		prefix := "/api/migration-assessment"

		if strings.HasPrefix(r.URL.Path, prefix) {
			r.URL.Path = strings.TrimPrefix(r.URL.Path, prefix)
			if r.URL.Path == "" {
				r.URL.Path = "/"
			}
		}

		next.ServeHTTP(w, r)
	})
}
