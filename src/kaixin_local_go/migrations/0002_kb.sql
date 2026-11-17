-- 本地知识库（v2）
-- 说明：迁移只能新增，不能改历史（AGENTS 第 8 节）。

-- 已加入知识库的文档
CREATE TABLE IF NOT EXISTS kb_documents (
    id         TEXT PRIMARY KEY,          -- 文件绝对路径（唯一）
    name       TEXT NOT NULL,
    size       INTEGER NOT NULL DEFAULT 0,
    mtime      TEXT NOT NULL DEFAULT '',
    chunks     INTEGER NOT NULL DEFAULT 0,
    status     TEXT NOT NULL,             -- indexed / failed / unsupported
    error      TEXT NOT NULL DEFAULT '',
    indexed_at TEXT
);

-- 切片：目前按字符数切分，后续接向量时在这里加 embedding 列
CREATE TABLE IF NOT EXISTS kb_chunks (
    doc_id TEXT NOT NULL,
    idx    INTEGER NOT NULL,
    text   TEXT NOT NULL,
    PRIMARY KEY (doc_id, idx)
);

CREATE INDEX IF NOT EXISTS idx_kb_chunks_doc ON kb_chunks(doc_id);
