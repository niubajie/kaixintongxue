-- 外网搜集：待审核资料
CREATE TABLE IF NOT EXISTS pending_docs (
  id           TEXT PRIMARY KEY,
  title        TEXT NOT NULL,
  url          TEXT NOT NULL DEFAULT '',
  source       TEXT NOT NULL DEFAULT '',
  keyword      TEXT NOT NULL DEFAULT '',
  category     TEXT NOT NULL DEFAULT '未分类',
  sub_category TEXT NOT NULL DEFAULT '',
  snippet      TEXT NOT NULL DEFAULT '',
  created_at   TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_pending_docs_created ON pending_docs (created_at DESC);

-- 定时搜索任务
CREATE TABLE IF NOT EXISTS search_schedules (
  id           TEXT PRIMARY KEY,
  keyword      TEXT NOT NULL,
  category     TEXT NOT NULL DEFAULT '未分类',
  sub_category TEXT NOT NULL DEFAULT '',
  freq         TEXT NOT NULL,
  hour         INTEGER NOT NULL DEFAULT 9,
  weekday      INTEGER NOT NULL DEFAULT 1,
  day          INTEGER NOT NULL DEFAULT 1,
  enabled      INTEGER NOT NULL DEFAULT 1,
  last_run_at  TEXT NOT NULL DEFAULT '',
  created_at   TEXT NOT NULL
);
