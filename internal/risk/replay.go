package risk

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/rs/zerolog"
	"github.com/shieldcaptcha/internal/storage"
)

type ReplayScorer struct {
	weight float64
	redis  *storage.RedisClient
	log    zerolog.Logger
}

func NewReplayScorer(redis *storage.RedisClient, log zerolog.Logger) *ReplayScorer {
	return &ReplayScorer{weight: 2.0, redis: redis, log: log}
}

func (s *ReplayScorer) Name() string    { return "replay" }
func (s *ReplayScorer) Weight() float64 { return s.weight }

func (s *ReplayScorer) Score(ctx context.Context, req *ScoringRequest) (*ScoreResult, error) {
	if req.BehaviorData == nil || len(req.BehaviorData.Trajectory) < 10 {
		return &ScoreResult{Score: 0, Confidence: 0.2}, nil
	}

	if s.redis == nil || !s.redis.Available() {
		return &ScoreResult{Score: 0, Confidence: 0.1, Reasons: []string{"replay check skipped (redis unavailable)"}}, nil
	}

	trajectory := req.BehaviorData.Trajectory
	signature := computeTrajectorySignature(trajectory)

	key := s.redis.Key("replay", req.Fingerprint)
	existing, err := s.redis.Get(ctx, key)
	if err == nil && existing != "" {
		similarity := compareSignatures(signature, existing)
		if similarity > 0.92 {
			return &ScoreResult{
				Score:      0.95,
				Confidence: 0.9,
				Reasons:    []string{"trajectory matches previous submission (replay detected)"},
				Tags:       map[string]string{"similarity": formatFloat(similarity)},
			}, nil
		}
		if similarity > 0.8 {
			return &ScoreResult{
				Score:      0.5,
				Confidence: 0.7,
				Reasons:    []string{"trajectory similar to previous submission"},
				Tags:       map[string]string{"similarity": formatFloat(similarity)},
			}, nil
		}
	}

	// Store this trajectory signature for future comparison
	_ = s.redis.Set(ctx, key, signature, 1*time.Hour)

	return &ScoreResult{Score: 0, Confidence: 0.8}, nil
}

func computeTrajectorySignature(trajectory [][]float64) string {
	if len(trajectory) == 0 {
		return ""
	}

	// Downsample to 20 representative points
	step := len(trajectory) / 20
	if step < 1 {
		step = 1
	}

	sig := ""
	for i := 0; i < len(trajectory) && i/step < 20; i += step {
		if len(trajectory[i]) >= 2 {
			x := int(trajectory[i][0]*10) / 10
			y := int(trajectory[i][1]*10) / 10
			sig += fmt.Sprintf("%d,%d;", x, y)
		}
	}
	return sig
}

func compareSignatures(a, b string) float64 {
	if a == b {
		return 1.0
	}
	if a == "" || b == "" {
		return 0.0
	}

	// Simple character-level similarity using Levenshtein ratio
	dist := levenshtein(a, b)
	maxLen := math.Max(float64(len(a)), float64(len(b)))
	if maxLen == 0 {
		return 1.0
	}
	return 1.0 - float64(dist)/maxLen
}

func levenshtein(a, b string) int {
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}

	prev := make([]int, lb+1)
	curr := make([]int, lb+1)

	for j := 0; j <= lb; j++ {
		prev[j] = j
	}

	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = min3(curr[j-1]+1, prev[j]+1, prev[j-1]+cost)
		}
		prev, curr = curr, prev
	}
	return prev[lb]
}

func min3(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}

func formatFloat(f float64) string {
	return fmt.Sprintf("%.4f", f)
}
