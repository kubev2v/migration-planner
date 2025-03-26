package log

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"go.uber.org/zap"
)

var (
	sugaredLogFormat = `[%s] "%s %s %s" from %s - %s %dB in %s`
)

func Logger(l interface{}, name string) func(next http.Handler) http.Handler {
	switch logger := l.(type) {

	case *zap.SugaredLogger:
		logger = zap.New(logger.Desugar().Core(), zap.AddCallerSkip(1)).Sugar().Named(name)
		return func(next http.Handler) http.Handler {
			fn := func(w http.ResponseWriter, r *http.Request) {
				ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
				t1 := time.Now()
				requestID := middleware.GetReqID(r.Context())
				defer func() {
					statusCode := ww.Status()
					status := statusLabel(statusCode)
					bytes := ww.BytesWritten()
					latency := time.Since(t1)
					args := []interface{}{
						requestID,
						r.Method,
						r.URL.Path,
						r.Proto,
						r.RemoteAddr,
						status,
						bytes,
						latency,
					}
					switch {
					case statusCode >= 500:
						logger.Errorf(sugaredLogFormat, args...)
					case statusCode >= 400:
						logger.Warnf(sugaredLogFormat, args...)
					default:
						if isDebugLog(r.Method, r.URL.Path) {
							logger.Debugf(sugaredLogFormat, args...)
						}
						logger.Infof(sugaredLogFormat, args...)
					}

				}()
				next.ServeHTTP(ww, r)
			}
			return http.HandlerFunc(fn)
		}
	default:
		log.Fatalf("Unknown logger passed in. Please provide *Zap.Logger or *Zap.SugaredLogger")
	}
	return nil
}

func isDebugLog(method string, path string) bool {
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
