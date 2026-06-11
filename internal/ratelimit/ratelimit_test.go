package ratelimit

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAllow(t *testing.T) {
	l := NewLimiter(2, 2)

	if !l.Allow("1.2.3.4") {
		t.Error("First request should be allowed")
	}
	if !l.Allow("1.2.3.4") {
		t.Error("Second request should be allowed (within burst)")
	}
	if l.Allow("1.2.3.4") {
		t.Error("Third request should be rejected (burst exhausted)")
	}
}

func TestDifferentIPs(t *testing.T) {
	l := NewLimiter(1, 1)

	l.Allow("1.1.1.1")
	if l.Allow("1.1.1.1") {
		t.Error("Same IP should be limited")
	}
	if !l.Allow("2.2.2.2") {
		t.Error("Different IP should not be limited")
	}
}

func TestMiddleware(t *testing.T) {
	l := NewLimiter(1, 1)
	handler := l.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "5.5.5.5:1234"

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("First request: expected 200, got %d", w.Code)
	}

	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("Second request: expected 429, got %d", w.Code)
	}
}
