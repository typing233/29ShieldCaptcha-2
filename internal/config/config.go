package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Port            int
	HMACSecret      []byte
	MinDifficulty   int
	MaxDifficulty   int
	BaseDifficulty  int
	ChallengeTTL    time.Duration
	RateLimit       float64
	RateBurst       int
	NonceExpiry     time.Duration
	TrustedProxies  []string
	AllowedOrigins  []string
	LogLevel        string
}

func Load() (*Config, error) {
	secret := os.Getenv("CAPTCHA_HMAC_SECRET")
	if secret == "" {
		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			return nil, fmt.Errorf("generate secret: %w", err)
		}
		secret = hex.EncodeToString(b)
		fmt.Fprintf(os.Stderr, "WARNING: No CAPTCHA_HMAC_SECRET set, generated ephemeral key: %s\n", secret)
	}

	secretBytes, err := hex.DecodeString(secret)
	if err != nil {
		return nil, fmt.Errorf("decode CAPTCHA_HMAC_SECRET (must be hex): %w", err)
	}

	cfg := &Config{
		Port:           getEnvInt("CAPTCHA_PORT", 8080),
		HMACSecret:     secretBytes,
		MinDifficulty:  getEnvInt("CAPTCHA_MIN_DIFFICULTY", 16),
		MaxDifficulty:  getEnvInt("CAPTCHA_MAX_DIFFICULTY", 24),
		BaseDifficulty: getEnvInt("CAPTCHA_BASE_DIFFICULTY", 18),
		ChallengeTTL:   getEnvDuration("CAPTCHA_CHALLENGE_TTL", 120*time.Second),
		RateLimit:      getEnvFloat("CAPTCHA_RATE_LIMIT", 10),
		RateBurst:      getEnvInt("CAPTCHA_RATE_BURST", 20),
		NonceExpiry:    getEnvDuration("CAPTCHA_NONCE_EXPIRY", 300*time.Second),
		LogLevel:       getEnvStr("CAPTCHA_LOG_LEVEL", "info"),
	}

	if cfg.BaseDifficulty < cfg.MinDifficulty || cfg.BaseDifficulty > cfg.MaxDifficulty {
		return nil, fmt.Errorf("base difficulty %d must be between min %d and max %d",
			cfg.BaseDifficulty, cfg.MinDifficulty, cfg.MaxDifficulty)
	}

	return cfg, nil
}

func getEnvInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func getEnvFloat(key string, def float64) float64 {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return def
	}
	return f
}

func getEnvDuration(key string, def time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return def
	}
	return d
}

func getEnvStr(key, def string) string {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	return v
}
