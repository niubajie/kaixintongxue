-- 待审核资料直接落在资料库分类目录：新增审核状态与去重指纹，并建立资料黑名单
ALTER TABLE pending_docs ADD COLUMN status TEXT NOT NULL DEFAULT 'pending';
ALTER TABLE pending_docs ADD COLUMN fingerprint TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_pending_docs_fingerprint ON pending_docs (fingerprint);

CREATE TABLE IF NOT EXISTS pending_blacklist (
  id          TEXT PRIMARY KEY,
  url         TEXT NOT NULL DEFAULT '',
  title       TEXT NOT NULL DEFAULT '',
  fingerprint TEXT NOT NULL DEFAULT '',
  created_at  TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_pending_blacklist_url ON pending_blacklist (url);
CREATE INDEX IF NOT EXISTS idx_pending_blacklist_fingerprint ON pending_blacklist (fingerprint);
