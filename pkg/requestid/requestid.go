package requestid

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

type contextKey string

const RequestIDKey contextKey = "request_id"

// Generate creates a new unique request ID
func Generate() string {
	return uuid.New().String()
}

// ToContext adds a request ID to the context
func ToContext(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, RequestIDKey, requestID)
}

// FromContext extracts the request ID from the context.
// Returns empty string if request ID is not found.
func FromContext(ctx context.Context) string {
	if requestID, ok := ctx.Value(RequestIDKey).(string); ok {
		return requestID
	}
	return ""
}

func FromContextPtr(ctx context.Context) *string {
	if requestID, ok := ctx.Value(RequestIDKey).(string); ok {
		return &requestID
	}
	return nil
}

// FromRequest extracts the request ID from the HTTP request context.
// Returns empty string if request ID is not found.
func FromRequest(r *http.Request) string {
	return FromContext(r.Context())
}
