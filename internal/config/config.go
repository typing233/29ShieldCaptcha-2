package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Port           int
	HMACSecret     []byte
	MinDifficulty  int
	MaxDifficulty  int
	BaseDifficulty int
	ChallengeTTL   time.Duration
	RateLimit      float64
	RateBurst      int
	NonceExpiry    time.Duration
	TrustedProxies []string
	AllowedOrigins []string
	LogLevel       string

	// Redis
	RedisURL     string
	EnableRedis  bool
	RedisPrefix  string

	// PostgreSQL
	PostgresURL    string
	EnablePostgres bool
	AutoMigrate    bool

	// Admin
	AdminEnabled  bool
	JWTSecret     []byte
	AdminUsername string
	AdminPassword string

	// Feature Flags
	FeatureRiskEngine    bool
	FeatureBehavior      bool
	FeatureAdaptiveRate  bool
	FeatureReplayDetect  bool

	// Metrics
	MetricsEnabled bool
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

	jwtSecret := os.Getenv("CAPTCHA_JWT_SECRET")
	var jwtBytes []byte
	if jwtSecret != "" {
		jwtBytes = []byte(jwtSecret)
	} else {
		jwtBytes = secretBytes
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

		RedisURL:    getEnvStr("CAPTCHA_REDIS_URL", ""),
		EnableRedis: getEnvBool("CAPTCHA_ENABLE_REDIS", false),
		RedisPrefix: getEnvStr("CAPTCHA_REDIS_PREFIX", "sc:"),

		PostgresURL:    getEnvStr("CAPTCHA_POSTGRES_URL", ""),
		EnablePostgres: getEnvBool("CAPTCHA_ENABLE_POSTGRES", false),
		AutoMigrate:    getEnvBool("CAPTCHA_AUTO_MIGRATE", true),

		AdminEnabled:  getEnvBool("CAPTCHA_ADMIN_ENABLED", false),
		JWTSecret:     jwtBytes,
		AdminUsername: getEnvStr("CAPTCHA_ADMIN_USER", "admin"),
		AdminPassword: getEnvStr("CAPTCHA_ADMIN_PASS", ""),

		FeatureRiskEngine:   getEnvBool("CAPTCHA_FEATURE_RISK_ENGINE", false),
		FeatureBehavior:     getEnvBool("CAPTCHA_FEATURE_BEHAVIOR", false),
		FeatureAdaptiveRate: getEnvBool("CAPTCHA_FEATURE_ADAPTIVE_RATE", false),
		FeatureReplayDetect: getEnvBool("CAPTCHA_FEATURE_REPLAY_DETECT", false),

		MetricsEnabled: getEnvBool("CAPTCHA_METRICS_ENABLED", true),
	}

	if cfg.BaseDifficulty < cfg.MinDifficulty || cfg.BaseDifficulty > cfg.MaxDifficulty {
		return nil, fmt.Errorf("base difficulty %d must be between min %d and max %d",
			cfg.BaseDifficulty, cfg.MinDifficulty, cfg.MaxDifficulty)
	}

	if cfg.EnableRedis && cfg.RedisURL == "" {
		cfg.RedisURL = "redis://localhost:6379/0"
	}

	if cfg.EnablePostgres && cfg.PostgresURL == "" {
		cfg.PostgresURL = "postgres://captcha:captcha@localhost:5432/shieldcaptcha?sslmode=disable"
	}

	if cfg.AllowedOrigins == nil {
		origins := getEnvStr("CAPTCHA_ALLOWED_ORIGINS", "*")
		cfg.AllowedOrigins = strings.Split(origins, ",")
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

func getEnvBool(key string, def bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	switch strings.ToLower(v) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	}
	return def
}
