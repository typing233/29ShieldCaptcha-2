package risk

import (
	"context"
	"math"
)

type BehaviorScorer struct {
	weight float64
}

func NewBehaviorScorer() *BehaviorScorer {
	return &BehaviorScorer{weight: 1.5}
}

func (s *BehaviorScorer) Name() string    { return "behavior" }
func (s *BehaviorScorer) Weight() float64 { return s.weight }

func (s *BehaviorScorer) Score(ctx context.Context, req *ScoringRequest) (*ScoreResult, error) {
	if req.BehaviorData == nil || req.BehaviorData.Features == nil {
		return &ScoreResult{Score: 0.3, Confidence: 0.3, Reasons: []string{"no behavioral data provided"}}, nil
	}

	f := req.BehaviorData.Features
	score := 0.0
	reasons := []string{}

	// Velocity variance check - bots have very constant velocity
	if f.VelocityVariance < 10 && f.AvgVelocity > 0 {
		score += 0.25
		reasons = append(reasons, "velocity too constant (bot-like)")
	}

	// Straightness check - humans rarely move in perfectly straight lines
	if f.Straightness > 0.98 && f.TotalPathLength > 50 {
		score += 0.2
		reasons = append(reasons, "trajectory too straight")
	}

	// No pauses - humans naturally pause
	if f.PauseCount == 0 && f.TotalPathLength > 100 {
		score += 0.15
		reasons = append(reasons, "no pauses detected (unnatural)")
	}

	// Direction changes - real humans have micro-adjustments
	if f.DirectionChanges < 2 && f.TotalPathLength > 100 {
		score += 0.15
		reasons = append(reasons, "too few direction changes")
	}

	// Jerk smoothness check - humans have irregular jerk
	if f.JerkSmoothness < 5 && f.AvgVelocity > 100 {
		score += 0.15
		reasons = append(reasons, "jerk too smooth (mechanical)")
	}

	// Curvature check - no curvature at all is suspicious
	if f.Curvature < 0.01 && f.TotalPathLength > 50 {
		score += 0.1
		reasons = append(reasons, "no curvature detected")
	}

	// Extremely fast completion
	if req.Interaction != nil && req.Interaction.Duration < 300 && f.TotalPathLength > 200 {
		score += 0.2
		reasons = append(reasons, "completed too quickly")
	}

	// Too many points in too short time (programmatic high-frequency)
	if len(req.BehaviorData.Trajectory) > 500 && req.Interaction != nil && req.Interaction.Duration < 1000 {
		score += 0.15
		reasons = append(reasons, "unnatural event frequency")
	}

	score = math.Min(score, 1.0)
	confidence := 0.7
	if len(req.BehaviorData.Trajectory) > 50 {
		confidence = 0.85
	}

	return &ScoreResult{
		Score:      score,
		Confidence: confidence,
		Reasons:    reasons,
		Tags:       map[string]string{"scorer": "behavior"},
	}, nil
}
