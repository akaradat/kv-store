package httpapi

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

func RequestLog(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			// Deferred so an aborted GET /kv stream (panic(http.ErrAbortHandler)) is logged too.
			defer func() {
				log.Info("request", "method", r.Method, "path", r.URL.EscapedPath(), "status", ww.Status(), "bytes", ww.BytesWritten(), "duration", time.Since(start))
			}()
			next.ServeHTTP(ww, r)
		})
	}
}

// NotFound replaces chi's plain-text 404 with the JSON error body (R-ERR-1).
func NotFound(w http.ResponseWriter, _ *http.Request) {
	WriteError(w, notFound())
}
