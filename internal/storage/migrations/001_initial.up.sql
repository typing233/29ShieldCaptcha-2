CREATE TABLE IF NOT EXISTS verification_logs (
    id              BIGSERIAL PRIMARY KEY,
    challenge_id    VARCHAR(64) NOT NULL,
    fingerprint     VARCHAR(64) NOT NULL,
    ip_address      INET NOT NULL,
    result          VARCHAR(20) NOT NULL,
    risk_score      REAL,
    hit_reasons     JSONB,
    behavior_data   JSONB,
    duration_ms     INTEGER,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_verif_logs_fingerprint ON verification_logs(fingerprint);
CREATE INDEX idx_verif_logs_ip ON verification_logs(ip_address);
CREATE INDEX idx_verif_logs_created ON verification_logs(created_at);
CREATE INDEX idx_verif_logs_result ON verification_logs(result);

CREATE TABLE IF NOT EXISTS risk_rules (
    id              SERIAL PRIMARY KEY,
    name            VARCHAR(128) NOT NULL UNIQUE,
    description     TEXT,
    condition_json  JSONB NOT NULL,
    action          VARCHAR(32) NOT NULL,
    weight          REAL NOT NULL DEFAULT 1.0,
    enabled         BOOLEAN NOT NULL DEFAULT true,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS admin_users (
    id              SERIAL PRIMARY KEY,
    username        VARCHAR(64) NOT NULL UNIQUE,
    password_hash   VARCHAR(256) NOT NULL,
    role            VARCHAR(32) NOT NULL DEFAULT 'viewer',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS audit_logs (
    id              BIGSERIAL PRIMARY KEY,
    user_id         INTEGER REFERENCES admin_users(id),
    username        VARCHAR(64),
    action          VARCHAR(64) NOT NULL,
    resource        VARCHAR(128),
    details         JSONB,
    ip_address      INET,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_audit_logs_user ON audit_logs(user_id);
CREATE INDEX idx_audit_logs_created ON audit_logs(created_at);
CREATE INDEX idx_audit_logs_action ON audit_logs(action);

CREATE TABLE IF NOT EXISTS experiments (
    id              SERIAL PRIMARY KEY,
    name            VARCHAR(128) NOT NULL,
    description     TEXT,
    traffic_pct     INTEGER NOT NULL DEFAULT 50,
    config_a        JSONB NOT NULL DEFAULT '{}',
    config_b        JSONB NOT NULL DEFAULT '{}',
    status          VARCHAR(20) NOT NULL DEFAULT 'draft',
    results         JSONB DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS widget_config (
    id              SERIAL PRIMARY KEY,
    name            VARCHAR(64) NOT NULL UNIQUE DEFAULT 'default',
    config          JSONB NOT NULL DEFAULT '{}',
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO widget_config (name, config) VALUES ('default', '{"primaryColor":"#1890ff","sliderShape":"round","width":380,"height":48}')
ON CONFLICT DO NOTHING;
