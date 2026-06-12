package risk

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"github.com/shieldcaptcha/internal/metrics"
)

type Rule struct {
	ID        int             `json:"id"`
	Name      string          `json:"name"`
	Condition json.RawMessage `json:"condition_json"`
	Action    string          `json:"action"`
	Weight    float64         `json:"weight"`
	Enabled   bool            `json:"enabled"`
}

type RuleCondition struct {
	Field    string           `json:"field"`
	Op       string           `json:"op"`
	Value    float64          `json:"value"`
	ValueStr string           `json:"value_str,omitempty"`
	And      []RuleCondition  `json:"and,omitempty"`
	Or       []RuleCondition  `json:"or,omitempty"`
	Not      *RuleCondition   `json:"not,omitempty"`
}

type RuleEvaluator struct {
	rules    atomic.Value // []Rule
	pool     *pgxpool.Pool
	log      zerolog.Logger
	stopCh   chan struct{}
	mu       sync.Mutex
	lastLoad time.Time
}

func NewRuleEvaluator(pool *pgxpool.Pool, log zerolog.Logger) *RuleEvaluator {
	re := &RuleEvaluator{
		pool:   pool,
		log:    log,
		stopCh: make(chan struct{}),
	}
	re.rules.Store([]Rule{})

	if pool != nil {
		re.loadRules()
		go re.pollLoop()
	}

	return re
}

func (re *RuleEvaluator) Reload() {
	re.loadRules()
}

func (re *RuleEvaluator) GetRules() []Rule {
	return re.rules.Load().([]Rule)
}

func (re *RuleEvaluator) Stop() {
	close(re.stopCh)
}

func (re *RuleEvaluator) Evaluate(score float64, scorerResults map[string]*ScoreResult, req *ScoringRequest) []string {
	rules := re.rules.Load().([]Rule)
	var hits []string

	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		var cond RuleCondition
		if err := json.Unmarshal(rule.Condition, &cond); err != nil {
			continue
		}
		if re.evaluateCondition(cond, score, scorerResults, req) {
			hits = append(hits, rule.Name)
		}
	}

	return hits
}

func (re *RuleEvaluator) GetActionOverride(ruleHits []string) Action {
	if len(ruleHits) == 0 {
		return ""
	}
	rules := re.rules.Load().([]Rule)
	ruleMap := make(map[string]*Rule)
	for i := range rules {
		ruleMap[rules[i].Name] = &rules[i]
	}

	highestPriority := ""
	for _, hit := range ruleHits {
		r, ok := ruleMap[hit]
		if !ok {
			continue
		}
		switch r.Action {
		case "block":
			return ActionBlock
		case "escalate":
			if highestPriority != "block" {
				highestPriority = "escalate"
			}
		case "review":
			if highestPriority == "" {
				highestPriority = "review"
			}
		}
	}
	return Action(highestPriority)
}

func (re *RuleEvaluator) evaluateCondition(cond RuleCondition, score float64, results map[string]*ScoreResult, req *ScoringRequest) bool {
	// AND conditions
	if len(cond.And) > 0 {
		for _, c := range cond.And {
			if !re.evaluateCondition(c, score, results, req) {
				return false
			}
		}
		return true
	}

	// OR conditions
	if len(cond.Or) > 0 {
		for _, c := range cond.Or {
			if re.evaluateCondition(c, score, results, req) {
				return true
			}
		}
		return false
	}

	// NOT condition
	if cond.Not != nil {
		return !re.evaluateCondition(*cond.Not, score, results, req)
	}

	// Leaf condition
	fieldVal := re.getFieldValue(cond.Field, score, results, req)
	return re.compareValues(fieldVal, cond.Op, cond.Value, cond.ValueStr)
}

func (re *RuleEvaluator) getFieldValue(field string, score float64, results map[string]*ScoreResult, req *ScoringRequest) float64 {
	switch field {
	case "risk_score":
		return score
	case "behavior_score":
		if r, ok := results["behavior"]; ok {
			return r.Score
		}
	case "replay_score":
		if r, ok := results["replay"]; ok {
			return r.Score
		}
	case "reputation_score":
		if r, ok := results["reputation"]; ok {
			return r.Score
		}
	case "velocity_variance":
		if req.BehaviorData != nil && req.BehaviorData.Features != nil {
			return req.BehaviorData.Features.VelocityVariance
		}
	case "straightness":
		if req.BehaviorData != nil && req.BehaviorData.Features != nil {
			return req.BehaviorData.Features.Straightness
		}
	case "duration":
		if req.Interaction != nil {
			return float64(req.Interaction.Duration)
		}
	}
	return 0
}

func (re *RuleEvaluator) compareValues(actual float64, op string, expected float64, expectedStr string) bool {
	switch op {
	case ">":
		return actual > expected
	case ">=":
		return actual >= expected
	case "<":
		return actual < expected
	case "<=":
		return actual <= expected
	case "==":
		return actual == expected
	case "!=":
		return actual != expected
	}
	return false
}

func (re *RuleEvaluator) loadRules() {
	re.mu.Lock()
	defer re.mu.Unlock()

	if re.pool == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := re.pool.Query(ctx,
		"SELECT id, name, condition_json, action, weight, enabled FROM risk_rules WHERE enabled = true ORDER BY weight DESC")
	if err != nil {
		re.log.Error().Err(err).Msg("failed to load rules")
		return
	}
	defer rows.Close()

	var rules []Rule
	for rows.Next() {
		var r Rule
		if err := rows.Scan(&r.ID, &r.Name, &r.Condition, &r.Action, &r.Weight, &r.Enabled); err != nil {
			re.log.Error().Err(err).Msg("failed to scan rule")
			continue
		}
		rules = append(rules, r)
	}

	re.rules.Store(rules)
	re.lastLoad = time.Now()
	metrics.RulesLoaded.Set(float64(len(rules)))
	re.log.Info().Int("count", len(rules)).Msg("risk rules loaded")
}

func (re *RuleEvaluator) pollLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-re.stopCh:
			return
		case <-ticker.C:
			re.loadRules()
		}
	}
}
