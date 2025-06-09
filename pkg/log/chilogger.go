package log

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"go.uber.org/zap"
)

func Logger(l *zap.Logger, name string) func(next http.Handler) http.Handler {
	if l == nil {
		panic("log.Logger received a nil *zap.Logger")
	}

	logger := l.WithOptions(zap.AddCallerSkip(1)).Named(name)

	return func(next http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			t1 := time.Now()
			requestID := middleware.GetReqID(r.Context())

			defer func() {
				latency := time.Since(t1)
				statusCode := ww.Status()
				bytesWritten := ww.BytesWritten()
				statusText := statusLabel(statusCode)

				fields := []zap.Field{
					zap.String("type", "http_request"),
					zap.String("request_id", requestID),
					zap.String("http_method", r.Method),
					zap.String("http_path", r.URL.Path),
					zap.String("http_proto", r.Proto),
					zap.String("remote_addr", r.RemoteAddr),
					zap.Int("http_status_code", statusCode),
					zap.String("http_status_text", statusText),
					zap.Int64("response_bytes", int64(bytesWritten)),
					zap.Duration("latency", latency),
					zap.String("user_agent", r.UserAgent()),
				}

				msg := fmt.Sprintf("HTTP request completed: %s", r.URL.Path)

				switch {
				case statusCode >= 500:
					logger.Error(msg, fields...)
				case statusCode >= 400:
					logger.Warn(msg, fields...)
				default:
					if isHealthCheck(r.Method, r.URL.Path) {
						logger.Debug(msg, fields...)
					} else {
						logger.Info(msg, fields...)
					}
				}
			}()

			next.ServeHTTP(ww, r)
		}
		return http.HandlerFunc(fn)
	}
}

// ConditionalLogger returns HTTP request logging middleware only if log level is debug/trace
func ConditionalLogger(logLevel string, l *zap.Logger, name string) func(next http.Handler) http.Handler {
	if l == nil {
		panic("log.ConditionalLogger received a nil *zap.Logger")
	}

	logger := l.WithOptions(zap.AddCallerSkip(1)).Named(name)
	level := strings.ToLower(logLevel)

	if level == "debug" || level == "trace" {
		logger.Info("HTTP request logging enabled (debug mode)")
		return Logger(l, name)
	}
	
	logger.Info("HTTP request logging disabled (info mode) - using Envoy for access logs")
	return func(next http.Handler) http.Handler {
		return next // Pass through without logging
	}
}

func isHealthCheck(method string, path string) bool {
	return method == http.MethodGet && path == "/health"
}

func statusLabel(status int) string {
	switch {
	case status >= 100 && status < 300:
		return fmt.Sprintf("%d OK", status)
	case status >= 300 && status < 400:
		return fmt.Sprintf("%d Redirect", status)
	case status >= 400 && status < 500:
		return fmt.Sprintf("%d Client Error", status)
	case status >= 500:
		return fmt.Sprintf("%d Server Error", status)
	default:
		return fmt.Sprintf("%d Unknown", status)
	}
}
