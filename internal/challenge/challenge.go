package challenge

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sync/atomic"
	"time"

	"github.com/shieldcaptcha/internal/config"
)

type Challenge struct {
	ID         string `json:"id"`
	Nonce      string `json:"nonce"`
	Difficulty int    `json:"difficulty"`
	Timestamp  int64  `json:"timestamp"`
	ExpiresAt  int64  `json:"expires_at"`
	Signature  string `json:"signature"`
}

type Service struct {
	cfg            *config.Config
	activeRequests atomic.Int64
}

func NewService(cfg *config.Config) *Service {
	return &Service{cfg: cfg}
}

func (s *Service) Generate() (*Challenge, error) {
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return nil, fmt.Errorf("generate challenge id: %w", err)
	}

	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}

	now := time.Now()
	difficulty := s.computeDifficulty()

	c := &Challenge{
		ID:         hex.EncodeToString(id),
		Nonce:      hex.EncodeToString(nonce),
		Difficulty: difficulty,
		Timestamp:  now.Unix(),
		ExpiresAt:  now.Add(s.cfg.ChallengeTTL).Unix(),
	}

	sig, err := s.sign(c)
	if err != nil {
		return nil, err
	}
	c.Signature = sig

	return c, nil
}

func (s *Service) Verify(c *Challenge) error {
	sig, err := s.sign(c)
	if err != nil {
		return fmt.Errorf("compute signature: %w", err)
	}
	if !hmac.Equal([]byte(sig), []byte(c.Signature)) {
		return fmt.Errorf("invalid challenge signature")
	}

	if time.Now().Unix() > c.ExpiresAt {
		return fmt.Errorf("challenge expired")
	}

	return nil
}

func (s *Service) VerifyPoW(challenge *Challenge, solution string) bool {
	data := challenge.ID + ":" + challenge.Nonce + ":" + solution
	hash := sha256.Sum256([]byte(data))
	return hasLeadingZeroBits(hash[:], challenge.Difficulty)
}

func (s *Service) IncrementLoad() {
	s.activeRequests.Add(1)
}

func (s *Service) DecrementLoad() {
	s.activeRequests.Add(-1)
}

func (s *Service) computeDifficulty() int {
	load := s.activeRequests.Load()
	scaled := float64(s.cfg.BaseDifficulty) + math.Log2(float64(load+1))
	d := int(math.Round(scaled))
	if d < s.cfg.MinDifficulty {
		d = s.cfg.MinDifficulty
	}
	if d > s.cfg.MaxDifficulty {
		d = s.cfg.MaxDifficulty
	}
	return d
}

func (s *Service) sign(c *Challenge) (string, error) {
	payload := fmt.Sprintf("%s:%s:%d:%d:%d", c.ID, c.Nonce, c.Difficulty, c.Timestamp, c.ExpiresAt)
	mac := hmac.New(sha256.New, s.cfg.HMACSecret)
	if _, err := mac.Write([]byte(payload)); err != nil {
		return "", err
	}
	return hex.EncodeToString(mac.Sum(nil)), nil
}

func hasLeadingZeroBits(hash []byte, bits int) bool {
	fullBytes := bits / 8
	remainBits := bits % 8

	for i := 0; i < fullBytes; i++ {
		if hash[i] != 0 {
			return false
		}
	}

	if remainBits > 0 {
		mask := byte(0xFF << (8 - remainBits))
		if hash[fullBytes]&mask != 0 {
			return false
		}
	}

	return true
}

func (c *Challenge) Marshal() ([]byte, error) {
	return json.Marshal(c)
}

func UnmarshalChallenge(data []byte) (*Challenge, error) {
	var c Challenge
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	return &c, nil
}
