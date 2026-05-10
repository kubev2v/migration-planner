package middleware

import (
	"context"
	"net/http"
)

type Key int

const (
	// ResponseWriterKey to store the ResponseWriter in the context of openapi
	ResponseWriterKey Key = 0
	// RequestKey to store the *http.Request in the context (needed for http.ServeContent)
	RequestKey Key = 1
)

// RequestContext Middleware to inject ResponseWriter and *http.Request into context.
// The Request is needed by http.ServeContent for handling byte-range requests.
func RequestContext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), ResponseWriterKey, w)
		ctx = context.WithValue(ctx, RequestKey, r)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
