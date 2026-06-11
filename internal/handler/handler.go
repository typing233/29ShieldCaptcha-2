package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/rs/zerolog"
	"github.com/shieldcaptcha/internal/challenge"
	"github.com/shieldcaptcha/internal/config"
	"github.com/shieldcaptcha/internal/store"
)

type Handler struct {
	cfg        *config.Config
	challenger *challenge.Service
	nonces     *store.NonceStore
	log        zerolog.Logger
}

func New(cfg *config.Config, challenger *challenge.Service, nonces *store.NonceStore, log zerolog.Logger) *Handler {
	return &Handler{
		cfg:        cfg,
		challenger: challenger,
		nonces:     nonces,
		log:        log,
	}
}

type VerifyRequest struct {
	Challenge   *challenge.Challenge `json:"challenge"`
	Solution    string               `json:"solution"`
	Fingerprint string               `json:"fingerprint"`
	Interaction *InteractionData     `json:"interaction"`
}

type InteractionData struct {
	Type       string      `json:"type"`
	StartTime  int64       `json:"start_time"`
	EndTime    int64       `json:"end_time"`
	Trajectory [][]float64 `json:"trajectory"`
}

type VerifyResponse struct {
	Success   bool   `json:"success"`
	Token     string `json:"token,omitempty"`
	Error     string `json:"error,omitempty"`
	Timestamp int64  `json:"timestamp"`
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

	h.log.Info().Str("challenge_id", c.ID).Int("difficulty", c.Difficulty).Msg("challenge issued")
	writeJSON(w, http.StatusOK, c)
}

func (h *Handler) VerifyChallenge(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "POST only")
		return
	}

	var req VerifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "malformed JSON request")
		return
	}

	if req.Challenge == nil || req.Solution == "" || req.Fingerprint == "" || req.Interaction == nil {
		writeError(w, http.StatusBadRequest, "missing_fields", "challenge, solution, fingerprint, and interaction are required")
		return
	}

	if err := h.challenger.Verify(req.Challenge); err != nil {
		h.log.Warn().Err(err).Str("challenge_id", req.Challenge.ID).Msg("challenge verification failed")
		writeJSON(w, http.StatusOK, &VerifyResponse{
			Success:   false,
			Error:     err.Error(),
			Timestamp: time.Now().Unix(),
		})
		return
	}

	if !h.nonces.MarkUsed(req.Challenge.Nonce) {
		h.log.Warn().Str("challenge_id", req.Challenge.ID).Msg("nonce replay detected")
		writeJSON(w, http.StatusOK, &VerifyResponse{
			Success:   false,
			Error:     "challenge already used (replay detected)",
			Timestamp: time.Now().Unix(),
		})
		return
	}

	if !h.challenger.VerifyPoW(req.Challenge, req.Solution) {
		h.log.Warn().Str("challenge_id", req.Challenge.ID).Msg("PoW verification failed")
		writeJSON(w, http.StatusOK, &VerifyResponse{
			Success:   false,
			Error:     "proof of work verification failed",
			Timestamp: time.Now().Unix(),
		})
		return
	}

	if !h.validateInteraction(req.Interaction) {
		h.log.Warn().Str("challenge_id", req.Challenge.ID).Msg("interaction validation failed")
		writeJSON(w, http.StatusOK, &VerifyResponse{
			Success:   false,
			Error:     "interaction validation failed",
			Timestamp: time.Now().Unix(),
		})
		return
	}

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
