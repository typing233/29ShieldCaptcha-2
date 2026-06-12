package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	ChallengesIssued = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "shieldcaptcha",
		Name:      "challenges_issued_total",
		Help:      "Total number of challenges issued",
	})

	VerificationsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "shieldcaptcha",
		Name:      "verifications_total",
		Help:      "Total verification attempts by result",
	}, []string{"result"})

	VerificationDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Namespace: "shieldcaptcha",
		Name:      "verification_duration_seconds",
		Help:      "Duration of verification requests",
		Buckets:   []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0},
	})

	RiskScoreHistogram = promauto.NewHistogram(prometheus.HistogramOpts{
		Namespace: "shieldcaptcha",
		Name:      "risk_score",
		Help:      "Distribution of risk scores",
		Buckets:   []float64{0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 1.0},
	})

	ActiveSessions = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "shieldcaptcha",
		Name:      "active_sessions",
		Help:      "Number of active challenge sessions",
	})

	RulesLoaded = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "shieldcaptcha",
		Name:      "rules_loaded",
		Help:      "Number of risk rules currently loaded",
	})

	RedisOps = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "shieldcaptcha",
		Name:      "redis_ops_total",
		Help:      "Redis operations by type and status",
	}, []string{"op", "status"})

	RateLimitHits = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "shieldcaptcha",
		Name:      "rate_limit_hits_total",
		Help:      "Rate limit hits by dimension",
	}, []string{"dimension"})

	HTTPRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "shieldcaptcha",
		Name:      "http_requests_total",
		Help:      "Total HTTP requests by method, path, and status",
	}, []string{"method", "path", "status"})

	HTTPRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "shieldcaptcha",
		Name:      "http_request_duration_seconds",
		Help:      "HTTP request latency",
		Buckets:   prometheus.DefBuckets,
	}, []string{"method", "path"})
)
