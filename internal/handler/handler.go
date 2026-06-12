package handler

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"github.com/shieldcaptcha/internal/challenge"
	"github.com/shieldcaptcha/internal/config"
	"github.com/shieldcaptcha/internal/feature"
	"github.com/shieldcaptcha/internal/metrics"
	"github.com/shieldcaptcha/internal/ratelimit"
	"github.com/shieldcaptcha/internal/risk"
	"github.com/shieldcaptcha/internal/storage"
	"github.com/shieldcaptcha/internal/store"
)

type Handler struct {
	cfg             *config.Config
	challenger      *challenge.Service
	nonces          store.NonceStorer
	log             zerolog.Logger
	flags           *feature.Flags
	riskEngine      *risk.Engine
	adaptiveLimiter *ratelimit.AdaptiveLimiter
	reputationScorer *risk.ReputationScorer
	pg              *pgxpool.Pool
	redis           *storage.RedisClient
	logCh           chan *verificationLog
}

type verificationLog struct {
	ChallengeID  string
	Fingerprint  string
	IP           string
	Result       string
	RiskScore    *float64
	HitReasons   []string
	BehaviorData *BehaviorData
	DurationMs   int
}

func New(cfg *config.Config, challenger *challenge.Service, nonces store.NonceStorer, log zerolog.Logger, flags *feature.Flags) *Handler {
	h := &Handler{
		cfg:        cfg,
		challenger: challenger,
		nonces:     nonces,
		log:        log,
		flags:      flags,
		logCh:      make(chan *verificationLog, 256),
	}
	go h.logWriter()
	return h
}

func (h *Handler) SetRiskEngine(engine *risk.Engine) {
	h.riskEngine = engine
}

func (h *Handler) SetAdaptiveLimiter(al *ratelimit.AdaptiveLimiter) {
	h.adaptiveLimiter = al
}

func (h *Handler) SetReputationScorer(rs *risk.ReputationScorer) {
	h.reputationScorer = rs
}

func (h *Handler) SetPostgres(pool *pgxpool.Pool) {
	h.pg = pool
}

func (h *Handler) SetRedis(redis *storage.RedisClient) {
	h.redis = redis
}

type BehaviorData struct {
	Trajectory [][]float64       `json:"trajectory"`
	Timestamps []int64           `json:"timestamps"`
	Pressures  []float64         `json:"pressures,omitempty"`
	Features   *BehaviorFeatures `json:"features,omitempty"`
}

type BehaviorFeatures struct {
	AvgVelocity      float64   `json:"avg_velocity"`
	MaxVelocity      float64   `json:"max_velocity"`
	VelocityVariance float64   `json:"velocity_variance"`
	AvgAcceleration  float64   `json:"avg_acceleration"`
	JerkSmoothness   float64   `json:"jerk_smoothness"`
	Curvature        float64   `json:"curvature"`
	PauseCount       int       `json:"pause_count"`
	PauseDurations   []float64 `json:"pause_durations,omitempty"`
	Straightness     float64   `json:"straightness"`
	DirectionChanges int       `json:"direction_changes"`
	TotalPathLength  float64   `json:"total_path_length"`
	Displacement     float64   `json:"displacement"`
}

type VerifyRequest struct {
	Challenge   *challenge.Challenge `json:"challenge"`
	Solution    string               `json:"solution"`
	Fingerprint string               `json:"fingerprint"`
	Interaction *InteractionData     `json:"interaction"`
	Behavior    *BehaviorData        `json:"behavior,omitempty"`
}

type InteractionData struct {
	Type       string      `json:"type"`
	StartTime  int64       `json:"start_time"`
	EndTime    int64       `json:"end_time"`
	Trajectory [][]float64 `json:"trajectory"`
}

type VerifyResponse struct {
	Success     bool     `json:"success"`
	Token       string   `json:"token,omitempty"`
	Error       string   `json:"error,omitempty"`
	Timestamp   int64    `json:"timestamp"`
	RiskScore   float64  `json:"risk_score,omitempty"`
	RiskReasons []string `json:"risk_reasons,omitempty"`
}

func (h *Handler) GetChallenge(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "GET only")
		return
	}

	h.challenger.IncrementLoad()
	defer h.challenger.DecrementLoad()

	c, err := h.challenger.Generate()
	if err != nil {
		h.log.Error().Err(err).Msg("failed to generate challenge")
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to generate challenge")
		return
	}

	metrics.ChallengesIssued.Inc()
	metrics.ActiveSessions.Inc()
	h.log.Info().Str("challenge_id", c.ID).Int("difficulty", c.Difficulty).Msg("challenge issued")
	writeJSON(w, http.StatusOK, c)
}

// GetWidgetConfig serves the current widget styling and experiment config to the SDK
func (h *Handler) GetWidgetConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "GET only")
		return
	}

	result := map[string]interface{}{
		"theme": map[string]interface{}{
			"primaryColor": "#1890ff",
			"sliderShape":  "round",
			"width":        380,
			"height":       48,
		},
		"experiment": nil,
	}

	if h.pg != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		// Load widget style config
		var configJSON []byte
		err := h.pg.QueryRow(ctx, "SELECT config FROM widget_config WHERE name = 'default' LIMIT 1").Scan(&configJSON)
		if err == nil && configJSON != nil {
			var cfg map[string]interface{}
			if json.Unmarshal(configJSON, &cfg) == nil {
				result["theme"] = cfg
			}
		}

		// Load active experiment (if any)
		var expName string
		var trafficPct int
		var configA, configB []byte
		err = h.pg.QueryRow(ctx, "SELECT name, traffic_pct, config_a, config_b FROM experiments WHERE status = 'active' LIMIT 1").
			Scan(&expName, &trafficPct, &configA, &configB)
		if err == nil {
			result["experiment"] = map[string]interface{}{
				"name":        expName,
				"traffic_pct": trafficPct,
				"config_a":    json.RawMessage(configA),
				"config_b":    json.RawMessage(configB),
			}
		}
	}

	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) VerifyChallenge(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "POST only")
		return
	}

	start := time.Now()
	ip := extractClientIP(r)

	defer func() {
		metrics.VerificationDuration.Observe(time.Since(start).Seconds())
		metrics.ActiveSessions.Dec()
	}()

	var req VerifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "malformed JSON request")
		return
	}

	if req.Challenge == nil || req.Solution == "" || req.Fingerprint == "" || req.Interaction == nil {
		writeError(w, http.StatusBadRequest, "missing_fields", "challenge, solution, fingerprint, and interaction are required")
		return
	}

	if !isValidFingerprint(req.Fingerprint) {
		writeError(w, http.StatusBadRequest, "invalid_fingerprint", "fingerprint must be a 64-character hex string (SHA-256)")
		return
	}

	// --- Adaptive rate limit check (per fingerprint+IP) ---
	if h.flags.Enabled(feature.FlagAdaptiveRate) && h.adaptiveLimiter != nil {
		allowed, blockedDim := h.adaptiveLimiter.AllowMulti(map[string]string{
			"ip": ip,
			"fp": req.Fingerprint,
		})
		if !allowed {
			h.log.Warn().Str("ip", ip).Str("dimension", blockedDim).Msg("adaptive rate limit hit")
			metrics.RateLimitHits.WithLabelValues(blockedDim).Inc()
			h.asyncLog(&verificationLog{
				ChallengeID: req.Challenge.ID, Fingerprint: req.Fingerprint,
				IP: ip, Result: "block", DurationMs: int(time.Since(start).Milliseconds()),
			})
			writeJSON(w, http.StatusOK, &VerifyResponse{
				Success:     false,
				Error:       "rate limit exceeded",
				Timestamp:   time.Now().Unix(),
				RiskReasons: []string{"adaptive rate limit: " + blockedDim},
			})
			return
		}
	}

	if err := h.challenger.Verify(req.Challenge); err != nil {
		h.log.Warn().Err(err).Str("challenge_id", req.Challenge.ID).Msg("challenge verification failed")
		metrics.VerificationsTotal.WithLabelValues("challenge_fail").Inc()
		h.asyncLog(&verificationLog{
			ChallengeID: req.Challenge.ID, Fingerprint: req.Fingerprint,
			IP: ip, Result: "fail", DurationMs: int(time.Since(start).Milliseconds()),
		})
		writeJSON(w, http.StatusOK, &VerifyResponse{
			Success: false, Error: err.Error(), Timestamp: time.Now().Unix(),
		})
		return
	}

	if !h.challenger.VerifyPoW(req.Challenge, req.Solution) {
		h.log.Warn().Str("challenge_id", req.Challenge.ID).Msg("PoW verification failed")
		metrics.VerificationsTotal.WithLabelValues("pow_fail").Inc()
		h.asyncLog(&verificationLog{
			ChallengeID: req.Challenge.ID, Fingerprint: req.Fingerprint,
			IP: ip, Result: "fail", DurationMs: int(time.Since(start).Milliseconds()),
		})
		writeJSON(w, http.StatusOK, &VerifyResponse{
			Success: false, Error: "proof of work verification failed", Timestamp: time.Now().Unix(),
		})
		return
	}

	if !h.validateInteraction(req.Interaction) {
		h.log.Warn().Str("challenge_id", req.Challenge.ID).Msg("interaction validation failed")
		metrics.VerificationsTotal.WithLabelValues("interaction_fail").Inc()
		h.asyncLog(&verificationLog{
			ChallengeID: req.Challenge.ID, Fingerprint: req.Fingerprint,
			IP: ip, Result: "fail", DurationMs: int(time.Since(start).Milliseconds()),
		})
		writeJSON(w, http.StatusOK, &VerifyResponse{
			Success: false, Error: "interaction validation failed", Timestamp: time.Now().Unix(),
		})
		return
	}

	// --- Risk Engine Evaluation ---
	var riskDecision *risk.RiskDecision
	if h.flags.Enabled(feature.FlagRiskEngine) && h.riskEngine != nil {
		scoringReq := h.buildScoringRequest(ip, &req)
		riskDecision = h.riskEngine.Evaluate(r.Context(), scoringReq)
		metrics.RiskScoreHistogram.Observe(riskDecision.Score)

		if riskDecision.Action == risk.ActionBlock {
			h.log.Warn().
				Str("challenge_id", req.Challenge.ID).
				Float64("risk_score", riskDecision.Score).
				Strs("reasons", riskDecision.Reasons).
				Msg("blocked by risk engine")
			metrics.VerificationsTotal.WithLabelValues("block").Inc()

			// Tighten adaptive rate limit for this IP/fingerprint
			if h.adaptiveLimiter != nil {
				h.adaptiveLimiter.RecordBlock(ip, req.Fingerprint)
			}
			// Record failure in reputation
			if h.reputationScorer != nil {
				h.reputationScorer.RecordResult(r.Context(), ip, req.Fingerprint, false)
			}

			h.asyncLog(&verificationLog{
				ChallengeID: req.Challenge.ID, Fingerprint: req.Fingerprint,
				IP: ip, Result: "block", RiskScore: &riskDecision.Score,
				HitReasons: riskDecision.Reasons, BehaviorData: req.Behavior,
				DurationMs: int(time.Since(start).Milliseconds()),
			})
			writeJSON(w, http.StatusOK, &VerifyResponse{
				Success:     false,
				Error:       "verification rejected by risk analysis",
				Timestamp:   time.Now().Unix(),
				RiskScore:   riskDecision.Score,
				RiskReasons: riskDecision.Reasons,
			})
			return
		}
	}

	// --- Nonce replay check (only consumed on full success) ---
	if !h.nonces.MarkUsed(req.Challenge.Nonce) {
		h.log.Warn().Str("challenge_id", req.Challenge.ID).Msg("nonce replay detected")
		metrics.VerificationsTotal.WithLabelValues("replay").Inc()
		h.asyncLog(&verificationLog{
			ChallengeID: req.Challenge.ID, Fingerprint: req.Fingerprint,
			IP: ip, Result: "fail", DurationMs: int(time.Since(start).Milliseconds()),
		})
		writeJSON(w, http.StatusOK, &VerifyResponse{
			Success: false, Error: "challenge already used (replay detected)", Timestamp: time.Now().Unix(),
		})
		return
	}

	// --- Success ---
	metrics.VerificationsTotal.WithLabelValues("pass").Inc()

	// Record success in reputation system
	if h.reputationScorer != nil {
		h.reputationScorer.RecordResult(r.Context(), ip, req.Fingerprint, true)
	}

	var riskScore float64
	var riskReasons []string
	if riskDecision != nil {
		riskScore = riskDecision.Score
		riskReasons = riskDecision.Reasons
	}

	h.asyncLog(&verificationLog{
		ChallengeID: req.Challenge.ID, Fingerprint: req.Fingerprint,
		IP: ip, Result: "pass", RiskScore: &riskScore,
		HitReasons: riskReasons, BehaviorData: req.Behavior,
		DurationMs: int(time.Since(start).Milliseconds()),
	})

	h.log.Info().
		Str("challenge_id", req.Challenge.ID).
		Str("fingerprint", req.Fingerprint[:8]+"...").
		Msg("verification successful")

	resp := &VerifyResponse{
		Success:   true,
		Token:     req.Challenge.ID,
		Timestamp: time.Now().Unix(),
	}
	if riskDecision != nil && riskDecision.Score > 0 {
		resp.RiskScore = riskDecision.Score
		resp.RiskReasons = riskDecision.Reasons
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) buildScoringRequest(ip string, req *VerifyRequest) *risk.ScoringRequest {
	sr := &risk.ScoringRequest{
		IP:          ip,
		Fingerprint: req.Fingerprint,
		ChallengeID: req.Challenge.ID,
	}

	if req.Behavior != nil {
		sr.BehaviorData = &risk.BehaviorInput{
			Trajectory: req.Behavior.Trajectory,
			Timestamps: req.Behavior.Timestamps,
			Pressures:  req.Behavior.Pressures,
		}
		if req.Behavior.Features != nil {
			sr.BehaviorData.Features = &risk.FeatureInput{
				AvgVelocity:      req.Behavior.Features.AvgVelocity,
				MaxVelocity:      req.Behavior.Features.MaxVelocity,
				VelocityVariance: req.Behavior.Features.VelocityVariance,
				AvgAcceleration:  req.Behavior.Features.AvgAcceleration,
				JerkSmoothness:   req.Behavior.Features.JerkSmoothness,
				Curvature:        req.Behavior.Features.Curvature,
				PauseCount:       req.Behavior.Features.PauseCount,
				Straightness:     req.Behavior.Features.Straightness,
				DirectionChanges: req.Behavior.Features.DirectionChanges,
				TotalPathLength:  req.Behavior.Features.TotalPathLength,
				Displacement:     req.Behavior.Features.Displacement,
			}
		}
	}

	if req.Interaction != nil {
		sr.Interaction = &risk.InteractionInput{
			Type:       req.Interaction.Type,
			StartTime:  req.Interaction.StartTime,
			EndTime:    req.Interaction.EndTime,
			Duration:   req.Interaction.EndTime - req.Interaction.StartTime,
			Trajectory: req.Interaction.Trajectory,
		}
	}

	return sr
}

// asyncLog sends a verification log to the background writer
func (h *Handler) asyncLog(entry *verificationLog) {
	select {
	case h.logCh <- entry:
	default:
		h.log.Warn().Msg("verification log channel full, dropping entry")
	}
}

// logWriter batches verification logs and writes them to PostgreSQL
func (h *Handler) logWriter() {
	batch := make([]*verificationLog, 0, 100)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case entry, ok := <-h.logCh:
			if !ok {
				h.flushLogs(batch)
				return
			}
			batch = append(batch, entry)
			if len(batch) >= 100 {
				h.flushLogs(batch)
				batch = batch[:0]
			}
		case <-ticker.C:
			if len(batch) > 0 {
				h.flushLogs(batch)
				batch = batch[:0]
			}
		}
	}
}

func (h *Handler) flushLogs(batch []*verificationLog) {
	if h.pg == nil || len(batch) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for _, entry := range batch {
		var hitReasonsJSON, behaviorJSON []byte
		if entry.HitReasons != nil {
			hitReasonsJSON, _ = json.Marshal(entry.HitReasons)
		}
		if entry.BehaviorData != nil {
			behaviorJSON, _ = json.Marshal(entry.BehaviorData)
		}

		_, err := h.pg.Exec(ctx,
			`INSERT INTO verification_logs (challenge_id, fingerprint, ip_address, result, risk_score, hit_reasons, behavior_data, duration_ms)
			 VALUES ($1, $2, $3::inet, $4, $5, $6, $7, $8)`,
			entry.ChallengeID, entry.Fingerprint, entry.IP, entry.Result,
			entry.RiskScore, hitReasonsJSON, behaviorJSON, entry.DurationMs)
		if err != nil {
			h.log.Error().Err(err).Str("challenge_id", entry.ChallengeID).Msg("failed to write verification log")
		}
	}
}

func (h *Handler) validateInteraction(data *InteractionData) bool {
	if data.Type != "click" && data.Type != "drag" {
		return false
	}

	duration := data.EndTime - data.StartTime
	if duration < 200 || duration > 30000 {
		return false
	}

	if len(data.Trajectory) < 3 {
		return false
	}

	if data.Type == "drag" && len(data.Trajectory) < 5 {
		return false
	}

	hasMovement := false
	for i := 1; i < len(data.Trajectory); i++ {
		if len(data.Trajectory[i]) < 2 || len(data.Trajectory[i-1]) < 2 {
			return false
		}
		dx := data.Trajectory[i][0] - data.Trajectory[i-1][0]
		dy := data.Trajectory[i][1] - data.Trajectory[i-1][1]
		if dx != 0 || dy != 0 {
			hasMovement = true
		}
	}

	return hasMovement
}

func extractClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return xff
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	host := r.RemoteAddr
	// Strip port from RemoteAddr
	for i := len(host) - 1; i >= 0; i-- {
		if host[i] == ':' {
			return host[:i]
		}
	}
	return host
}

func isValidFingerprint(fp string) bool {
	if len(fp) != 64 {
		return false
	}
	_, err := hex.DecodeString(fp)
	return err == nil
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{
		"error":   code,
		"message": message,
	})
}
