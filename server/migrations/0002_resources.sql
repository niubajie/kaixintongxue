-- =====================================================================
-- 0002_resources.sql — 资源库、联系方式密文、系统配置
--
-- 幂等：全部使用 IF NOT EXISTS / ON CONFLICT。
-- 回滚见 0002_resources_rollback.sql（生产先备份再执行）。
--
-- 约束（AGENTS.md）：
--   - 软删除：deleted_at IS NULL
--   - 联系方式独立表，与主表物理隔离
--   - 金额整数分 BIGINT
--   - 时间 timestamptz
-- =====================================================================

CREATE SCHEMA IF NOT EXISTS circle;
SET search_path TO circle, public;

-- ---------------------------------------------------------------------
-- resources — 资源主表（脱敏，检索服务只读本表）
-- ---------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS circle.resources (
    id              BIGSERIAL PRIMARY KEY,
    uuid            UUID NOT NULL DEFAULT gen_random_uuid(),
    owner_user_id   BIGINT NOT NULL REFERENCES circle.users(id),
    category_code   TEXT NOT NULL REFERENCES circle.resource_categories(code),
    title           TEXT NOT NULL,
    summary         TEXT NOT NULL DEFAULT '',
    detail_md       TEXT NOT NULL DEFAULT '',
    region_code     TEXT NOT NULL DEFAULT '',           -- 行政区划码，如 440300
    provider_type   TEXT NOT NULL DEFAULT 'enterprise'
                    CHECK (provider_type IN ('enterprise','individual','broker')),
    contact_mode    TEXT NOT NULL DEFAULT 'apply'
                    CHECK (contact_mode IN ('apply')),   -- 仅支持申请后可见
    status          TEXT NOT NULL DEFAULT 'draft'
                    CHECK (status IN ('draft','pending','online','rejected','offline','expired')),
    quality_score   INT NOT NULL DEFAULT 0 CHECK (quality_score BETWEEN 0 AND 100),
    dedup_hmac      TEXT NOT NULL,                       -- 强去重：HMAC(分类|地区|规范化标题)
    content_simhash BIGINT NOT NULL DEFAULT 0,           -- 弱去重：内容 SimHash
    reject_reason   TEXT,
    expires_at      TIMESTAMPTZ,
    published_at    TIMESTAMPTZ,
    reviewed_by     TEXT,
    reviewed_at     TIMESTAMPTZ,
    deleted_at      TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_resources_uuid ON circle.resources(uuid);

-- 强去重：仅对未删除且处于 pending/online 的记录生效
CREATE UNIQUE INDEX IF NOT EXISTS uq_resources_dedup
    ON circle.resources(dedup_hmac)
    WHERE deleted_at IS NULL AND status IN ('pending','online');

CREATE INDEX IF NOT EXISTS idx_resources_cat_status
    ON circle.resources(category_code, status, published_at DESC)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_resources_title_trgm
    ON circle.resources USING gin (title gin_trgm_ops);

CREATE INDEX IF NOT EXISTS idx_resources_summary_trgm
    ON circle.resources USING gin (summary gin_trgm_ops);

CREATE INDEX IF NOT EXISTS idx_resources_owner
    ON circle.resources(owner_user_id, status, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_resources_expire
    ON circle.resources(expires_at)
    WHERE status = 'online' AND deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_resources_simhash
    ON circle.resources(category_code, content_simhash)
    WHERE deleted_at IS NULL;

-- ---------------------------------------------------------------------
-- resource_contacts — 联系方式密文（极高敏感，业务层不直接查询）
-- ---------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS circle.resource_contacts (
    id             BIGSERIAL PRIMARY KEY,
    resource_id    BIGINT NOT NULL UNIQUE REFERENCES circle.resources(id),
    contact_name   TEXT NOT NULL DEFAULT '',
    phone_cipher   BYTEA,
    phone_hmac     TEXT,
    phone_nonce    BYTEA,
    wechat_cipher  BYTEA,
    wechat_hmac    TEXT,
    wechat_nonce   BYTEA,
    dek_wrapped    BYTEA,                               -- KMS 包装后的 DEK（S2 预留）
    key_id         TEXT NOT NULL DEFAULT 'env_s1',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_contacts_phone_hmac
    ON circle.resource_contacts(phone_hmac);

-- ---------------------------------------------------------------------
-- settings — 系统配置（质量分权重、去重阈值、默认有效期等）
-- ---------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS circle.settings (
    key        TEXT PRIMARY KEY,
    value_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    updated_by TEXT,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO circle.settings(key, value_json) VALUES
  ('resource.default_expire_days', '90'::jsonb),
  ('resource.simhash_hanning_threshold', '3'::jsonb),
  ('resource.quality_threshold', '50'::jsonb),
  ('resource.review_sla_hours', '24'::jsonb)
ON CONFLICT (key) DO NOTHING;

-- 分类默认定价（联系方式单条价 / 分类包期价，单位：分）
UPDATE circle.resource_categories SET contact_price_cents = 990,  access_price_cents = 9900  WHERE code = 'factory_office'   AND contact_price_cents = 0;
UPDATE circle.resource_categories SET contact_price_cents = 1990, access_price_cents = 19900 WHERE code = 'partner_talent'   AND contact_price_cents = 0;
UPDATE circle.resource_categories SET contact_price_cents = 990,  access_price_cents = 9900  WHERE code = 'industrial_product' AND contact_price_cents = 0;
UPDATE circle.resource_categories SET contact_price_cents = 990,  access_price_cents = 9900  WHERE code = 'commodity'        AND contact_price_cents = 0;
UPDATE circle.resource_categories SET contact_price_cents = 1290, access_price_cents = 12900 WHERE code = 'business_service' AND contact_price_cents = 0;
UPDATE circle.resource_categories SET contact_price_cents = 2990, access_price_cents = 29900 WHERE code = 'ip_trade'         AND contact_price_cents = 0;
UPDATE circle.resource_categories SET contact_price_cents = 4990, access_price_cents = 49900 WHERE code = 'finance'          AND contact_price_cents = 0;
UPDATE circle.resource_categories SET contact_price_cents = 990,  access_price_cents = 9900  WHERE code = 'other'            AND contact_price_cents = 0;
