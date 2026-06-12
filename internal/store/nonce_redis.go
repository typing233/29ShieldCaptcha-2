package store

import (
	"context"
	"time"

	"github.com/rs/zerolog"
	"github.com/shieldcaptcha/internal/storage"
)

type RedisNonceStore struct {
	redis  *storage.RedisClient
	expiry time.Duration
	log    zerolog.Logger
}

func NewRedisNonceStore(redis *storage.RedisClient, expiry time.Duration, log zerolog.Logger) *RedisNonceStore {
	return &RedisNonceStore{
		redis:  redis,
		expiry: expiry,
		log:    log,
	}
}

func (s *RedisNonceStore) MarkUsed(nonce string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	key := s.redis.Key("nonce", nonce)
	ok, err := s.redis.SetNX(ctx, key, "1", s.expiry)
	if err != nil {
		s.log.Warn().Err(err).Str("nonce", nonce).Msg("redis nonce check failed")
		return true // fail-open on redis error
	}
	return ok
}

func (s *RedisNonceStore) IsUsed(nonce string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	key := s.redis.Key("nonce", nonce)
	exists, err := s.redis.Exists(ctx, key)
	if err != nil {
		return false
	}
	return exists
}

func (s *RedisNonceStore) Close() {}
