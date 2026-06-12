package storage

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
)

type RedisClient struct {
	client       *redis.Client
	log          zerolog.Logger
	prefix       string
	circuitOpen  atomic.Bool
	failures     atomic.Int64
	lastAttempt  atomic.Int64
	mu           sync.RWMutex
}

const (
	circuitThreshold   = 5
	circuitResetPeriod = 10 * time.Second
)

func NewRedisClient(url string, prefix string, log zerolog.Logger) (*RedisClient, error) {
	opts, err := redis.ParseURL(url)
	if err != nil {
		return nil, err
	}
	opts.PoolSize = 20
	opts.MinIdleConns = 5
	opts.DialTimeout = 3 * time.Second
	opts.ReadTimeout = 2 * time.Second
	opts.WriteTimeout = 2 * time.Second

	client := redis.NewClient(opts)
	rc := &RedisClient{
		client: client,
		log:    log,
		prefix: prefix,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		log.Warn().Err(err).Msg("redis initial ping failed, circuit breaker starting open")
		rc.circuitOpen.Store(true)
	} else {
		log.Info().Msg("redis connected")
	}

	return rc, nil
}

func (rc *RedisClient) Available() bool {
	if !rc.circuitOpen.Load() {
		return true
	}
	last := rc.lastAttempt.Load()
	now := time.Now().UnixMilli()
	if now-last > circuitResetPeriod.Milliseconds() {
		rc.lastAttempt.Store(now)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := rc.client.Ping(ctx).Err(); err == nil {
			rc.circuitOpen.Store(false)
			rc.failures.Store(0)
			rc.log.Info().Msg("redis circuit breaker closed (recovered)")
			return true
		}
	}
	return false
}

func (rc *RedisClient) recordSuccess() {
	rc.failures.Store(0)
}

func (rc *RedisClient) recordFailure(err error) {
	count := rc.failures.Add(1)
	if count >= circuitThreshold {
		rc.circuitOpen.Store(true)
		rc.lastAttempt.Store(time.Now().UnixMilli())
		rc.log.Warn().Err(err).Int64("failures", count).Msg("redis circuit breaker opened")
	}
}

func (rc *RedisClient) Key(parts ...string) string {
	key := rc.prefix
	for i, p := range parts {
		if i > 0 {
			key += ":"
		}
		key += p
	}
	return key
}

func (rc *RedisClient) Client() *redis.Client {
	return rc.client
}

func (rc *RedisClient) Close() error {
	return rc.client.Close()
}

func (rc *RedisClient) SetNX(ctx context.Context, key string, value interface{}, expiration time.Duration) (bool, error) {
	if !rc.Available() {
		return false, ErrCircuitOpen
	}
	result, err := rc.client.SetNX(ctx, key, value, expiration).Result()
	if err != nil {
		rc.recordFailure(err)
		return false, err
	}
	rc.recordSuccess()
	return result, nil
}

func (rc *RedisClient) Exists(ctx context.Context, key string) (bool, error) {
	if !rc.Available() {
		return false, ErrCircuitOpen
	}
	n, err := rc.client.Exists(ctx, key).Result()
	if err != nil {
		rc.recordFailure(err)
		return false, err
	}
	rc.recordSuccess()
	return n > 0, nil
}

func (rc *RedisClient) Incr(ctx context.Context, key string) (int64, error) {
	if !rc.Available() {
		return 0, ErrCircuitOpen
	}
	val, err := rc.client.Incr(ctx, key).Result()
	if err != nil {
		rc.recordFailure(err)
		return 0, err
	}
	rc.recordSuccess()
	return val, nil
}

func (rc *RedisClient) Expire(ctx context.Context, key string, expiration time.Duration) error {
	if !rc.Available() {
		return ErrCircuitOpen
	}
	err := rc.client.Expire(ctx, key, expiration).Err()
	if err != nil {
		rc.recordFailure(err)
	} else {
		rc.recordSuccess()
	}
	return err
}

func (rc *RedisClient) Get(ctx context.Context, key string) (string, error) {
	if !rc.Available() {
		return "", ErrCircuitOpen
	}
	val, err := rc.client.Get(ctx, key).Result()
	if err != nil {
		if err != redis.Nil {
			rc.recordFailure(err)
		}
		return "", err
	}
	rc.recordSuccess()
	return val, nil
}

func (rc *RedisClient) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	if !rc.Available() {
		return ErrCircuitOpen
	}
	err := rc.client.Set(ctx, key, value, expiration).Err()
	if err != nil {
		rc.recordFailure(err)
	} else {
		rc.recordSuccess()
	}
	return err
}

func (rc *RedisClient) Del(ctx context.Context, keys ...string) error {
	if !rc.Available() {
		return ErrCircuitOpen
	}
	err := rc.client.Del(ctx, keys...).Err()
	if err != nil {
		rc.recordFailure(err)
	} else {
		rc.recordSuccess()
	}
	return err
}

func (rc *RedisClient) ZAdd(ctx context.Context, key string, members ...redis.Z) error {
	if !rc.Available() {
		return ErrCircuitOpen
	}
	err := rc.client.ZAdd(ctx, key, members...).Err()
	if err != nil {
		rc.recordFailure(err)
	} else {
		rc.recordSuccess()
	}
	return err
}

func (rc *RedisClient) ZRemRangeByScore(ctx context.Context, key, min, max string) error {
	if !rc.Available() {
		return ErrCircuitOpen
	}
	err := rc.client.ZRemRangeByScore(ctx, key, min, max).Err()
	if err != nil {
		rc.recordFailure(err)
	} else {
		rc.recordSuccess()
	}
	return err
}

func (rc *RedisClient) ZCard(ctx context.Context, key string) (int64, error) {
	if !rc.Available() {
		return 0, ErrCircuitOpen
	}
	val, err := rc.client.ZCard(ctx, key).Result()
	if err != nil {
		rc.recordFailure(err)
		return 0, err
	}
	rc.recordSuccess()
	return val, nil
}

func (rc *RedisClient) HealthCheck(ctx context.Context) error {
	if !rc.Available() {
		return ErrCircuitOpen
	}
	return rc.client.Ping(ctx).Err()
}
