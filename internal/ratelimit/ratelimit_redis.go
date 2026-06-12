package ratelimit

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/rs/zerolog"
	"github.com/shieldcaptcha/internal/storage"
)

type RedisLimiter struct {
	redis    *storage.RedisClient
	rate     int
	window   time.Duration
	log      zerolog.Logger
	fallback *Limiter
}

func NewRedisLimiter(redis *storage.RedisClient, ratePerWindow int, window time.Duration, fallback *Limiter, log zerolog.Logger) *RedisLimiter {
	return &RedisLimiter{
		redis:    redis,
		rate:     ratePerWindow,
		window:   window,
		log:      log,
		fallback: fallback,
	}
}

func (rl *RedisLimiter) Allow(ip string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	now := time.Now()
	windowStart := now.Add(-rl.window)
	key := rl.redis.Key("rl", "ip", ip)

	err := rl.redis.ZRemRangeByScore(ctx, key, "0", fmt.Sprintf("%d", windowStart.UnixMilli()))
	if err != nil {
		return rl.fallback.Allow(ip)
	}

	count, err := rl.redis.ZCard(ctx, key)
	if err != nil {
		return rl.fallback.Allow(ip)
	}

	if count >= int64(rl.rate) {
		return false
	}

	member := fmt.Sprintf("%d", now.UnixNano())
	err = rl.redis.ZAdd(ctx, key, struct {
		Score  float64
		Member interface{}
	}{Score: float64(now.UnixMilli()), Member: member})
	if err != nil {
		return rl.fallback.Allow(ip)
	}

	_ = rl.redis.Expire(ctx, key, rl.window+time.Second)
	return true
}

func (rl *RedisLimiter) AllowMulti(dimensions map[string]string) bool {
	for dim, val := range dimensions {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		now := time.Now()
		windowStart := now.Add(-rl.window)
		key := rl.redis.Key("rl", dim, val)

		_ = rl.redis.ZRemRangeByScore(ctx, key, "0", fmt.Sprintf("%d", windowStart.UnixMilli()))
		count, err := rl.redis.ZCard(ctx, key)
		cancel()
		if err != nil {
			continue
		}
		if count >= int64(rl.rate) {
			return false
		}
	}
	return true
}

func (rl *RedisLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := extractIP(r)
		if !rl.Allow(ip) {
			http.Error(w, `{"error":"rate_limited","message":"too many requests"}`, http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}
