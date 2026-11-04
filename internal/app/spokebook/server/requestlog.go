package server

import (
	"context"
	"log/slog"
	"net/http"
	"time"
)

// logRequests logs every request at debug level: method, path, status,
// duration and client address. The query is left out, it may carry the
// setup token.
func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.log.Enabled(r.Context(), slog.LevelDebug) {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		s.log.LogAttrs(context.Background(), slog.LevelDebug, "request",
			slog.String("method", r.Method), slog.String("path", r.URL.Path),
			slog.Int("status", sw.status), slog.Duration("duration", time.Since(start)),
			slog.String("client", clientIP(r, s.trustedProxies)))
	})
}

// statusWriter remembers the status code of a response.
type statusWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (w *statusWriter) WriteHeader(code int) {
	if !w.wroteHeader {
		w.status, w.wroteHeader = code, true
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
