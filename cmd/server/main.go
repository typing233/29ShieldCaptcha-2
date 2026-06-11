package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/shieldcaptcha/internal/challenge"
	"github.com/shieldcaptcha/internal/config"
	"github.com/shieldcaptcha/internal/handler"
	"github.com/shieldcaptcha/internal/middleware"
	"github.com/shieldcaptcha/internal/ratelimit"
	"github.com/shieldcaptcha/internal/store"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}

	level, _ := zerolog.ParseLevel(cfg.LogLevel)
	log := zerolog.New(os.Stdout).With().Timestamp().Logger().Level(level)

	nonceStore := store.NewNonceStore(cfg.NonceExpiry)
	defer nonceStore.Close()

	challengeSvc := challenge.NewService(cfg)
	limiter := ratelimit.NewLimiter(cfg.RateLimit, cfg.RateBurst)
	h := handler.New(cfg, challengeSvc, nonceStore, log)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/challenge", h.GetChallenge)
	mux.HandleFunc("/api/verify", h.VerifyChallenge)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})
	mux.Handle("/", http.FileServer(http.Dir("web")))

	var srv http.Handler = mux
	srv = limiter.Middleware(srv)
	srv = middleware.CORS(srv)
	srv = middleware.Logging(log)(srv)
	srv = middleware.Recovery(log)(srv)

	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      srv,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Info().Int("port", cfg.Port).Msg("server starting")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("server failed")
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info().Msg("shutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Fatal().Err(err).Msg("server shutdown failed")
	}
	log.Info().Msg("server stopped")
}
