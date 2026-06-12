package handler

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
	"github.com/shieldcaptcha/internal/feature"
	"github.com/shieldcaptcha/internal/store"
)

func testHandler() (*Handler, *challenge.Service) {
	cfg := &config.Config{
		HMACSecret:     []byte("handler-test-secret-32-bytes!!!"),
		MinDifficulty:  4,
		MaxDifficulty:  8,
		BaseDifficulty: 4,
		ChallengeTTL:   60 * time.Second,
		NonceExpiry:    60 * time.Second,
	}
	log := zerolog.Nop()
	nonceStore := store.NewNonceStore(cfg.NonceExpiry)
	challengeSvc := challenge.NewService(cfg)
	flags := feature.NewFlags()
	h := New(cfg, challengeSvc, nonceStore, log, flags)
	return h, challengeSvc
}

func validTrajectory() [][]float64 {
	t := make([][]float64, 20)
	for i := range t {
		t[i] = []float64{float64(i * 15), float64(5 + i%3)}
	}
	return t
}

func solvePoW(id, nonce string, difficulty int) string {
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

func TestVerify_InvalidFingerprint_Short(t *testing.T) {
	h, challengeSvc := testHandler()
	ch, _ := challengeSvc.Generate()
	sol := solvePoW(ch.ID, ch.Nonce, ch.Difficulty)

	body := VerifyRequest{
		Challenge:   ch,
		Solution:    sol,
		Fingerprint: "abc",
		Interaction: &InteractionData{
			Type:       "drag",
			StartTime:  time.Now().UnixMilli() - 1500,
			EndTime:    time.Now().UnixMilli(),
			Trajectory: validTrajectory(),
		},
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/api/verify", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.VerifyChallenge(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 for short fingerprint, got %d", w.Code)
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["error"] != "invalid_fingerprint" {
		t.Errorf("Expected error code 'invalid_fingerprint', got '%s'", resp["error"])
	}
}

func TestVerify_InvalidFingerprint_NotHex(t *testing.T) {
	h, challengeSvc := testHandler()
	ch, _ := challengeSvc.Generate()
	sol := solvePoW(ch.ID, ch.Nonce, ch.Difficulty)

	body := VerifyRequest{
		Challenge:   ch,
		Solution:    sol,
		Fingerprint: "zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz",
		Interaction: &InteractionData{
			Type:       "drag",
			StartTime:  time.Now().UnixMilli() - 1500,
			EndTime:    time.Now().UnixMilli(),
			Trajectory: validTrajectory(),
		},
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/api/verify", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.VerifyChallenge(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 for non-hex fingerprint, got %d", w.Code)
	}
}

func TestVerify_InvalidFingerprint_Empty(t *testing.T) {
	h, _ := testHandler()

	body := VerifyRequest{
		Challenge:   &challenge.Challenge{ID: "x", Nonce: "y", Signature: "z"},
		Solution:    "sol",
		Fingerprint: "",
		Interaction: &InteractionData{Type: "drag"},
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/api/verify", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.VerifyChallenge(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 for empty fingerprint, got %d", w.Code)
	}
}

func TestVerify_FailedPoW_DoesNotConsumeNonce(t *testing.T) {
	h, challengeSvc := testHandler()
	ch, _ := challengeSvc.Generate()

	validFP := "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2"

	// First attempt: wrong PoW solution
	body1 := VerifyRequest{
		Challenge:   ch,
		Solution:    "wrong-solution",
		Fingerprint: validFP,
		Interaction: &InteractionData{
			Type:       "drag",
			StartTime:  time.Now().UnixMilli() - 1500,
			EndTime:    time.Now().UnixMilli(),
			Trajectory: validTrajectory(),
		},
	}
	bodyBytes1, _ := json.Marshal(body1)
	req1 := httptest.NewRequest("POST", "/api/verify", bytes.NewReader(bodyBytes1))
	req1.Header.Set("Content-Type", "application/json")
	w1 := httptest.NewRecorder()
	h.VerifyChallenge(w1, req1)

	var result1 VerifyResponse
	json.NewDecoder(w1.Body).Decode(&result1)
	if result1.Success {
		t.Fatal("Wrong PoW should fail")
	}

	// Second attempt: correct PoW — should NOT be rejected as replay
	correctSol := solvePoW(ch.ID, ch.Nonce, ch.Difficulty)
	body2 := VerifyRequest{
		Challenge:   ch,
		Solution:    correctSol,
		Fingerprint: validFP,
		Interaction: &InteractionData{
			Type:       "drag",
			StartTime:  time.Now().UnixMilli() - 1500,
			EndTime:    time.Now().UnixMilli(),
			Trajectory: validTrajectory(),
		},
	}
	bodyBytes2, _ := json.Marshal(body2)
	req2 := httptest.NewRequest("POST", "/api/verify", bytes.NewReader(bodyBytes2))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	h.VerifyChallenge(w2, req2)

	var result2 VerifyResponse
	json.NewDecoder(w2.Body).Decode(&result2)
	if !result2.Success {
		t.Fatalf("Correct retry after failed PoW should succeed, got: %s", result2.Error)
	}
}

func TestVerify_FailedInteraction_DoesNotConsumeNonce(t *testing.T) {
	h, challengeSvc := testHandler()
	ch, _ := challengeSvc.Generate()

	validFP := "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2"
	correctSol := solvePoW(ch.ID, ch.Nonce, ch.Difficulty)

	// First attempt: correct PoW but bot-like interaction (too fast, too few points)
	body1 := VerifyRequest{
		Challenge:   ch,
		Solution:    correctSol,
		Fingerprint: validFP,
		Interaction: &InteractionData{
			Type:       "drag",
			StartTime:  time.Now().UnixMilli() - 10,
			EndTime:    time.Now().UnixMilli(),
			Trajectory: [][]float64{{0, 0}, {300, 0}},
		},
	}
	bodyBytes1, _ := json.Marshal(body1)
	req1 := httptest.NewRequest("POST", "/api/verify", bytes.NewReader(bodyBytes1))
	req1.Header.Set("Content-Type", "application/json")
	w1 := httptest.NewRecorder()
	h.VerifyChallenge(w1, req1)

	var result1 VerifyResponse
	json.NewDecoder(w1.Body).Decode(&result1)
	if result1.Success {
		t.Fatal("Bot interaction should fail")
	}

	// Second attempt: correct interaction — should succeed, nonce not burned
	body2 := VerifyRequest{
		Challenge:   ch,
		Solution:    correctSol,
		Fingerprint: validFP,
		Interaction: &InteractionData{
			Type:       "drag",
			StartTime:  time.Now().UnixMilli() - 1500,
			EndTime:    time.Now().UnixMilli(),
			Trajectory: validTrajectory(),
		},
	}
	bodyBytes2, _ := json.Marshal(body2)
	req2 := httptest.NewRequest("POST", "/api/verify", bytes.NewReader(bodyBytes2))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	h.VerifyChallenge(w2, req2)

	var result2 VerifyResponse
	json.NewDecoder(w2.Body).Decode(&result2)
	if !result2.Success {
		t.Fatalf("Correct retry after bad interaction should succeed, got: %s", result2.Error)
	}
}

func TestVerify_SuccessThenReplay(t *testing.T) {
	h, challengeSvc := testHandler()
	ch, _ := challengeSvc.Generate()

	validFP := "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2"
	correctSol := solvePoW(ch.ID, ch.Nonce, ch.Difficulty)

	body := VerifyRequest{
		Challenge:   ch,
		Solution:    correctSol,
		Fingerprint: validFP,
		Interaction: &InteractionData{
			Type:       "drag",
			StartTime:  time.Now().UnixMilli() - 1500,
			EndTime:    time.Now().UnixMilli(),
			Trajectory: validTrajectory(),
		},
	}
	bodyBytes, _ := json.Marshal(body)

	// First: success
	req1 := httptest.NewRequest("POST", "/api/verify", bytes.NewReader(bodyBytes))
	req1.Header.Set("Content-Type", "application/json")
	w1 := httptest.NewRecorder()
	h.VerifyChallenge(w1, req1)
	var r1 VerifyResponse
	json.NewDecoder(w1.Body).Decode(&r1)
	if !r1.Success {
		t.Fatalf("First attempt should succeed: %s", r1.Error)
	}

	// Second: replay — must fail
	req2 := httptest.NewRequest("POST", "/api/verify", bytes.NewReader(bodyBytes))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	h.VerifyChallenge(w2, req2)
	var r2 VerifyResponse
	json.NewDecoder(w2.Body).Decode(&r2)
	if r2.Success {
		t.Error("Replay after success should be rejected")
	}
	if r2.Error != "challenge already used (replay detected)" {
		t.Errorf("Expected replay error, got: %s", r2.Error)
	}
}
