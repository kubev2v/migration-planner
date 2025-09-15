package middleware

import (
	"net/http"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/kubev2v/migration-planner/pkg/requestid"
)

// RequestID gets the request ID from the x-request-id header or generates
// a unique request ID for each HTTP request and injects it into the
// request's context.Context for consistent access across the application layer.
// This middleware enhances Chi's built-in RequestID middleware by ensuring
// request IDs are available via our requestid package.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// First try to get request ID from x-request-id header
		requestID := r.Header.Get("x-request-id")

		// If no header provided, check if Chi already generated one
		if requestID == "" {
			requestID = middleware.GetReqID(r.Context())
		}

		// If still no request ID, generate a new UUID
		if requestID == "" {
			requestID = requestid.Generate()
		}

		// Create new context with requestID and replace request context
		ctx := requestid.ToContext(r.Context(), requestID)
		r = r.WithContext(ctx)

		next.ServeHTTP(w, r)
	})
}

// GetRequestIDFromRequest extracts the request ID from the HTTP request.
// Returns empty string if request ID is not found.
func GetRequestIDFromRequest(r *http.Request) string {
	return requestid.FromRequest(r)
}
