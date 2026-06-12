package storage

import "errors"

var (
	ErrCircuitOpen = errors.New("circuit breaker open: redis unavailable")
	ErrNotFound    = errors.New("not found")
)
