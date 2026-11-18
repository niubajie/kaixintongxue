-- 模型调用用量（v5）：每次调用写一条，供「结果回显」回溯用了哪个模型、花了多少。
CREATE TABLE IF NOT EXISTS usage_records (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    model             TEXT NOT NULL,
    prompt_tokens     INTEGER NOT NULL DEFAULT 0,
    completion_tokens INTEGER NOT NULL DEFAULT 0,
    cost_cents        INTEGER NOT NULL DEFAULT 0,
    elapsed_ms        INTEGER NOT NULL DEFAULT 0,
    created_at        TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_usage_created ON usage_records(created_at DESC);
