package middleware

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/kubev2v/migration-planner/pkg/requestid"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Logger returns a middleware that logs HTTP requests using zap logger.
// It logs request start with requestId and all fields except status, then request end with requestId and status.
func Logger() func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			// Store the original values since some middlewares might modify them
			path := r.URL.Path
			query := r.URL.RawQuery
			requestID := requestid.FromRequest(r)

			// Log request start with requestId and current fields (except status)
			startFields := []zapcore.Field{
				zap.String("request_id", requestID),
				zap.String("method", r.Method),
				zap.String("path", path),
				zap.String("query", query),
				zap.String("ip", getClientIP(r)),
				zap.String("user-agent", r.UserAgent()),
				zap.String("time", start.Format(time.RFC3339)),
			}
			zap.S().Named("http").Desugar().Info("Request started", startFields...)

			// Wrap the response writer to capture status and bytes
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			next.ServeHTTP(ww, r)

			// Log request end with requestId and status
			end := time.Now()
			latency := end.Sub(start)

			endFields := []zapcore.Field{
				zap.String("request_id", requestID),
				zap.Int("status", ww.Status()),
				zap.String("method", r.Method),
				zap.String("path", path),
				zap.String("query", query),
				zap.String("ip", getClientIP(r)),
				zap.String("user-agent", r.UserAgent()),
				zap.Duration("latency", latency),
				zap.Int("response_bytes", ww.BytesWritten()),
				zap.String("time", end.Format(time.RFC3339)),
			}

			// Log based on status code level
			msg := "Request completed"
			switch {
			case ww.Status() >= 500:
				zap.S().Named("http").Desugar().Error(msg, endFields...)
			case ww.Status() >= 400:
				zap.S().Named("http").Desugar().Warn(msg, endFields...)
			default:
				zap.S().Named("http").Desugar().Info(msg, endFields...)
			}
		})
	}
}

// getClientIP extracts the real client IP from various headers and fallbacks
func getClientIP(r *http.Request) string {
	// Try X-Forwarded-For header first (most common)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// X-Forwarded-For can contain multiple IPs, take the first one
		if idx := len(xff); idx > 0 {
			if commaIdx := 0; commaIdx < idx {
				for i, c := range xff {
					if c == ',' {
						commaIdx = i
						break
					} else if i == idx-1 {
						commaIdx = idx
					}
				}
				return xff[:commaIdx]
			}
		}
	}

	// Try X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Try X-Forwarded header
	if xf := r.Header.Get("X-Forwarded"); xf != "" {
		return xf
	}

	// Fallback to RemoteAddr
	return r.RemoteAddr
}
