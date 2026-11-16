-- 本地库初始结构（v1）
-- 设计原则见 docs/08：单一落点 %LOCALAPPDATA%\Kaixin\data\app.db，
-- 由 Go sidecar 独占，前端不直接访问数据库文件。

-- 自定义技能：用户在「新增技能」里填的名称与描述，以及匹配结果
CREATE TABLE IF NOT EXISTS custom_skills (
    id           TEXT PRIMARY KEY,
    name         TEXT NOT NULL,
    description  TEXT NOT NULL DEFAULT '',
    software_ids TEXT NOT NULL DEFAULT '[]', -- JSON 数组
    site_ids     TEXT NOT NULL DEFAULT '[]', -- JSON 数组
    created_at   TEXT NOT NULL,
    updated_at   TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_custom_skills_updated
    ON custom_skills(updated_at DESC);

-- 部署历史：每次「部署方案」执行完落一条，含逐步结果
CREATE TABLE IF NOT EXISTS deploy_runs (
    id            TEXT PRIMARY KEY,
    state         TEXT NOT NULL,             -- done / failed / cancelled
    started_at    TEXT NOT NULL,
    finished_at   TEXT,
    total_size_mb INTEGER NOT NULL DEFAULT 0,
    log_path      TEXT NOT NULL DEFAULT '',
    steps_json    TEXT NOT NULL DEFAULT '[]'
);

CREATE INDEX IF NOT EXISTS idx_deploy_runs_started
    ON deploy_runs(started_at DESC);

-- 键值设置：窗口偏好、上次选择的技能等
CREATE TABLE IF NOT EXISTS settings (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
