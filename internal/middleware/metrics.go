package middleware

import (
	"fmt"
	"net/http"
	"time"

	"github.com/shieldcaptcha/internal/metrics"
)

func Metrics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &metricsWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(sw, r)
		duration := time.Since(start).Seconds()

		path := normalizePath(r.URL.Path)
		status := fmt.Sprintf("%d", sw.status)

		metrics.HTTPRequestsTotal.WithLabelValues(r.Method, path, status).Inc()
		metrics.HTTPRequestDuration.WithLabelValues(r.Method, path).Observe(duration)
	})
}

type metricsWriter struct {
	http.ResponseWriter
	status int
}

func (mw *metricsWriter) WriteHeader(code int) {
	mw.status = code
	mw.ResponseWriter.WriteHeader(code)
}

func normalizePath(path string) string {
	switch {
	case path == "/api/challenge":
		return "/api/challenge"
	case path == "/api/verify":
		return "/api/verify"
	case path == "/health":
		return "/health"
	case path == "/metrics":
		return "/metrics"
	case len(path) > 10 && path[:10] == "/admin/api":
		return "/admin/api"
	default:
		return "/static"
	}
}
