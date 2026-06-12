package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"github.com/shieldcaptcha/internal/config"
	"github.com/shieldcaptcha/internal/middleware"
	"github.com/shieldcaptcha/internal/ratelimit"
	"github.com/shieldcaptcha/internal/risk"
	"github.com/shieldcaptcha/internal/storage"
	"golang.org/x/crypto/bcrypt"
)

type AdminHandler struct {
	cfg             *config.Config
	pg              *pgxpool.Pool
	redis           *storage.RedisClient
	rules           *risk.RuleEvaluator
	auth            *middleware.AuthMiddleware
	log             zerolog.Logger
	adaptiveLimiter *ratelimit.AdaptiveLimiter
}

func NewAdminHandler(cfg *config.Config, pg *pgxpool.Pool, redis *storage.RedisClient, rules *risk.RuleEvaluator, auth *middleware.AuthMiddleware, log zerolog.Logger) *AdminHandler {
	return &AdminHandler{
		cfg:   cfg,
		pg:    pg,
		redis: redis,
		rules: rules,
		auth:  auth,
		log:   log,
	}
}

func (ah *AdminHandler) SetAdaptiveLimiter(al *ratelimit.AdaptiveLimiter) {
	ah.adaptiveLimiter = al
}

func (ah *AdminHandler) Routes() chi.Router {
	r := chi.NewRouter()

	// Public
	r.Post("/login", ah.Login)

	// Protected
	r.Group(func(r chi.Router) {
		r.Use(ah.auth.Authenticate)

		r.Get("/me", ah.GetMe)
		r.Get("/dashboard/stats", ah.GetDashboardStats)
		r.Get("/logs", ah.GetLogs)
		r.Get("/logs/{id}", ah.GetLogDetail)
		r.Get("/audit", ah.GetAuditLogs)

		// Operator+
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireRole("admin", "operator"))
			r.Get("/rules", ah.GetRules)
			r.Post("/rules", ah.CreateRule)
			r.Put("/rules/{id}", ah.UpdateRule)
			r.Delete("/rules/{id}", ah.DeleteRule)
			r.Post("/rules/reload", ah.ReloadRules)
			r.Post("/unblock", ah.Unblock)
			r.Get("/config/difficulty", ah.GetDifficulty)
			r.Put("/config/difficulty", ah.SetDifficulty)
			r.Get("/config/widget", ah.GetWidgetStyle)
			r.Put("/config/widget", ah.SetWidgetStyle)
			r.Get("/experiments", ah.GetExperiments)
			r.Post("/experiments", ah.CreateExperiment)
			r.Put("/experiments/{id}", ah.UpdateExperiment)
		})

		// Admin only
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireRole("admin"))
			r.Get("/users", ah.GetUsers)
			r.Post("/users", ah.CreateUser)
		})
	})

	return r
}

// --- Auth ---

func (ah *AdminHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid request"})
		return
	}

	if ah.pg == nil {
		// Fallback: check against config
		if req.Username == ah.cfg.AdminUsername && req.Password == ah.cfg.AdminPassword {
			token, _ := ah.auth.GenerateToken(1, req.Username, "admin")
			writeJSON(w, 200, map[string]string{"token": token})
			return
		}
		writeJSON(w, 401, map[string]string{"error": "invalid credentials"})
		return
	}

	ctx := r.Context()
	var userID int
	var passwordHash, role string
	err := ah.pg.QueryRow(ctx, "SELECT id, password_hash, role FROM admin_users WHERE username = $1", req.Username).
		Scan(&userID, &passwordHash, &role)
	if err != nil {
		writeJSON(w, 401, map[string]string{"error": "invalid credentials"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(req.Password)); err != nil {
		writeJSON(w, 401, map[string]string{"error": "invalid credentials"})
		return
	}

	token, err := ah.auth.GenerateToken(userID, req.Username, role)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "token generation failed"})
		return
	}

	ah.auditLog(ctx, userID, req.Username, "login", "", nil, r)
	writeJSON(w, 200, map[string]string{"token": token, "role": role})
}

func (ah *AdminHandler) GetMe(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	writeJSON(w, 200, user)
}

// --- Dashboard ---

func (ah *AdminHandler) GetDashboardStats(w http.ResponseWriter, r *http.Request) {
	if ah.pg == nil {
		writeJSON(w, 200, map[string]interface{}{"total": 0, "passed": 0, "blocked": 0, "error_rate": 0})
		return
	}

	ctx := r.Context()
	stats := map[string]interface{}{}

	var total, passed, blocked, failed int64
	ah.pg.QueryRow(ctx, "SELECT COUNT(*) FROM verification_logs WHERE created_at > NOW() - INTERVAL '24 hours'").Scan(&total)
	ah.pg.QueryRow(ctx, "SELECT COUNT(*) FROM verification_logs WHERE result = 'pass' AND created_at > NOW() - INTERVAL '24 hours'").Scan(&passed)
	ah.pg.QueryRow(ctx, "SELECT COUNT(*) FROM verification_logs WHERE result = 'block' AND created_at > NOW() - INTERVAL '24 hours'").Scan(&blocked)
	ah.pg.QueryRow(ctx, "SELECT COUNT(*) FROM verification_logs WHERE result = 'fail' AND created_at > NOW() - INTERVAL '24 hours'").Scan(&failed)

	var avgRisk float64
	ah.pg.QueryRow(ctx, "SELECT COALESCE(AVG(risk_score), 0) FROM verification_logs WHERE created_at > NOW() - INTERVAL '24 hours'").Scan(&avgRisk)

	stats["total_24h"] = total
	stats["passed_24h"] = passed
	stats["blocked_24h"] = blocked
	stats["failed_24h"] = failed
	stats["avg_risk_score"] = avgRisk
	if total > 0 {
		stats["pass_rate"] = float64(passed) / float64(total)
		stats["block_rate"] = float64(blocked) / float64(total)
	}

	writeJSON(w, 200, stats)
}

// --- Logs ---

func (ah *AdminHandler) GetLogs(w http.ResponseWriter, r *http.Request) {
	if ah.pg == nil {
		writeJSON(w, 200, map[string]interface{}{"logs": []interface{}{}, "total": 0})
		return
	}

	ctx := r.Context()
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if page < 1 { page = 1 }
	if limit < 1 || limit > 100 { limit = 20 }
	offset := (page - 1) * limit

	ip := r.URL.Query().Get("ip")
	fp := r.URL.Query().Get("fingerprint")
	result := r.URL.Query().Get("result")

	query := "SELECT id, challenge_id, fingerprint, ip_address, result, risk_score, hit_reasons, duration_ms, created_at FROM verification_logs WHERE 1=1"
	countQuery := "SELECT COUNT(*) FROM verification_logs WHERE 1=1"
	args := []interface{}{}
	argIdx := 1

	if ip != "" {
		query += fmt.Sprintf(" AND ip_address = $%d::inet", argIdx)
		countQuery += fmt.Sprintf(" AND ip_address = $%d::inet", argIdx)
		args = append(args, ip)
		argIdx++
	}
	if fp != "" {
		query += fmt.Sprintf(" AND fingerprint = $%d", argIdx)
		countQuery += fmt.Sprintf(" AND fingerprint = $%d", argIdx)
		args = append(args, fp)
		argIdx++
	}
	if result != "" {
		query += fmt.Sprintf(" AND result = $%d", argIdx)
		countQuery += fmt.Sprintf(" AND result = $%d", argIdx)
		args = append(args, result)
		argIdx++
	}

	var total int64
	ah.pg.QueryRow(ctx, countQuery, args...).Scan(&total)

	query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := ah.pg.Query(ctx, query, args...)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "query failed"})
		return
	}
	defer rows.Close()

	type LogEntry struct {
		ID          int64           `json:"id"`
		ChallengeID string          `json:"challenge_id"`
		Fingerprint string          `json:"fingerprint"`
		IPAddress   string          `json:"ip_address"`
		Result      string          `json:"result"`
		RiskScore   *float64        `json:"risk_score"`
		HitReasons  json.RawMessage `json:"hit_reasons"`
		DurationMs  *int            `json:"duration_ms"`
		CreatedAt   time.Time       `json:"created_at"`
	}

	var logs []LogEntry
	for rows.Next() {
		var l LogEntry
		rows.Scan(&l.ID, &l.ChallengeID, &l.Fingerprint, &l.IPAddress, &l.Result, &l.RiskScore, &l.HitReasons, &l.DurationMs, &l.CreatedAt)
		logs = append(logs, l)
	}
	if logs == nil {
		logs = []LogEntry{}
	}

	writeJSON(w, 200, map[string]interface{}{
		"logs":  logs,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

func (ah *AdminHandler) GetLogDetail(w http.ResponseWriter, r *http.Request) {
	if ah.pg == nil {
		writeJSON(w, 404, map[string]string{"error": "not found"})
		return
	}
	id := chi.URLParam(r, "id")
	ctx := r.Context()

	var log struct {
		ID           int64           `json:"id"`
		ChallengeID  string          `json:"challenge_id"`
		Fingerprint  string          `json:"fingerprint"`
		IPAddress    string          `json:"ip_address"`
		Result       string          `json:"result"`
		RiskScore    *float64        `json:"risk_score"`
		HitReasons   json.RawMessage `json:"hit_reasons"`
		BehaviorData json.RawMessage `json:"behavior_data"`
		DurationMs   *int            `json:"duration_ms"`
		CreatedAt    time.Time       `json:"created_at"`
	}
	err := ah.pg.QueryRow(ctx,
		"SELECT id, challenge_id, fingerprint, ip_address, result, risk_score, hit_reasons, behavior_data, duration_ms, created_at FROM verification_logs WHERE id = $1", id).
		Scan(&log.ID, &log.ChallengeID, &log.Fingerprint, &log.IPAddress, &log.Result, &log.RiskScore, &log.HitReasons, &log.BehaviorData, &log.DurationMs, &log.CreatedAt)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, 200, log)
}

// --- Rules ---

func (ah *AdminHandler) GetRules(w http.ResponseWriter, r *http.Request) {
	if ah.pg == nil {
		writeJSON(w, 200, []interface{}{})
		return
	}
	ctx := r.Context()
	rows, err := ah.pg.Query(ctx, "SELECT id, name, description, condition_json, action, weight, enabled, created_at, updated_at FROM risk_rules ORDER BY weight DESC")
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "query failed"})
		return
	}
	defer rows.Close()

	type RuleDTO struct {
		ID          int             `json:"id"`
		Name        string          `json:"name"`
		Description *string         `json:"description"`
		Condition   json.RawMessage `json:"condition_json"`
		Action      string          `json:"action"`
		Weight      float64         `json:"weight"`
		Enabled     bool            `json:"enabled"`
		CreatedAt   time.Time       `json:"created_at"`
		UpdatedAt   time.Time       `json:"updated_at"`
	}

	var rules []RuleDTO
	for rows.Next() {
		var r RuleDTO
		rows.Scan(&r.ID, &r.Name, &r.Description, &r.Condition, &r.Action, &r.Weight, &r.Enabled, &r.CreatedAt, &r.UpdatedAt)
		rules = append(rules, r)
	}
	if rules == nil {
		rules = []RuleDTO{}
	}
	writeJSON(w, 200, rules)
}

func (ah *AdminHandler) CreateRule(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Condition   json.RawMessage `json:"condition_json"`
		Action      string          `json:"action"`
		Weight      float64         `json:"weight"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid request"})
		return
	}

	ctx := r.Context()
	var id int
	err := ah.pg.QueryRow(ctx,
		"INSERT INTO risk_rules (name, description, condition_json, action, weight) VALUES ($1, $2, $3, $4, $5) RETURNING id",
		req.Name, req.Description, req.Condition, req.Action, req.Weight).Scan(&id)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "create failed: " + err.Error()})
		return
	}

	user := middleware.GetUser(ctx)
	ah.auditLog(ctx, user.UserID, user.Username, "create_rule", req.Name, map[string]interface{}{"rule_id": id}, r)
	if ah.rules != nil {
		ah.rules.Reload()
	}
	writeJSON(w, 201, map[string]interface{}{"id": id})
}

func (ah *AdminHandler) UpdateRule(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Condition   json.RawMessage `json:"condition_json"`
		Action      string          `json:"action"`
		Weight      float64         `json:"weight"`
		Enabled     *bool           `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid request"})
		return
	}

	ctx := r.Context()
	_, err := ah.pg.Exec(ctx,
		"UPDATE risk_rules SET name=$1, description=$2, condition_json=$3, action=$4, weight=$5, enabled=COALESCE($6, enabled), updated_at=NOW() WHERE id=$7",
		req.Name, req.Description, req.Condition, req.Action, req.Weight, req.Enabled, id)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "update failed"})
		return
	}

	user := middleware.GetUser(ctx)
	ah.auditLog(ctx, user.UserID, user.Username, "update_rule", id, nil, r)
	if ah.rules != nil {
		ah.rules.Reload()
	}
	writeJSON(w, 200, map[string]string{"status": "updated"})
}

func (ah *AdminHandler) DeleteRule(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ctx := r.Context()
	_, err := ah.pg.Exec(ctx, "DELETE FROM risk_rules WHERE id = $1", id)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "delete failed"})
		return
	}

	user := middleware.GetUser(ctx)
	ah.auditLog(ctx, user.UserID, user.Username, "delete_rule", id, nil, r)
	if ah.rules != nil {
		ah.rules.Reload()
	}
	writeJSON(w, 200, map[string]string{"status": "deleted"})
}

func (ah *AdminHandler) ReloadRules(w http.ResponseWriter, r *http.Request) {
	if ah.rules != nil {
		ah.rules.Reload()
	}
	user := middleware.GetUser(r.Context())
	ah.auditLog(r.Context(), user.UserID, user.Username, "reload_rules", "", nil, r)
	writeJSON(w, 200, map[string]string{"status": "reloaded"})
}

// --- Unblock ---

func (ah *AdminHandler) Unblock(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IP          string `json:"ip"`
		Fingerprint string `json:"fingerprint"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid request"})
		return
	}

	// Clear Redis keys
	if ah.redis != nil {
		ctx := r.Context()
		if req.IP != "" {
			_ = ah.redis.Del(ctx, ah.redis.Key("arl", "ip", req.IP))
			_ = ah.redis.Del(ctx, ah.redis.Key("rl", "ip", req.IP))
			_ = ah.redis.Del(ctx, ah.redis.Key("rep", "ip_fail", req.IP))
		}
		if req.Fingerprint != "" {
			_ = ah.redis.Del(ctx, ah.redis.Key("arl", "fp", req.Fingerprint))
			_ = ah.redis.Del(ctx, ah.redis.Key("rep", "fp_fail", req.Fingerprint))
		}
	}

	// Also relax the in-memory tightened state so this process immediately allows traffic
	if ah.adaptiveLimiter != nil {
		ah.adaptiveLimiter.Unblock(req.IP, req.Fingerprint)
	}

	user := middleware.GetUser(r.Context())
	ah.auditLog(r.Context(), user.UserID, user.Username, "unblock", "", map[string]interface{}{"ip": req.IP, "fp": req.Fingerprint}, r)
	writeJSON(w, 200, map[string]string{"status": "unblocked"})
}

// --- Config ---

func (ah *AdminHandler) GetDifficulty(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]interface{}{
		"base":  ah.cfg.BaseDifficulty,
		"min":   ah.cfg.MinDifficulty,
		"max":   ah.cfg.MaxDifficulty,
	})
}

func (ah *AdminHandler) SetDifficulty(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Base int `json:"base"`
		Min  int `json:"min"`
		Max  int `json:"max"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid request"})
		return
	}
	if req.Base >= req.Min && req.Base <= req.Max {
		ah.cfg.BaseDifficulty = req.Base
		ah.cfg.MinDifficulty = req.Min
		ah.cfg.MaxDifficulty = req.Max
	}

	user := middleware.GetUser(r.Context())
	ah.auditLog(r.Context(), user.UserID, user.Username, "set_difficulty", "", map[string]interface{}{"base": req.Base, "min": req.Min, "max": req.Max}, r)
	writeJSON(w, 200, map[string]string{"status": "updated"})
}

// --- Widget Style Config ---

func (ah *AdminHandler) GetWidgetStyle(w http.ResponseWriter, r *http.Request) {
	if ah.pg == nil {
		writeJSON(w, 200, map[string]interface{}{
			"primaryColor": "#1890ff", "sliderShape": "round", "width": 380, "height": 48,
		})
		return
	}
	ctx := r.Context()
	var configJSON []byte
	err := ah.pg.QueryRow(ctx, "SELECT config FROM widget_config WHERE name = 'default' LIMIT 1").Scan(&configJSON)
	if err != nil || configJSON == nil {
		writeJSON(w, 200, map[string]interface{}{
			"primaryColor": "#1890ff", "sliderShape": "round", "width": 380, "height": 48,
		})
		return
	}
	var cfg map[string]interface{}
	if json.Unmarshal(configJSON, &cfg) != nil {
		writeJSON(w, 200, map[string]interface{}{})
		return
	}
	writeJSON(w, 200, cfg)
}

func (ah *AdminHandler) SetWidgetStyle(w http.ResponseWriter, r *http.Request) {
	var req map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid request"})
		return
	}

	if ah.pg == nil {
		writeJSON(w, 500, map[string]string{"error": "database not available"})
		return
	}

	configJSON, err := json.Marshal(req)
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid config data"})
		return
	}

	ctx := r.Context()
	_, err = ah.pg.Exec(ctx,
		`INSERT INTO widget_config (name, config, updated_at) VALUES ('default', $1, NOW())
		 ON CONFLICT (name) DO UPDATE SET config = $1, updated_at = NOW()`,
		configJSON)
	if err != nil {
		ah.log.Error().Err(err).Msg("failed to save widget config")
		writeJSON(w, 500, map[string]string{"error": "save failed"})
		return
	}

	user := middleware.GetUser(ctx)
	ah.auditLog(ctx, user.UserID, user.Username, "set_widget_style", "widget_config", req, r)
	writeJSON(w, 200, map[string]string{"status": "updated"})
}

// --- Experiments ---

func (ah *AdminHandler) GetExperiments(w http.ResponseWriter, r *http.Request) {
	if ah.pg == nil {
		writeJSON(w, 200, []interface{}{})
		return
	}
	ctx := r.Context()
	rows, err := ah.pg.Query(ctx, "SELECT id, name, description, traffic_pct, config_a, config_b, status, created_at FROM experiments ORDER BY created_at DESC")
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "query failed"})
		return
	}
	defer rows.Close()

	type ExpDTO struct {
		ID         int             `json:"id"`
		Name       string          `json:"name"`
		Desc       *string         `json:"description"`
		TrafficPct int             `json:"traffic_pct"`
		ConfigA    json.RawMessage `json:"config_a"`
		ConfigB    json.RawMessage `json:"config_b"`
		Status     string          `json:"status"`
		CreatedAt  time.Time       `json:"created_at"`
	}
	var exps []ExpDTO
	for rows.Next() {
		var e ExpDTO
		rows.Scan(&e.ID, &e.Name, &e.Desc, &e.TrafficPct, &e.ConfigA, &e.ConfigB, &e.Status, &e.CreatedAt)
		exps = append(exps, e)
	}
	if exps == nil {
		exps = []ExpDTO{}
	}
	writeJSON(w, 200, exps)
}

func (ah *AdminHandler) CreateExperiment(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name       string          `json:"name"`
		Desc       string          `json:"description"`
		TrafficPct int             `json:"traffic_pct"`
		ConfigA    json.RawMessage `json:"config_a"`
		ConfigB    json.RawMessage `json:"config_b"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid request"})
		return
	}
	ctx := r.Context()
	var id int
	err := ah.pg.QueryRow(ctx,
		"INSERT INTO experiments (name, description, traffic_pct, config_a, config_b) VALUES ($1, $2, $3, $4, $5) RETURNING id",
		req.Name, req.Desc, req.TrafficPct, req.ConfigA, req.ConfigB).Scan(&id)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "create failed"})
		return
	}

	user := middleware.GetUser(ctx)
	ah.auditLog(ctx, user.UserID, user.Username, "create_experiment", req.Name, nil, r)
	writeJSON(w, 201, map[string]interface{}{"id": id})
}

func (ah *AdminHandler) UpdateExperiment(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req struct {
		Name       string          `json:"name"`
		TrafficPct int             `json:"traffic_pct"`
		ConfigA    json.RawMessage `json:"config_a"`
		ConfigB    json.RawMessage `json:"config_b"`
		Status     string          `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid request"})
		return
	}
	ctx := r.Context()
	_, err := ah.pg.Exec(ctx,
		"UPDATE experiments SET name=$1, traffic_pct=$2, config_a=$3, config_b=$4, status=$5, updated_at=NOW() WHERE id=$6",
		req.Name, req.TrafficPct, req.ConfigA, req.ConfigB, req.Status, id)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "update failed"})
		return
	}

	user := middleware.GetUser(ctx)
	ah.auditLog(ctx, user.UserID, user.Username, "update_experiment", id, nil, r)
	writeJSON(w, 200, map[string]string{"status": "updated"})
}

// --- Users (admin only) ---

func (ah *AdminHandler) GetUsers(w http.ResponseWriter, r *http.Request) {
	if ah.pg == nil {
		writeJSON(w, 200, []interface{}{})
		return
	}
	ctx := r.Context()
	rows, _ := ah.pg.Query(ctx, "SELECT id, username, role, created_at FROM admin_users ORDER BY id")
	defer rows.Close()
	type UserDTO struct {
		ID       int       `json:"id"`
		Username string    `json:"username"`
		Role     string    `json:"role"`
		Created  time.Time `json:"created_at"`
	}
	var users []UserDTO
	for rows.Next() {
		var u UserDTO
		rows.Scan(&u.ID, &u.Username, &u.Role, &u.Created)
		users = append(users, u)
	}
	if users == nil {
		users = []UserDTO{}
	}
	writeJSON(w, 200, users)
}

func (ah *AdminHandler) CreateUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid request"})
		return
	}
	if req.Role == "" {
		req.Role = "viewer"
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "hash failed"})
		return
	}

	ctx := r.Context()
	var id int
	err = ah.pg.QueryRow(ctx, "INSERT INTO admin_users (username, password_hash, role) VALUES ($1, $2, $3) RETURNING id",
		req.Username, string(hash), req.Role).Scan(&id)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "create failed: " + err.Error()})
		return
	}

	user := middleware.GetUser(ctx)
	ah.auditLog(ctx, user.UserID, user.Username, "create_user", req.Username, map[string]interface{}{"role": req.Role}, r)
	writeJSON(w, 201, map[string]interface{}{"id": id})
}

// --- Audit ---

func (ah *AdminHandler) GetAuditLogs(w http.ResponseWriter, r *http.Request) {
	if ah.pg == nil {
		writeJSON(w, 200, map[string]interface{}{"logs": []interface{}{}, "total": 0})
		return
	}
	ctx := r.Context()
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if page < 1 { page = 1 }
	if limit < 1 || limit > 100 { limit = 20 }
	offset := (page - 1) * limit

	rows, err := ah.pg.Query(ctx,
		"SELECT id, username, action, resource, details, ip_address, created_at FROM audit_logs ORDER BY created_at DESC LIMIT $1 OFFSET $2",
		limit, offset)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "query failed"})
		return
	}
	defer rows.Close()

	type AuditEntry struct {
		ID        int64           `json:"id"`
		Username  *string         `json:"username"`
		Action    string          `json:"action"`
		Resource  *string         `json:"resource"`
		Details   json.RawMessage `json:"details"`
		IP        *string         `json:"ip_address"`
		CreatedAt time.Time       `json:"created_at"`
	}
	var logs []AuditEntry
	for rows.Next() {
		var l AuditEntry
		rows.Scan(&l.ID, &l.Username, &l.Action, &l.Resource, &l.Details, &l.IP, &l.CreatedAt)
		logs = append(logs, l)
	}
	if logs == nil {
		logs = []AuditEntry{}
	}
	writeJSON(w, 200, map[string]interface{}{"logs": logs})
}

func (ah *AdminHandler) auditLog(ctx context.Context, userID int, username, action, resource string, details map[string]interface{}, r *http.Request) {
	if ah.pg == nil {
		return
	}
	detailsJSON, _ := json.Marshal(details)
	ip := r.RemoteAddr
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		ip = xff
	}
	ah.pg.Exec(ctx,
		"INSERT INTO audit_logs (user_id, username, action, resource, details, ip_address) VALUES ($1, $2, $3, $4, $5, $6::inet)",
		userID, username, action, resource, detailsJSON, ip)
}
