-- 知识库版本化（v4）
-- 借鉴 Rakazo 的记忆修订表：文档保留历史版本，重新索引不会把旧内容直接冲掉。

ALTER TABLE kb_documents ADD COLUMN revision INTEGER NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS kb_revisions (
    doc_id     TEXT NOT NULL,
    revision   INTEGER NOT NULL,
    content    TEXT NOT NULL,
    chunks     INTEGER NOT NULL DEFAULT 0,
    indexed_at TEXT NOT NULL,
    source     TEXT NOT NULL DEFAULT 'index',  -- index / rollback
    PRIMARY KEY (doc_id, revision)
);

CREATE INDEX IF NOT EXISTS idx_kb_revisions_doc ON kb_revisions(doc_id, revision DESC);
