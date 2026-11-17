package main

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

// 本地知识库：先做「文本切片 + 关键词检索」，不假装有向量。
// 接向量时只需在 kb_chunks 加 embedding 列，接口与界面不用改。

const (
	chunkSize    = 400 // 每个切片的字符数
	chunkOverlap = 60  // 相邻切片重叠，避免切断上下文
	maxIndexSize = 8 << 20
	searchLimit  = 20
)

// 可索引的纯文本类型；其余类型如实标记为不支持。
var indexableExt = map[string]bool{
	".txt": true, ".md": true, ".markdown": true,
	".csv": true, ".json": true, ".log": true,
	".yaml": true, ".yml": true, ".ini": true,
}

type KBStatus struct {
	Path      string `json:"path"`
	Name      string `json:"name"`
	Chunks    int    `json:"chunks"`
	Status    string `json:"status"` // indexed / failed / unsupported / absent
	Error     string `json:"error"`
	IndexedAt string `json:"indexedAt"`
}

type KBSearchHit struct {
	Path    string `json:"path"`
	Name    string `json:"name"`
	Idx     int    `json:"idx"`
	Snippet string `json:"snippet"`
}

type KBBatchResult struct {
	OK      int        `json:"ok"`
	Failed  int        `json:"failed"`
	Skipped int        `json:"skipped"`
	Results []KBStatus `json:"results"`
}

func kbDocID(path string) string { return path }

func chunkText(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	runes := []rune(text)
	if len(runes) == 0 {
		return nil
	}
	if len(runes) <= chunkSize {
		return []string{string(runes)}
	}
	out := []string{}
	step := chunkSize - chunkOverlap
	if step <= 0 {
		step = chunkSize
	}
	for start := 0; start < len(runes); start += step {
		end := start + chunkSize
		if end > len(runes) {
			end = len(runes)
		}
		piece := strings.TrimSpace(string(runes[start:end]))
		if piece != "" {
			out = append(out, piece)
		}
		if end == len(runes) {
			break
		}
	}
	return out
}

// IndexFile 把一个文件切分入本地库。返回值描述结果，不抛错给调用方。
func (s *Store) IndexFile(ctx context.Context, path string) KBStatus {
	status := KBStatus{Path: path, Name: filepath.Base(path)}

	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		status.Status = "failed"
		status.Error = "文件不存在或不是文件"
		return status
	}
	ext := strings.ToLower(filepath.Ext(path))
	if !indexableExt[ext] {
		// 不支持的类型不写库，如实返回，避免界面显示假状态
		status.Status = "unsupported"
		status.Error = "该类型暂不支持索引（当前支持纯文本类）"
		return status
	}
	if info.Size() > maxIndexSize {
		status.Status = "failed"
		status.Error = fmt.Sprintf("文件过大（%.1f MB），超过 8 MB 上限", float64(info.Size())/(1<<20))
		return status
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		status.Status = "failed"
		status.Error = "读取失败：" + err.Error()
		return status
	}
	if !utf8.Valid(raw) {
		status.Status = "failed"
		status.Error = "不是 UTF-8 文本，暂不支持编码转换"
		return status
	}

	chunks := chunkText(string(raw))
	if len(chunks) == 0 {
		status.Status = "failed"
		status.Error = "文件内容为空"
		return status
	}

	id := kbDocID(path)
	now := time.Now().UTC().Format(time.RFC3339)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		status.Status = "failed"
		status.Error = "开启事务失败：" + err.Error()
		return status
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM kb_chunks WHERE doc_id = ?`, id); err != nil {
		status.Status = "failed"
		status.Error = "清理旧切片失败：" + err.Error()
		return status
	}
	for i, text := range chunks {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO kb_chunks(doc_id, idx, text) VALUES(?, ?, ?)`, id, i, text); err != nil {
			status.Status = "failed"
			status.Error = "写入切片失败：" + err.Error()
			return status
		}
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO kb_documents(id, name, size, mtime, chunks, status, error, indexed_at)
		 VALUES(?, ?, ?, ?, ?, 'indexed', '', ?)
		 ON CONFLICT(id) DO UPDATE SET
		   name = excluded.name, size = excluded.size, mtime = excluded.mtime,
		   chunks = excluded.chunks, status = 'indexed', error = '', indexed_at = excluded.indexed_at`,
		id, status.Name, info.Size(), info.ModTime().UTC().Format(time.RFC3339), len(chunks), now); err != nil {
		status.Status = "failed"
		status.Error = "写入文档记录失败：" + err.Error()
		return status
	}
	if err := tx.Commit(); err != nil {
		status.Status = "failed"
		status.Error = "提交失败：" + err.Error()
		return status
	}

	status.Status = "indexed"
	status.Chunks = len(chunks)
	status.IndexedAt = now
	return status
}

// UnindexFile 从知识库移除；文件本身不动。
func (s *Store) UnindexFile(ctx context.Context, path string) error {
	id := kbDocID(path)
	if _, err := s.db.ExecContext(ctx, `DELETE FROM kb_chunks WHERE doc_id = ?`, id); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM kb_documents WHERE id = ?`, id)
	return err
}

// KBStatusAll 返回全部已索引文档，供界面判断某个文件是否已在库中。
func (s *Store) KBStatusAll(ctx context.Context) ([]KBStatus, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, chunks, status, error, COALESCE(indexed_at,'') FROM kb_documents`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []KBStatus{}
	for rows.Next() {
		var item KBStatus
		if err := rows.Scan(&item.Path, &item.Name, &item.Chunks, &item.Status, &item.Error, &item.IndexedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) KBStatusOf(ctx context.Context, path string) KBStatus {
	var item KBStatus
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, chunks, status, error, COALESCE(indexed_at,'') FROM kb_documents WHERE id = ?`,
		kbDocID(path)).Scan(&item.Path, &item.Name, &item.Chunks, &item.Status, &item.Error, &item.IndexedAt)
	if err == sql.ErrNoRows {
		return KBStatus{Path: path, Status: "absent"}
	}
	if err != nil {
		return KBStatus{Path: path, Status: "failed", Error: err.Error()}
	}
	return item
}

// SearchKB 关键词检索（多词 AND）。返回命中切片与摘录。
func (s *Store) SearchKB(ctx context.Context, query string) ([]KBSearchHit, error) {
	terms := strings.Fields(strings.TrimSpace(query))
	if len(terms) == 0 {
		return []KBSearchHit{}, nil
	}

	where := make([]string, 0, len(terms))
	args := make([]any, 0, len(terms)+1)
	for _, term := range terms {
		where = append(where, "c.text LIKE ?")
		args = append(args, "%"+term+"%")
	}
	args = append(args, searchLimit)

	sqlText := `SELECT c.doc_id, d.name, c.idx, c.text
	              FROM kb_chunks c JOIN kb_documents d ON d.id = c.doc_id
	             WHERE ` + strings.Join(where, " AND ") + `
	             ORDER BY c.doc_id, c.idx LIMIT ?`

	rows, err := s.db.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	hits := []KBSearchHit{}
	for rows.Next() {
		var (
			path, name, text string
			idx              int
		)
		if err := rows.Scan(&path, &name, &idx, &text); err != nil {
			return nil, err
		}
		hits = append(hits, KBSearchHit{Path: path, Name: name, Idx: idx, Snippet: snippet(text, terms[0], 80)})
	}
	return hits, rows.Err()
}

// snippet 截取命中词附近的文本，便于界面展示。
func snippet(text, term string, width int) string {
	runes := []rune(text)
	lower := strings.ToLower(text)
	at := strings.Index(lower, strings.ToLower(term))
	if at < 0 {
		if len(runes) > width {
			return string(runes[:width]) + "…"
		}
		return text
	}
	// 字节位置换算成 rune 位置，避免截断多字节字符
	runeAt := utf8.RuneCountInString(text[:at])
	start := runeAt - width/3
	if start < 0 {
		start = 0
	}
	end := start + width
	if end > len(runes) {
		end = len(runes)
	}
	out := string(runes[start:end])
	if start > 0 {
		out = "…" + out
	}
	if end < len(runes) {
		out += "…"
	}
	return out
}

// ---------- HTTP ----------

func (a *App) handleKBIndex(w http.ResponseWriter, r *http.Request) {
	if a.store == nil {
		fail(w, http.StatusServiceUnavailable, "本地库未初始化，无法建立索引")
		return
	}
	var request struct {
		Paths []string `json:"paths"`
		Path  string   `json:"path"`
	}
	if err := decodeJSON(r, &request); err != nil {
		fail(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	paths := request.Paths
	if len(paths) == 0 && request.Path != "" {
		paths = []string{request.Path}
	}
	if len(paths) == 0 {
		fail(w, http.StatusBadRequest, "请指定要索引的文件")
		return
	}

	root := a.resolveLibraryRoot()
	batch := KBBatchResult{Results: []KBStatus{}}
	for _, path := range paths {
		if err := ensureInside(root, path); err != nil {
			batch.Results = append(batch.Results, KBStatus{
				Path: path, Name: filepath.Base(path), Status: "failed", Error: err.Error(),
			})
			batch.Failed++
			continue
		}
		result := a.store.IndexFile(r.Context(), path)
		batch.Results = append(batch.Results, result)
		switch result.Status {
		case "indexed":
			batch.OK++
		case "unsupported":
			batch.Skipped++
		default:
			batch.Failed++
		}
	}
	writeJSON(w, http.StatusOK, batch)
}

func (a *App) handleKBRemove(w http.ResponseWriter, r *http.Request) {
	if a.store == nil {
		fail(w, http.StatusServiceUnavailable, "本地库未初始化")
		return
	}
	var request struct {
		Paths []string `json:"paths"`
		Path  string   `json:"path"`
	}
	if err := decodeJSON(r, &request); err != nil {
		fail(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	paths := request.Paths
	if len(paths) == 0 && request.Path != "" {
		paths = []string{request.Path}
	}
	if len(paths) == 0 {
		fail(w, http.StatusBadRequest, "缺少文件路径")
		return
	}

	removed := []string{}
	for _, path := range paths {
		if err := a.store.UnindexFile(r.Context(), path); err != nil {
			continue
		}
		removed = append(removed, path)
	}
	writeJSON(w, http.StatusOK, map[string]any{"removed": removed, "count": len(removed)})
}

func (a *App) handleKBStatus(w http.ResponseWriter, r *http.Request) {
	if a.store == nil {
		writeJSON(w, http.StatusOK, map[string]any{"documents": []KBStatus{}})
		return
	}
	items, err := a.store.KBStatusAll(r.Context())
	if err != nil {
		fail(w, http.StatusInternalServerError, "读取知识库状态失败："+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"documents": items})
}

func (a *App) handleKBSearch(w http.ResponseWriter, r *http.Request) {
	if a.store == nil {
		fail(w, http.StatusServiceUnavailable, "本地库未初始化")
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	hits, err := a.store.SearchKB(r.Context(), query)
	if err != nil {
		fail(w, http.StatusInternalServerError, "检索失败："+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"query": query, "hits": hits})
}
