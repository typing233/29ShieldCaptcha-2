package store

import (
	"sync"
	"time"
)

type NonceStore struct {
	mu      sync.RWMutex
	nonces  map[string]time.Time
	expiry  time.Duration
	closeCh chan struct{}
}

func NewNonceStore(expiry time.Duration) *NonceStore {
	s := &NonceStore{
		nonces:  make(map[string]time.Time),
		expiry:  expiry,
		closeCh: make(chan struct{}),
	}
	go s.cleanup()
	return s
}

func (s *NonceStore) MarkUsed(nonce string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.nonces[nonce]; exists {
		return false
	}
	s.nonces[nonce] = time.Now().Add(s.expiry)
	return true
}

func (s *NonceStore) IsUsed(nonce string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, exists := s.nonces[nonce]
	return exists
}

func (s *NonceStore) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.nonces)
}

func (s *NonceStore) Close() {
	close(s.closeCh)
}

func (s *NonceStore) cleanup() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-s.closeCh:
			return
		case now := <-ticker.C:
			s.mu.Lock()
			for k, exp := range s.nonces {
				if now.After(exp) {
					delete(s.nonces, k)
				}
			}
			s.mu.Unlock()
		}
	}
}
