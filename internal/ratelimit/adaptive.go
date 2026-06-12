package ratelimit

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/shieldcaptcha/internal/storage"
)

type AdaptiveLimiter struct {
	redis      *storage.RedisClient
	baseRate   int
	window     time.Duration
	log        zerolog.Logger
	fallback   *Limiter
	mu         sync.RWMutex
	tightened  map[string]time.Time
}

func NewAdaptiveLimiter(redis *storage.RedisClient, baseRate int, window time.Duration, fallback *Limiter, log zerolog.Logger) *AdaptiveLimiter {
	al := &AdaptiveLimiter{
		redis:     redis,
		baseRate:  baseRate,
		window:    window,
		log:       log,
		fallback:  fallback,
		tightened: make(map[string]time.Time),
	}
	go al.cleanupLoop()
	return al
}

func (al *AdaptiveLimiter) Tighten(dimension, value string, duration time.Duration) {
	al.mu.Lock()
	defer al.mu.Unlock()
	key := dimension + ":" + value
	al.tightened[key] = time.Now().Add(duration)
	al.log.Info().Str("key", key).Dur("duration", duration).Msg("rate limit tightened")
}

func (al *AdaptiveLimiter) Relax(dimension, value string) {
	al.mu.Lock()
	defer al.mu.Unlock()
	key := dimension + ":" + value
	delete(al.tightened, key)
	al.log.Info().Str("key", key).Msg("rate limit relaxed")
}

func (al *AdaptiveLimiter) isTightened(dimension, value string) bool {
	al.mu.RLock()
	defer al.mu.RUnlock()
	key := dimension + ":" + value
	expiry, exists := al.tightened[key]
	if !exists {
		return false
	}
	return time.Now().Before(expiry)
}

func (al *AdaptiveLimiter) effectiveRate(dimension, value string) int {
	if al.isTightened(dimension, value) {
		return al.baseRate / 4
	}
	return al.baseRate
}

func (al *AdaptiveLimiter) AllowMulti(dimensions map[string]string) (bool, string) {
	for dim, val := range dimensions {
		rate := al.effectiveRate(dim, val)
		if !al.checkDimension(dim, val, rate) {
			return false, dim
		}
	}
	return true, ""
}

func (al *AdaptiveLimiter) checkDimension(dimension, value string, limit int) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	now := time.Now()
	windowStart := now.Add(-al.window)
	key := al.redis.Key("arl", dimension, value)

	err := al.redis.ZRemRangeByScore(ctx, key, "0", fmt.Sprintf("%d", windowStart.UnixMilli()))
	if err != nil {
		return al.fallback.Allow(value)
	}

	count, err := al.redis.ZCard(ctx, key)
	if err != nil {
		return al.fallback.Allow(value)
	}

	if count >= int64(limit) {
		return false
	}

	member := fmt.Sprintf("%d-%d", now.UnixNano(), count)
	err = al.redis.ZAdd(ctx, key, struct {
		Score  float64
		Member interface{}
	}{Score: float64(now.UnixMilli()), Member: member})
	if err != nil {
		return al.fallback.Allow(value)
	}

	_ = al.redis.Expire(ctx, key, al.window+time.Second)
	return true
}

func (al *AdaptiveLimiter) RecordBlock(ip, fingerprint string) {
	al.Tighten("ip", ip, 5*time.Minute)
	if fingerprint != "" {
		al.Tighten("fp", fingerprint, 10*time.Minute)
	}
}

func (al *AdaptiveLimiter) Unblock(ip, fingerprint string) {
	if ip != "" {
		al.Relax("ip", ip)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = al.redis.Del(ctx, al.redis.Key("arl", "ip", ip))
		cancel()
	}
	if fingerprint != "" {
		al.Relax("fp", fingerprint)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = al.redis.Del(ctx, al.redis.Key("arl", "fp", fingerprint))
		cancel()
	}
}

func (al *AdaptiveLimiter) cleanupLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		al.mu.Lock()
		now := time.Now()
		for k, exp := range al.tightened {
			if now.After(exp) {
				delete(al.tightened, k)
			}
		}
		al.mu.Unlock()
	}
}
