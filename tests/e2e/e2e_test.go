package e2e

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/shieldcaptcha/internal/challenge"
	"github.com/shieldcaptcha/internal/config"
	"github.com/shieldcaptcha/internal/handler"
	"github.com/shieldcaptcha/internal/middleware"
	"github.com/shieldcaptcha/internal/ratelimit"
	"github.com/shieldcaptcha/internal/store"
)

func setupServer(t *testing.T) *httptest.Server {
	cfg := &config.Config{
		HMACSecret:     []byte("e2e-test-secret-32-bytes-long!!"),
		MinDifficulty:  4,
		MaxDifficulty:  8,
		BaseDifficulty: 4,
		ChallengeTTL:   60 * time.Second,
		RateLimit:      100,
		RateBurst:      200,
		NonceExpiry:    60 * time.Second,
	}

	log := zerolog.Nop()
	nonceStore := store.NewNonceStore(cfg.NonceExpiry)
	challengeSvc := challenge.NewService(cfg)
	limiter := ratelimit.NewLimiter(cfg.RateLimit, cfg.RateBurst)
	h := handler.New(cfg, challengeSvc, nonceStore, log)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/challenge", h.GetChallenge)
	mux.HandleFunc("/api/verify", h.VerifyChallenge)

	var srv http.Handler = mux
	srv = limiter.Middleware(srv)
	srv = middleware.CORS(srv)

	t.Cleanup(func() { nonceStore.Close() })
	return httptest.NewServer(srv)
}

func TestFullFlow(t *testing.T) {
	server := setupServer(t)
	defer server.Close()

	resp, err := http.Get(server.URL + "/api/challenge")
	if err != nil {
		t.Fatalf("GET /api/challenge: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("Expected 200, got %d", resp.StatusCode)
	}

	var ch challenge.Challenge
	if err := json.NewDecoder(resp.Body).Decode(&ch); err != nil {
		t.Fatalf("Decode challenge: %v", err)
	}

	if ch.ID == "" || ch.Nonce == "" || ch.Signature == "" {
		t.Fatal("Challenge fields should not be empty")
	}

	solution := solveChallenge(ch.ID, ch.Nonce, ch.Difficulty)
	if solution == "" {
		t.Fatal("Failed to solve PoW")
	}

	trajectory := make([][]float64, 20)
	for i := range trajectory {
		trajectory[i] = []float64{float64(i * 15), float64(5 + i%3)}
	}

	body := map[string]interface{}{
		"challenge":   ch,
		"solution":    solution,
		"fingerprint": "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
		"interaction": map[string]interface{}{
			"type":       "drag",
			"start_time": time.Now().UnixMilli() - 1500,
			"end_time":   time.Now().UnixMilli(),
			"trajectory": trajectory,
		},
	}

	bodyBytes, _ := json.Marshal(body)
	vResp, err := http.Post(server.URL+"/api/verify", "application/json", bytes.NewReader(bodyBytes))
	if err != nil {
		t.Fatalf("POST /api/verify: %v", err)
	}
	defer vResp.Body.Close()

	var result handler.VerifyResponse
	if err := json.NewDecoder(vResp.Body).Decode(&result); err != nil {
		t.Fatalf("Decode verify response: %v", err)
	}

	if !result.Success {
		t.Fatalf("Verification should succeed, got error: %s", result.Error)
	}
	if result.Token == "" {
		t.Error("Token should not be empty on success")
	}
}

func TestReplayProtection(t *testing.T) {
	server := setupServer(t)
	defer server.Close()

	resp, _ := http.Get(server.URL + "/api/challenge")
	var ch challenge.Challenge
	json.NewDecoder(resp.Body).Decode(&ch)
	resp.Body.Close()

	solution := solveChallenge(ch.ID, ch.Nonce, ch.Difficulty)
	trajectory := make([][]float64, 20)
	for i := range trajectory {
		trajectory[i] = []float64{float64(i * 15), float64(5 + i%3)}
	}

	body := map[string]interface{}{
		"challenge":   ch,
		"solution":    solution,
		"fingerprint": "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
		"interaction": map[string]interface{}{
			"type":       "drag",
			"start_time": time.Now().UnixMilli() - 1500,
			"end_time":   time.Now().UnixMilli(),
			"trajectory": trajectory,
		},
	}
	bodyBytes, _ := json.Marshal(body)

	vResp, _ := http.Post(server.URL+"/api/verify", "application/json", bytes.NewReader(bodyBytes))
	var result1 handler.VerifyResponse
	json.NewDecoder(vResp.Body).Decode(&result1)
	vResp.Body.Close()
	if !result1.Success {
		t.Fatalf("First verification should succeed: %s", result1.Error)
	}

	vResp2, _ := http.Post(server.URL+"/api/verify", "application/json", bytes.NewReader(bodyBytes))
	var result2 handler.VerifyResponse
	json.NewDecoder(vResp2.Body).Decode(&result2)
	vResp2.Body.Close()

	if result2.Success {
		t.Error("Replay should be rejected")
	}
}

func TestInvalidPoW(t *testing.T) {
	server := setupServer(t)
	defer server.Close()

	resp, _ := http.Get(server.URL + "/api/challenge")
	var ch challenge.Challenge
	json.NewDecoder(resp.Body).Decode(&ch)
	resp.Body.Close()

	trajectory := make([][]float64, 20)
	for i := range trajectory {
		trajectory[i] = []float64{float64(i * 15), float64(5 + i%3)}
	}

	body := map[string]interface{}{
		"challenge":   ch,
		"solution":    "wrong-solution",
		"fingerprint": "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
		"interaction": map[string]interface{}{
			"type":       "drag",
			"start_time": time.Now().UnixMilli() - 1500,
			"end_time":   time.Now().UnixMilli(),
			"trajectory": trajectory,
		},
	}
	bodyBytes, _ := json.Marshal(body)

	vResp, _ := http.Post(server.URL+"/api/verify", "application/json", bytes.NewReader(bodyBytes))
	var result handler.VerifyResponse
	json.NewDecoder(vResp.Body).Decode(&result)
	vResp.Body.Close()

	if result.Success {
		t.Error("Invalid PoW should be rejected")
	}
}

func TestBotInteraction(t *testing.T) {
	server := setupServer(t)
	defer server.Close()

	resp, _ := http.Get(server.URL + "/api/challenge")
	var ch challenge.Challenge
	json.NewDecoder(resp.Body).Decode(&ch)
	resp.Body.Close()

	solution := solveChallenge(ch.ID, ch.Nonce, ch.Difficulty)

	body := map[string]interface{}{
		"challenge":   ch,
		"solution":    solution,
		"fingerprint": "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
		"interaction": map[string]interface{}{
			"type":       "drag",
			"start_time": time.Now().UnixMilli() - 10,
			"end_time":   time.Now().UnixMilli(),
			"trajectory": [][]float64{{0, 0}, {300, 0}},
		},
	}
	bodyBytes, _ := json.Marshal(body)

	vResp, _ := http.Post(server.URL+"/api/verify", "application/json", bytes.NewReader(bodyBytes))
	var result handler.VerifyResponse
	json.NewDecoder(vResp.Body).Decode(&result)
	vResp.Body.Close()

	if result.Success {
		t.Error("Bot-like interaction (too fast, too few points) should be rejected")
	}
}

func solveChallenge(id, nonce string, difficulty int) string {
	for i := 0; i < 10000000; i++ {
		solution := fmt.Sprintf("%x", i)
		data := id + ":" + nonce + ":" + solution
		hash := sha256.Sum256([]byte(data))
		if hasLeadingZeros(hash[:], difficulty) {
			return solution
		}
	}
	return ""
}

func hasLeadingZeros(hash []byte, bits int) bool {
	fullBytes := bits / 8
	remainBits := bits % 8

	for i := 0; i < fullBytes; i++ {
		if hash[i] != 0 {
			return false
		}
	}
	if remainBits > 0 {
		mask := byte(0xFF << (8 - remainBits))
		if hash[fullBytes]&mask != 0 {
			return false
		}
	}
	return true
}

