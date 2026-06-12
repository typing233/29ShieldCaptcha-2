package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
	"github.com/shieldcaptcha/internal/challenge"
	"github.com/shieldcaptcha/internal/config"
	"github.com/shieldcaptcha/internal/feature"
	"github.com/shieldcaptcha/internal/handler"
	"github.com/shieldcaptcha/internal/middleware"
	"github.com/shieldcaptcha/internal/ratelimit"
	"github.com/shieldcaptcha/internal/risk"
	"github.com/shieldcaptcha/internal/storage"
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

	flags := feature.NewFlags()
	flags.Set(feature.FlagRiskEngine, cfg.FeatureRiskEngine)
	flags.Set(feature.FlagBehavior, cfg.FeatureBehavior)
	flags.Set(feature.FlagAdaptiveRate, cfg.FeatureAdaptiveRate)
	flags.Set(feature.FlagReplayDetect, cfg.FeatureReplayDetect)
	flags.Set(feature.FlagAdminPanel, cfg.AdminEnabled)

	// Redis (optional)
	var redisClient *storage.RedisClient
	if cfg.EnableRedis {
		var redisErr error
		redisClient, redisErr = storage.NewRedisClient(cfg.RedisURL, cfg.RedisPrefix, log)
		if redisErr != nil {
			log.Warn().Err(redisErr).Msg("redis connection failed, using in-memory fallback")
		}
	}

	// PostgreSQL (optional)
	var pgClient *storage.PostgresClient
	if cfg.EnablePostgres {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		var pgErr error
		pgClient, pgErr = storage.NewPostgresClient(ctx, cfg.PostgresURL, log)
		cancel()
		if pgErr != nil {
			log.Warn().Err(pgErr).Msg("postgres connection failed, continuing without persistence")
		} else if cfg.AutoMigrate {
			if err := pgClient.RunMigrations(cfg.PostgresURL); err != nil {
				log.Error().Err(err).Msg("migration failed")
			}
		}
	}

	// Nonce Store (Redis-backed or in-memory fallback)
	var nonceStorer store.NonceStorer
	if redisClient != nil && redisClient.Available() {
		nonceStorer = store.NewRedisNonceStore(redisClient, cfg.NonceExpiry, log)
		log.Info().Msg("using redis-backed nonce store")
	} else {
		memStore := store.NewNonceStore(cfg.NonceExpiry)
		nonceStorer = memStore
		log.Info().Msg("using in-memory nonce store")
	}

	// Rate Limiter
	memLimiter := ratelimit.NewLimiter(cfg.RateLimit, cfg.RateBurst)
	var rateMw func(http.Handler) http.Handler
	if redisClient != nil && redisClient.Available() {
		redisLimiter := ratelimit.NewRedisLimiter(redisClient, int(cfg.RateLimit)*60, time.Minute, memLimiter, log)
		rateMw = redisLimiter.Middleware
		log.Info().Msg("using redis-backed rate limiter")
	} else {
		rateMw = memLimiter.Middleware
		log.Info().Msg("using in-memory rate limiter")
	}

	// Risk Engine
	riskEngine := risk.NewEngine(log)
	riskEngine.RegisterScorer(risk.NewBehaviorScorer())
	if redisClient != nil {
		riskEngine.RegisterScorer(risk.NewReplayScorer(redisClient, log))
		riskEngine.RegisterScorer(risk.NewReputationScorer(redisClient, log))
	}

	// Rule Evaluator
	var ruleEval *risk.RuleEvaluator
	if pgClient != nil {
		ruleEval = risk.NewRuleEvaluator(pgClient.Pool, log)
		riskEngine.SetRules(ruleEval)
	} else {
		ruleEval = risk.NewRuleEvaluator(nil, log)
	}

	challengeSvc := challenge.NewService(cfg)
	h := handler.New(cfg, challengeSvc, nonceStorer, log, flags)

	r := chi.NewRouter()

	// Global middleware
	r.Use(chimw.RealIP)
	r.Use(middleware.Recovery(log))
	r.Use(middleware.Logging(log))
	r.Use(middleware.CORS)
	if cfg.MetricsEnabled {
		r.Use(middleware.Metrics)
	}

	// Public API routes
	r.Route("/api", func(api chi.Router) {
		api.Use(rateMw)
		api.Get("/challenge", h.GetChallenge)
		api.Post("/verify", h.VerifyChallenge)
	})

	// Admin panel API
	if cfg.AdminEnabled {
		authMw := middleware.NewAuthMiddleware(cfg.JWTSecret, log)
		var pgPool *storage.PostgresClient
		if pgClient != nil {
			pgPool = pgClient
		}
		if pgPool != nil {
			adminH := handler.NewAdminHandler(cfg, pgPool.Pool, redisClient, ruleEval, authMw, log)
			r.Route("/admin/api", func(admin chi.Router) {
				admin.Mount("/", adminH.Routes())
			})
		} else {
			adminH := handler.NewAdminHandler(cfg, nil, redisClient, ruleEval, authMw, log)
			r.Route("/admin/api", func(admin chi.Router) {
				admin.Mount("/", adminH.Routes())
			})
		}
		log.Info().Msg("admin panel enabled")
	}

	// Health check
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		health := map[string]interface{}{"status": "ok"}
		if redisClient != nil {
			if redisClient.Available() {
				health["redis"] = "connected"
			} else {
				health["redis"] = "disconnected"
			}
		}
		if pgClient != nil {
			ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
			defer cancel()
			if pgClient.HealthCheck(ctx) == nil {
				health["postgres"] = "connected"
			} else {
				health["postgres"] = "disconnected"
			}
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"ok","redis":"%v","postgres":"%v"}`, health["redis"], health["postgres"])
	})

	// Prometheus metrics
	if cfg.MetricsEnabled {
		r.Handle("/metrics", promhttp.Handler())
	}

	// Static files (backward compat)
	r.Handle("/*", http.FileServer(http.Dir("web")))

	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Info().Int("port", cfg.Port).Bool("redis", cfg.EnableRedis).Bool("postgres", cfg.EnablePostgres).Msg("server starting")
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

	if ruleEval != nil {
		ruleEval.Stop()
	}
	if redisClient != nil {
		redisClient.Close()
	}
	if pgClient != nil {
		pgClient.Close()
	}
	nonceStorer.Close()

	log.Info().Msg("server stopped")
}
