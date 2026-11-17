package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

// 知识库版本化：借鉴 Rakazo 的记忆修订设计。
// 重新索引不再直接冲掉旧内容——每次都追加一个修订，可查历史、可回滚。
// 检索仍走关键词方案，版本化只解决「改坏了能不能退回去」。

const kbMaxRevisions = 10 // 每个文件保留最近 10 个版本，控制体积

type KBRevision struct {
	Path      string `json:"path"`
	Revision  int    `json:"revision"`
	Chars     int    `json:"chars"`
	Chunks    int    `json:"chunks"`
	IndexedAt string `json:"indexedAt"`
	Source    string `json:"source"`
}

// IndexFile 保持原有签名，内部走带版本的写入路径。
func (s *Store) listRevisions(ctx context.Context, docID string) ([]KBRevision, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT doc_id, revision, LENGTH(content), chunks, indexed_at, source
		   FROM kb_revisions WHERE doc_id = ? ORDER BY revision DESC`, docID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []KBRevision{}
	for rows.Next() {
		var item KBRevision
		if err := rows.Scan(&item.Path, &item.Revision, &item.Chars, &item.Chunks, &item.IndexedAt, &item.Source); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// recordRevision 追加一条修订，并裁剪超出保留数量的旧版本。
func (s *Store) recordRevision(ctx context.Context, docID, content string, chunks int, source string) (int, error) {
	var current int
	err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(revision, 0) FROM kb_documents WHERE id = ?`, docID).Scan(&current)
	if err != nil {
		current = 0
	}
	next := current + 1
	now := time.Now().UTC().Format(time.RFC3339)

	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO kb_revisions(doc_id, revision, content, chunks, indexed_at, source)
		 VALUES(?, ?, ?, ?, ?, ?)`,
		docID, next, content, chunks, now, source); err != nil {
		return 0, err
	}
	// 只保留最近 N 个版本
	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM kb_revisions WHERE doc_id = ? AND revision <= ?`,
		docID, next-kbMaxRevisions); err != nil {
		return 0, err
	}
	// 文档行的 revision 与修订表保持一致
	if _, err := s.db.ExecContext(ctx,
		`UPDATE kb_documents SET revision = ? WHERE id = ?`, next, docID); err != nil {
		return 0, err
	}
	return next, nil
}

// indexText 是带版本的写入核心：重建切片 + 追加修订 + 更新文档行。
func (s *Store) indexText(ctx context.Context, path, name string, size int64, mtime, content, source string) (KBStatus, error) {
	chunks := chunkText(content)
	if len(chunks) == 0 {
		return KBStatus{Path: path, Name: name, Status: "failed", Error: "文件内容为空"}, nil
	}
	id := kbDocID(path)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return KBStatus{}, err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM kb_chunks WHERE doc_id = ?`, id); err != nil {
		return KBStatus{}, err
	}
	for i, text := range chunks {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO kb_chunks(doc_id, idx, text) VALUES(?, ?, ?)`, id, i, text); err != nil {
			return KBStatus{}, err
		}
	}

	var current int
	if err := tx.QueryRowContext(ctx,
		`SELECT COALESCE(revision, 0) FROM kb_documents WHERE id = ?`, id).Scan(&current); err != nil {
		current = 0
	}
	next := current + 1
	now := time.Now().UTC().Format(time.RFC3339)

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO kb_revisions(doc_id, revision, content, chunks, indexed_at, source)
		 VALUES(?, ?, ?, ?, ?, ?)`,
		id, next, content, len(chunks), now, source); err != nil {
		return KBStatus{}, err
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM kb_revisions WHERE doc_id = ? AND revision <= ?`, id, next-kbMaxRevisions); err != nil {
		return KBStatus{}, err
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO kb_documents(id, name, size, mtime, chunks, status, error, indexed_at, revision)
		 VALUES(?, ?, ?, ?, ?, 'indexed', '', ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   name = excluded.name, size = excluded.size, mtime = excluded.mtime,
		   chunks = excluded.chunks, status = 'indexed', error = '',
		   indexed_at = excluded.indexed_at, revision = excluded.revision`,
		id, name, size, mtime, len(chunks), now, next); err != nil {
		return KBStatus{}, err
	}
	if err := tx.Commit(); err != nil {
		return KBStatus{}, err
	}

	return KBStatus{
		Path: path, Name: name, Chunks: len(chunks), Revision: next,
		Status: "indexed", IndexedAt: now,
	}, nil
}

// rollbackKB 把文档恢复到指定版本：以历史内容重新索引，并记一条 source=rollback 的修订。
func (s *Store) rollbackKB(ctx context.Context, path string, revision int) (KBStatus, error) {
	id := kbDocID(path)
	var content string
	err := s.db.QueryRowContext(ctx,
		`SELECT content FROM kb_revisions WHERE doc_id = ? AND revision = ?`, id, revision).Scan(&content)
	if err != nil {
		return KBStatus{}, fmt.Errorf("找不到第 %d 版", revision)
	}
	var name string
	var size int64
	var mtime string
	_ = s.db.QueryRowContext(ctx,
		`SELECT name, size, COALESCE(mtime,'') FROM kb_documents WHERE id = ?`, id).
		Scan(&name, &size, &mtime)
	if name == "" {
		name = filepath.Base(path)
	}
	return s.indexText(ctx, path, name, size, mtime, content, "rollback")
}

// ---------- HTTP ----------

func (a *App) handleKBRevisions(w http.ResponseWriter, r *http.Request) {
	if a.store == nil {
		fail(w, http.StatusServiceUnavailable, "本地库未初始化")
		return
	}
	path := strings.TrimSpace(r.URL.Query().Get("path"))
	if path == "" {
		fail(w, http.StatusBadRequest, "缺少文件路径")
		return
	}
	items, err := a.store.listRevisions(r.Context(), kbDocID(path))
	if err != nil {
		fail(w, http.StatusInternalServerError, "读取版本历史失败："+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"revisions": items})
}

func (a *App) handleKBRollback(w http.ResponseWriter, r *http.Request) {
	if a.store == nil {
		fail(w, http.StatusServiceUnavailable, "本地库未初始化")
		return
	}
	var request struct {
		Path     string `json:"path"`
		Revision int    `json:"revision"`
	}
	if err := decodeJSON(r, &request); err != nil || strings.TrimSpace(request.Path) == "" {
		fail(w, http.StatusBadRequest, "缺少文件路径")
		return
	}
	if err := ensureInside(a.resolveLibraryRoot(), request.Path); err != nil {
		fail(w, http.StatusForbidden, err.Error())
		return
	}
	result, err := a.store.rollbackKB(r.Context(), request.Path, request.Revision)
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// 防止误删：版本内容只读，没有删除接口；保留数量由 kbMaxRevisions 控制。
var _ = os.Stat
var _ = utf8.ValidString
