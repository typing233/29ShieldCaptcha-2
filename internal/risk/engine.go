package risk

import (
	"context"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

type Engine struct {
	scorers   []Scorer
	rules     *RuleEvaluator
	log       zerolog.Logger
	mu        sync.RWMutex
}

func NewEngine(log zerolog.Logger) *Engine {
	return &Engine{
		scorers: make([]Scorer, 0),
		log:     log,
	}
}

func (e *Engine) RegisterScorer(s Scorer) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.scorers = append(e.scorers, s)
	e.log.Info().Str("scorer", s.Name()).Float64("weight", s.Weight()).Msg("scorer registered")
}

func (e *Engine) SetRules(rules *RuleEvaluator) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.rules = rules
}

func (e *Engine) Evaluate(ctx context.Context, req *ScoringRequest) *RiskDecision {
	e.mu.RLock()
	scorers := make([]Scorer, len(e.scorers))
	copy(scorers, e.scorers)
	rules := e.rules
	e.mu.RUnlock()

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	results := make(map[string]*ScoreResult, len(scorers))
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, s := range scorers {
		wg.Add(1)
		go func(scorer Scorer) {
			defer wg.Done()
			result, err := scorer.Score(ctx, req)
			if err != nil {
				e.log.Warn().Err(err).Str("scorer", scorer.Name()).Msg("scorer failed")
				return
			}
			mu.Lock()
			results[scorer.Name()] = result
			mu.Unlock()
		}(s)
	}
	wg.Wait()

	totalScore, totalWeight := 0.0, 0.0
	var allReasons []string
	for name, result := range results {
		scorer := e.findScorer(name)
		if scorer == nil {
			continue
		}
		weight := scorer.Weight()
		totalScore += result.Score * weight
		totalWeight += weight
		allReasons = append(allReasons, result.Reasons...)
	}

	finalScore := 0.0
	if totalWeight > 0 {
		finalScore = totalScore / totalWeight
	}

	decision := &RiskDecision{
		Score:         finalScore,
		Reasons:       allReasons,
		ScorerDetails: results,
	}

	// Apply rules
	if rules != nil {
		ruleHits := rules.Evaluate(finalScore, results, req)
		decision.RuleHits = ruleHits
	}

	// Determine action based on final score
	switch {
	case finalScore >= 0.8:
		decision.Action = ActionBlock
	case finalScore >= 0.6:
		decision.Action = ActionEscalate
	case finalScore >= 0.4:
		decision.Action = ActionReview
	default:
		decision.Action = ActionPass
	}

	// Rule overrides can change the action
	if rules != nil {
		if override := rules.GetActionOverride(decision.RuleHits); override != "" {
			decision.Action = override
		}
	}

	e.log.Debug().
		Float64("score", finalScore).
		Str("action", string(decision.Action)).
		Int("reasons", len(allReasons)).
		Msg("risk evaluation complete")

	return decision
}

func (e *Engine) findScorer(name string) Scorer {
	for _, s := range e.scorers {
		if s.Name() == name {
			return s
		}
	}
	return nil
}
