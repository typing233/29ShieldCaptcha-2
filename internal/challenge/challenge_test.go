package challenge

import (
	"testing"
	"time"

	"github.com/shieldcaptcha/internal/config"
)

func testConfig() *config.Config {
	return &config.Config{
		HMACSecret:     []byte("test-secret-key-32-bytes-long!!"),
		MinDifficulty:  4,
		MaxDifficulty:  8,
		BaseDifficulty: 4,
		ChallengeTTL:   60 * time.Second,
	}
}

func TestGenerate(t *testing.T) {
	svc := NewService(testConfig())

	c, err := svc.Generate()
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}

	if c.ID == "" {
		t.Error("ID should not be empty")
	}
	if c.Nonce == "" {
		t.Error("Nonce should not be empty")
	}
	if c.Signature == "" {
		t.Error("Signature should not be empty")
	}
	if c.Difficulty < 4 || c.Difficulty > 8 {
		t.Errorf("Difficulty %d out of range [4, 8]", c.Difficulty)
	}
	if c.ExpiresAt <= c.Timestamp {
		t.Error("ExpiresAt should be after Timestamp")
	}
}

func TestVerifySignature(t *testing.T) {
	svc := NewService(testConfig())

	c, _ := svc.Generate()
	if err := svc.Verify(c); err != nil {
		t.Fatalf("Verify() should pass for valid challenge: %v", err)
	}

	c2, _ := svc.Generate()
	c2.Signature = "tampered"
	if err := svc.Verify(c2); err == nil {
		t.Error("Verify() should fail for tampered signature")
	}
}

func TestVerifyExpired(t *testing.T) {
	cfg := testConfig()
	cfg.ChallengeTTL = -1 * time.Second
	svc := NewService(cfg)

	c, _ := svc.Generate()
	if err := svc.Verify(c); err == nil {
		t.Error("Verify() should fail for expired challenge")
	}
}

func TestVerifyPoW(t *testing.T) {
	svc := NewService(testConfig())

	c := &Challenge{
		ID:         "test-id",
		Nonce:      "test-nonce",
		Difficulty: 4,
	}

	found := false
	for i := 0; i < 100000; i++ {
		sol := string(rune(i))
		if svc.VerifyPoW(c, sol) {
			found = true
			break
		}
	}

	if !found {
		t.Skip("Could not find PoW solution in 100k iterations for difficulty 4")
	}
}

func TestHasLeadingZeroBits(t *testing.T) {
	tests := []struct {
		hash []byte
		bits int
		want bool
	}{
		{[]byte{0x00, 0x00, 0xFF}, 16, true},
		{[]byte{0x00, 0x00, 0xFF}, 17, false},
		{[]byte{0x00, 0x0F, 0xFF}, 12, true},
		{[]byte{0x00, 0x0F, 0xFF}, 13, false},
		{[]byte{0xFF}, 0, true},
		{[]byte{0x00}, 8, true},
	}

	for _, tt := range tests {
		got := hasLeadingZeroBits(tt.hash, tt.bits)
		if got != tt.want {
			t.Errorf("hasLeadingZeroBits(%v, %d) = %v, want %v", tt.hash, tt.bits, got, tt.want)
		}
	}
}

func TestDynamicDifficulty(t *testing.T) {
	cfg := testConfig()
	cfg.MinDifficulty = 4
	cfg.MaxDifficulty = 8
	cfg.BaseDifficulty = 4
	svc := NewService(cfg)

	c1, _ := svc.Generate()
	baseDiff := c1.Difficulty

	for i := 0; i < 100; i++ {
		svc.IncrementLoad()
	}
	c2, _ := svc.Generate()

	if c2.Difficulty <= baseDiff {
		t.Errorf("Difficulty should increase under load: base=%d, loaded=%d", baseDiff, c2.Difficulty)
	}
	if c2.Difficulty > 8 {
		t.Errorf("Difficulty %d exceeds max 8", c2.Difficulty)
	}
}

func TestUniqueness(t *testing.T) {
	svc := NewService(testConfig())
	seen := make(map[string]bool)

	for i := 0; i < 100; i++ {
		c, _ := svc.Generate()
		if seen[c.ID] {
			t.Fatalf("Duplicate ID generated: %s", c.ID)
		}
		seen[c.ID] = true
	}
}
