package middleware

import (
	"bufio"
	"log/slog"
	"net"
	"net/http"
	"time"

	chimw "github.com/go-chi/chi/v5/middleware"
)

// statusRecorder wraps http.ResponseWriter to capture the status code.
// It delegates Hijack and Flush to the underlying writer when available,
// so it doesn't break WebSocket upgrades or streaming responses.
type statusRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (sr *statusRecorder) WriteHeader(code int) {
	sr.statusCode = code
	sr.ResponseWriter.WriteHeader(code)
}

func (sr *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := sr.ResponseWriter.(http.Hijacker); ok {
		return hj.Hijack()
	}
	return nil, nil, http.ErrNotSupported
}

// Unwrap returns the underlying ResponseWriter for middleware that
// needs to walk the wrapper chain (e.g. gorilla/websocket).
func (sr *statusRecorder) Unwrap() http.ResponseWriter {
	return sr.ResponseWriter
}

// RequestLogger is an slog-based request logging middleware.
// It logs the method, path, status code, duration, and request ID for each request.
func RequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		rec := &statusRecorder{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}

		next.ServeHTTP(rec, r)

		reqID := chimw.GetReqID(r.Context())
		slog.Debug("request completed",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.statusCode,
			"duration_ms", time.Since(start).Milliseconds(),
			"request_id", reqID,
		)
	})
}
