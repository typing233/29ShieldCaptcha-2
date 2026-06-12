package handler

import (
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"

	"github.com/rs/zerolog"
	"github.com/shieldcaptcha/internal/challenge"
	"github.com/shieldcaptcha/internal/config"
	"github.com/shieldcaptcha/internal/feature"
	"github.com/shieldcaptcha/internal/metrics"
	"github.com/shieldcaptcha/internal/store"
)

type Handler struct {
	cfg        *config.Config
	challenger *challenge.Service
	nonces     store.NonceStorer
	log        zerolog.Logger
	flags      *feature.Flags
}

func New(cfg *config.Config, challenger *challenge.Service, nonces store.NonceStorer, log zerolog.Logger, flags *feature.Flags) *Handler {
	return &Handler{
		cfg:        cfg,
		challenger: challenger,
		nonces:     nonces,
		log:        log,
		flags:      flags,
	}
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

func (h *Handler) VerifyChallenge(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "POST only")
		return
	}

	start := time.Now()
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

	if err := h.challenger.Verify(req.Challenge); err != nil {
		h.log.Warn().Err(err).Str("challenge_id", req.Challenge.ID).Msg("challenge verification failed")
		metrics.VerificationsTotal.WithLabelValues("challenge_fail").Inc()
		writeJSON(w, http.StatusOK, &VerifyResponse{
			Success:   false,
			Error:     err.Error(),
			Timestamp: time.Now().Unix(),
		})
		return
	}

	if !h.challenger.VerifyPoW(req.Challenge, req.Solution) {
		h.log.Warn().Str("challenge_id", req.Challenge.ID).Msg("PoW verification failed")
		metrics.VerificationsTotal.WithLabelValues("pow_fail").Inc()
		writeJSON(w, http.StatusOK, &VerifyResponse{
			Success:   false,
			Error:     "proof of work verification failed",
			Timestamp: time.Now().Unix(),
		})
		return
	}

	if !h.validateInteraction(req.Interaction) {
		h.log.Warn().Str("challenge_id", req.Challenge.ID).Msg("interaction validation failed")
		metrics.VerificationsTotal.WithLabelValues("interaction_fail").Inc()
		writeJSON(w, http.StatusOK, &VerifyResponse{
			Success:   false,
			Error:     "interaction validation failed",
			Timestamp: time.Now().Unix(),
		})
		return
	}

	if !h.nonces.MarkUsed(req.Challenge.Nonce) {
		h.log.Warn().Str("challenge_id", req.Challenge.ID).Msg("nonce replay detected")
		metrics.VerificationsTotal.WithLabelValues("replay").Inc()
		writeJSON(w, http.StatusOK, &VerifyResponse{
			Success:   false,
			Error:     "challenge already used (replay detected)",
			Timestamp: time.Now().Unix(),
		})
		return
	}

	metrics.VerificationsTotal.WithLabelValues("pass").Inc()
	h.log.Info().
		Str("challenge_id", req.Challenge.ID).
		Str("fingerprint", req.Fingerprint[:8]+"...").
		Msg("verification successful")

	writeJSON(w, http.StatusOK, &VerifyResponse{
		Success:   true,
		Token:     req.Challenge.ID,
		Timestamp: time.Now().Unix(),
	})
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
