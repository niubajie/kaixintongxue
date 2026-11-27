-- =====================================================================
-- 0001_init.sql — initial schema for gocircle (独立 schema `circle`)
--
-- Rules from AGENTS.md:
--   - IDs: internal BIGINT serial, external uuidv7
--   - Phone / contact info encrypted at rest
--   - All business tables soft-deletable (deleted_at timestamptz NULL)
--   - Every query must include user_id unless admin
--   - No batch export API; each decryption writes audit_logs
--
-- Apply: psql "DATABASE_URL" -f migrations/0001_init.sql
-- Rollback (manual, never modify this file): psql "DATABASE_URL" -f migrations/0001_init_rollback.sql
-- =====================================================================

CREATE EXTENSION IF NOT EXISTS "pgcrypto";
CREATE EXTENSION IF NOT EXISTS "pg_trgm";

CREATE SCHEMA IF NOT EXISTS circle;
SET search_path TO circle, public;

-- ---------------------------------------------------------------------
-- users / wechat_accounts / user_private
-- ---------------------------------------------------------------------

CREATE TABLE circle.users (
    id           BIGSERIAL PRIMARY KEY,
    uuid         UUID NOT NULL DEFAULT gen_random_uuid(),
    nickname     TEXT,
    avatar_url   TEXT,
    status       TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','blocked','pending_delete')),
    risk_level   TEXT NOT NULL DEFAULT 'normal' CHECK (risk_level IN ('normal','low','medium','high')),
    last_login_at TIMESTAMPTZ,
    deleted_at   TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX uq_users_uuid ON circle.users(uuid);
CREATE INDEX idx_users_status ON circle.users(status) WHERE deleted_at IS NULL;

CREATE TABLE circle.wechat_accounts (
    id         BIGSERIAL PRIMARY KEY,
    user_id    BIGINT NOT NULL REFERENCES circle.users(id),
    open_id    TEXT NOT NULL,
    union_id   TEXT,
    app_type   TEXT NOT NULL DEFAULT 'mini' CHECK (app_type IN ('mini','web')),
    last_session_key TEXT, -- encrypted; only used server-side for decrypting getPhoneNumber
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(open_id, app_type)
);
CREATE INDEX idx_wechat_user ON circle.wechat_accounts(user_id);

-- user_private holds encrypted phone. DEK wrapped by KMS (or dev fallback).
-- phone_hmac is for dedup / lookup without decrypting.
CREATE TABLE circle.user_private (
    id             BIGSERIAL PRIMARY KEY,
    user_id        BIGINT NOT NULL UNIQUE REFERENCES circle.users(id),
    phone_cipher   BYTEA NOT NULL,
    phone_hmac     TEXT NOT NULL,
    dek_wrapped    BYTEA NOT NULL,
    key_id         TEXT NOT NULL,       -- which KMS key version wrapped DEK
    aes_gcm_nonce  BYTEA NOT NULL,
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ---------------------------------------------------------------------
-- sessions / ip_registrations / risk_events
-- ---------------------------------------------------------------------

CREATE TABLE circle.sessions (
    id            BIGSERIAL PRIMARY KEY,
    user_id       BIGINT NOT NULL REFERENCES circle.users(id),
    token_hash    TEXT NOT NULL,           -- SHA-256 of access token
    refresh_hash  TEXT NOT NULL,           -- SHA-256 of refresh token opaque part
    device_hash   TEXT NOT NULL,           -- opaque device fingerprint
    created_ip    INET,
    expires_at    TIMESTAMPTZ NOT NULL,
    revoked_at    TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_sessions_user ON circle.sessions(user_id);
CREATE INDEX idx_sessions_hash ON circle.sessions(token_hash);

CREATE TABLE circle.ip_registrations (
    id         BIGSERIAL PRIMARY KEY,
    ip         INET NOT NULL,
    open_id    TEXT NOT NULL,
    user_id    BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_ipregs_ip_time ON circle.ip_registrations(ip, created_at);
CREATE INDEX idx_ipregs_openid ON circle.ip_registrations(open_id);

CREATE TABLE circle.risk_events (
    id          BIGSERIAL PRIMARY KEY,
    event_type  TEXT NOT NULL,
    severity    TEXT NOT NULL CHECK (severity IN ('low','medium','high','critical')),
    user_id     BIGINT,
    ip          INET,
    device_hash TEXT,
    detail_json JSONB,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_risk_user ON circle.risk_events(user_id) WHERE user_id IS NOT NULL;
CREATE INDEX idx_risk_ip ON circle.risk_events(ip) WHERE ip IS NOT NULL;
CREATE INDEX idx_risk_time ON circle.risk_events(created_at DESC);

-- ---------------------------------------------------------------------
-- admin_users / admin_roles — management backend
-- ---------------------------------------------------------------------

CREATE TABLE circle.admin_roles (
    id          BIGSERIAL PRIMARY KEY,
    role_code   TEXT NOT NULL UNIQUE,   -- e.g. 'admin_operator', 'reviewer', 'finance', 'super_admin', 'auditor'
    role_name   TEXT NOT NULL,
    permissions TEXT[] NOT NULL DEFAULT ARRAY[]::TEXT[],
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE circle.admin_users (
    id           BIGSERIAL PRIMARY KEY,
    username     TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,   -- argon2id
    role_code    TEXT NOT NULL REFERENCES circle.admin_roles(role_code),
    status       TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','disabled')),
    ip_whitelist INET[],
    tfa_enabled  BOOLEAN NOT NULL DEFAULT FALSE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_login_at TIMESTAMPTZ
);

-- ---------------------------------------------------------------------
-- audit_logs — immutable append-only chain
-- Each row carries prev_hash of the previous row to enable tamper detection.
-- ---------------------------------------------------------------------

CREATE TABLE circle.audit_logs (
    id          BIGSERIAL PRIMARY KEY,
    actor_type  TEXT NOT NULL,           -- 'user' | 'admin' | 'system'
    actor_id    TEXT NOT NULL,
    action      TEXT NOT NULL,           -- 'LOGIN' | 'PHONE_BIND' | 'CONTACT_DECRYPT' | ...
    target_type TEXT NOT NULL,
    target_id   TEXT NOT NULL,
    trace_id    TEXT,
    detail_json JSONB,                    -- NEVER store plaintext phone/token
    prev_hash   TEXT NOT NULL DEFAULT '',
    hash        TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX uq_audit_hash ON circle.audit_logs(hash);
CREATE INDEX idx_audit_time ON circle.audit_logs(created_at DESC);
CREATE INDEX idx_audit_actor ON circle.audit_logs(actor_type, actor_id, created_at DESC);
CREATE INDEX idx_audit_action ON circle.audit_logs(action);

-- ---------------------------------------------------------------------
-- categories seed (fixed 8 categories per 4.1)
-- ---------------------------------------------------------------------

CREATE TABLE circle.resource_categories (
    code            TEXT PRIMARY KEY,
    name            TEXT NOT NULL,
    enabled         BOOLEAN NOT NULL DEFAULT TRUE,
    sort_order      INT NOT NULL DEFAULT 0,
    contact_price_cents  BIGINT NOT NULL DEFAULT 0,
    access_price_cents   BIGINT NOT NULL DEFAULT 0,
    sensitive_level TEXT NOT NULL DEFAULT 'low' CHECK (sensitive_level IN ('low','medium','high','extreme')),
    disclaimer_text TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO circle.resource_categories(code, name, sort_order, sensitive_level, disclaimer_text) VALUES
  ('factory_office',       '厂房/办公室租赁与销售', 1, 'medium', NULL),
  ('partner_talent',       '合伙人力资源',          2, 'high',   '人力资源服务资质正在申请中'),
  ('industrial_product',   '工业产品',              3, 'low',    NULL),
  ('commodity',            '商品',                  4, 'low',    NULL),
  ('business_service',     '商务服务',              5, 'medium', NULL),
  ('ip_trade',             '知识产权交易',          6, 'high',   NULL),
  ('finance',              '金融类',                7, 'extreme','本平台仅发布企业信息，不提供投融资/借贷中介服务'),
  ('other',                '其它资源',              8, 'medium', NULL)
ON CONFLICT (code) DO NOTHING;

-- Default roles
INSERT INTO circle.admin_roles(role_code, role_name, permissions) VALUES
  ('super_admin',     '超级管理员', ARRAY['*']),
  ('admin_operator',  '运营管理员', ARRAY['category.*','resource.*','quota.*','risk.*','user.read']),
  ('reviewer',        '审核员',     ARRAY['resource.review','resource.read','complaint.*']),
  ('finance',         '财务',       ARRAY['order.read','payment.read','refund.issue','reconcile.*']),
  ('auditor',         '审计员',     ARRAY['audit.read','risk.read'])
ON CONFLICT (role_code) DO NOTHING;
