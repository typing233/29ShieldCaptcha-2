package risk

import "context"

type Action string

const (
	ActionPass     Action = "pass"
	ActionEscalate Action = "escalate"
	ActionBlock    Action = "block"
	ActionReview   Action = "review"
)

type ScoreResult struct {
	Score      float64
	Confidence float64
	Reasons    []string
	Tags       map[string]string
}

type Scorer interface {
	Name() string
	Score(ctx context.Context, req *ScoringRequest) (*ScoreResult, error)
	Weight() float64
}

type ScoringRequest struct {
	IP            string
	Fingerprint   string
	ChallengeID   string
	BehaviorData  *BehaviorInput
	Interaction   *InteractionInput
	Metadata      map[string]interface{}
}

type BehaviorInput struct {
	Trajectory   [][]float64
	Timestamps   []int64
	Pressures    []float64
	Features     *FeatureInput
}

type FeatureInput struct {
	AvgVelocity      float64 `json:"avg_velocity"`
	MaxVelocity      float64 `json:"max_velocity"`
	VelocityVariance float64 `json:"velocity_variance"`
	AvgAcceleration  float64 `json:"avg_acceleration"`
	JerkSmoothness   float64 `json:"jerk_smoothness"`
	Curvature        float64 `json:"curvature"`
	PauseCount       int     `json:"pause_count"`
	Straightness     float64 `json:"straightness"`
	DirectionChanges int     `json:"direction_changes"`
	TotalPathLength  float64 `json:"total_path_length"`
	Displacement     float64 `json:"displacement"`
}

type InteractionInput struct {
	Type       string
	StartTime  int64
	EndTime    int64
	Duration   int64
	Trajectory [][]float64
}

type RiskDecision struct {
	Action    Action   `json:"action"`
	Score     float64  `json:"score"`
	Reasons   []string `json:"reasons"`
	RuleHits  []string `json:"rule_hits,omitempty"`
	ScorerDetails map[string]*ScoreResult `json:"scorer_details,omitempty"`
}
