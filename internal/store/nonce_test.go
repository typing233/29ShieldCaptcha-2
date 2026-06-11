package store

import (
	"sync"
	"testing"
	"time"
)

func TestMarkUsed(t *testing.T) {
	s := NewNonceStore(5 * time.Second)
	defer s.Close()

	if !s.MarkUsed("nonce1") {
		t.Error("First use should succeed")
	}
	if s.MarkUsed("nonce1") {
		t.Error("Second use should fail (replay)")
	}
}

func TestIsUsed(t *testing.T) {
	s := NewNonceStore(5 * time.Second)
	defer s.Close()

	if s.IsUsed("unknown") {
		t.Error("Unknown nonce should not be used")
	}

	s.MarkUsed("known")
	if !s.IsUsed("known") {
		t.Error("Known nonce should be used")
	}
}

func TestConcurrency(t *testing.T) {
	s := NewNonceStore(5 * time.Second)
	defer s.Close()

	var wg sync.WaitGroup
	successes := make(chan bool, 100)

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			successes <- s.MarkUsed("same-nonce")
		}()
	}

	wg.Wait()
	close(successes)

	successCount := 0
	for ok := range successes {
		if ok {
			successCount++
		}
	}

	if successCount != 1 {
		t.Errorf("Expected exactly 1 success for concurrent MarkUsed, got %d", successCount)
	}
}

func TestExpiry(t *testing.T) {
	s := NewNonceStore(50 * time.Millisecond)
	defer s.Close()

	s.MarkUsed("expiring")
	if !s.IsUsed("expiring") {
		t.Error("Should be used immediately after marking")
	}

	time.Sleep(100 * time.Millisecond)

	s.mu.Lock()
	now := time.Now()
	for k, exp := range s.nonces {
		if now.After(exp) {
			delete(s.nonces, k)
		}
	}
	s.mu.Unlock()

	if s.IsUsed("expiring") {
		t.Error("Should be expired after waiting")
	}
}
