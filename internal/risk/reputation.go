package risk

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog"
	"github.com/shieldcaptcha/internal/storage"
)

type ReputationScorer struct {
	weight float64
	redis  *storage.RedisClient
	log    zerolog.Logger
}

func NewReputationScorer(redis *storage.RedisClient, log zerolog.Logger) *ReputationScorer {
	return &ReputationScorer{weight: 1.0, redis: redis, log: log}
}

func (s *ReputationScorer) Name() string    { return "reputation" }
func (s *ReputationScorer) Weight() float64 { return s.weight }

func (s *ReputationScorer) Score(ctx context.Context, req *ScoringRequest) (*ScoreResult, error) {
	if s.redis == nil || !s.redis.Available() {
		return &ScoreResult{Score: 0, Confidence: 0.1}, nil
	}

	score := 0.0
	reasons := []string{}

	// Check IP failure rate
	ipFailKey := s.redis.Key("rep", "ip_fail", req.IP)
	ipFailCount, err := s.getCounter(ctx, ipFailKey)
	if err == nil && ipFailCount > 10 {
		score += 0.3
		reasons = append(reasons, fmt.Sprintf("IP has %d recent failures", ipFailCount))
	} else if err == nil && ipFailCount > 5 {
		score += 0.15
		reasons = append(reasons, fmt.Sprintf("IP has %d recent failures", ipFailCount))
	}

	// Check distinct fingerprints per IP (many fingerprints = suspicious)
	ipFpKey := s.redis.Key("rep", "ip_fps", req.IP)
	distinctFps, _ := s.redis.ZCard(ctx, ipFpKey)
	if distinctFps > 20 {
		score += 0.3
		reasons = append(reasons, fmt.Sprintf("IP associated with %d distinct fingerprints", distinctFps))
	} else if distinctFps > 10 {
		score += 0.15
		reasons = append(reasons, fmt.Sprintf("IP associated with %d distinct fingerprints", distinctFps))
	}

	// Check distinct IPs per fingerprint (many IPs = rotating proxy)
	fpIpKey := s.redis.Key("rep", "fp_ips", req.Fingerprint)
	distinctIPs, _ := s.redis.ZCard(ctx, fpIpKey)
	if distinctIPs > 15 {
		score += 0.25
		reasons = append(reasons, fmt.Sprintf("fingerprint seen from %d distinct IPs", distinctIPs))
	} else if distinctIPs > 8 {
		score += 0.1
		reasons = append(reasons, fmt.Sprintf("fingerprint seen from %d distinct IPs", distinctIPs))
	}

	// Fingerprint failure rate
	fpFailKey := s.redis.Key("rep", "fp_fail", req.Fingerprint)
	fpFailCount, err := s.getCounter(ctx, fpFailKey)
	if err == nil && fpFailCount > 10 {
		score += 0.2
		reasons = append(reasons, fmt.Sprintf("fingerprint has %d recent failures", fpFailCount))
	}

	if score > 1.0 {
		score = 1.0
	}

	return &ScoreResult{
		Score:      score,
		Confidence: 0.75,
		Reasons:    reasons,
		Tags:       map[string]string{"scorer": "reputation"},
	}, nil
}

func (s *ReputationScorer) RecordResult(ctx context.Context, ip, fingerprint string, passed bool) {
	if s.redis == nil || !s.redis.Available() {
		return
	}

	now := float64(time.Now().UnixMilli())
	window := 1 * time.Hour

	// Track IP -> fingerprint mapping
	ipFpKey := s.redis.Key("rep", "ip_fps", ip)
	_ = s.redis.ZAdd(ctx, ipFpKey, struct {
		Score  float64
		Member interface{}
	}{Score: now, Member: fingerprint})
	_ = s.redis.Expire(ctx, ipFpKey, window)

	// Track fingerprint -> IP mapping
	fpIpKey := s.redis.Key("rep", "fp_ips", fingerprint)
	_ = s.redis.ZAdd(ctx, fpIpKey, struct {
		Score  float64
		Member interface{}
	}{Score: now, Member: ip})
	_ = s.redis.Expire(ctx, fpIpKey, window)

	if !passed {
		// Increment failure counters
		ipFailKey := s.redis.Key("rep", "ip_fail", ip)
		_, _ = s.redis.Incr(ctx, ipFailKey)
		_ = s.redis.Expire(ctx, ipFailKey, window)

		fpFailKey := s.redis.Key("rep", "fp_fail", fingerprint)
		_, _ = s.redis.Incr(ctx, fpFailKey)
		_ = s.redis.Expire(ctx, fpFailKey, window)
	}
}

func (s *ReputationScorer) getCounter(ctx context.Context, key string) (int64, error) {
	val, err := s.redis.Get(ctx, key)
	if err != nil {
		return 0, err
	}
	var count int64
	fmt.Sscanf(val, "%d", &count)
	return count, nil
}
