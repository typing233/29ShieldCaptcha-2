package ratelimit

import (
	"net/http"
	"sync"

	"golang.org/x/time/rate"
)

type Limiter struct {
	mu       sync.RWMutex
	visitors map[string]*rate.Limiter
	rate     rate.Limit
	burst    int
}

func NewLimiter(r float64, burst int) *Limiter {
	return &Limiter{
		visitors: make(map[string]*rate.Limiter),
		rate:     rate.Limit(r),
		burst:    burst,
	}
}

func (l *Limiter) GetLimiter(ip string) *rate.Limiter {
	l.mu.RLock()
	limiter, exists := l.visitors[ip]
	l.mu.RUnlock()

	if exists {
		return limiter
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	if limiter, exists = l.visitors[ip]; exists {
		return limiter
	}

	limiter = rate.NewLimiter(l.rate, l.burst)
	l.visitors[ip] = limiter
	return limiter
}

func (l *Limiter) Allow(ip string) bool {
	return l.GetLimiter(ip).Allow()
}

func (l *Limiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := extractIP(r)
		if !l.Allow(ip) {
			http.Error(w, `{"error":"rate_limited","message":"too many requests"}`, http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func extractIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return xff
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	return r.RemoteAddr
}
