package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// 四大类固定顺序；其余目录归入「未分类」。
var libraryCategories = []string{"工作", "生活", "学习", "爱好", "未分类"}

func mapCategory(group string) string {
	switch {
	case strings.Contains(group, "工作"):
		return "工作"
	case strings.Contains(group, "生活"):
		return "生活"
	case strings.Contains(group, "学习"):
		return "学习"
	case strings.Contains(group, "爱好"), strings.Contains(group, "兴趣"):
		return "爱好"
	default:
		return "未分类"
	}
}

func normalizeCategory(value string) string {
	for _, name := range libraryCategories {
		if value == name {
			return name
		}
	}
	return mapCategory(value)
}

type TaxonomySub struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type TaxonomyCategory struct {
	Key   string        `json:"key"`
	Name  string        `json:"name"`
	Count int           `json:"count"`
	Subs  []TaxonomySub `json:"subs"`
}

type LibraryTaxonomy struct {
	Categories []TaxonomyCategory `json:"categories"`
	Pending    int                `json:"pending"`
	Root       string             `json:"root"`
}

// buildTaxonomy 按「第一层目录=大类、第二层=子类」生成分类树。
func buildTaxonomy(scan LibraryScan) []TaxonomyCategory {
	type bucket struct {
		count int
		subs  map[string]int
	}
	buckets := map[string]*bucket{}
	for _, name := range libraryCategories {
		buckets[name] = &bucket{subs: map[string]int{}}
	}
	for _, item := range scan.Items {
		b := buckets[item.Category]
		if b == nil {
			b = buckets["未分类"]
		}
		b.count++
		if item.SubCategory != "" {
			b.subs[item.SubCategory]++
		}
	}
	out := make([]TaxonomyCategory, 0, len(libraryCategories))
	for _, name := range libraryCategories {
		b := buckets[name]
		subs := make([]TaxonomySub, 0, len(b.subs))
		for sub, count := range b.subs {
			subs = append(subs, TaxonomySub{Name: sub, Count: count})
		}
		sort.Slice(subs, func(i, j int) bool {
			if subs[i].Count != subs[j].Count {
				return subs[i].Count > subs[j].Count
			}
			return subs[i].Name < subs[j].Name
		})
		out = append(out, TaxonomyCategory{Key: name, Name: name, Count: b.count, Subs: subs})
	}
	return out
}

func (a *App) pendingCount(ctx context.Context) int {
	if a.store == nil {
		return 0
	}
	var count int
	if err := a.store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pending_docs`).Scan(&count); err != nil {
		return 0
	}
	return count
}

func (a *App) handleLibraryTaxonomy(w http.ResponseWriter, r *http.Request) {
	scan := scanLibrary(a.resolveLibraryRoot())
	writeJSON(w, http.StatusOK, LibraryTaxonomy{
		Categories: buildTaxonomy(scan),
		Pending:    a.pendingCount(r.Context()),
		Root:       scan.Root,
	})
}

type PendingDoc struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	URL         string `json:"url"`
	Source      string `json:"source"`
	Keyword     string `json:"keyword"`
	Category    string `json:"category"`
	SubCategory string `json:"subCategory"`
	Snippet     string `json:"snippet"`
	CreatedAt   string `json:"createdAt"`
}

func newRandomID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}

// sanitizeSegment 清理文件名 / 目录名中的非法字符，避免写文件失败或路径穿越。
func sanitizeSegment(value string) string {
	value = strings.TrimSpace(value)
	var builder strings.Builder
	lastSpace := false
	for _, r := range value {
		switch {
		case r == '.' || r == '-' || r == '_' || r == '·':
			builder.WriteRune(r)
			lastSpace = false
		case unicode.IsSpace(r):
			if !lastSpace {
				builder.WriteRune(' ')
				lastSpace = true
			}
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			builder.WriteRune(r)
			lastSpace = false
		case unicode.Is(unicode.Han, r):
			builder.WriteRune(r)
			lastSpace = false
		}
	}
	out := strings.Trim(builder.String(), " .")
	if utf8.RuneCountInString(out) > 60 {
		runes := []rune(out)
		out = string(runes[:60])
	}
	return out
}

// insertPending 写入一条待审核记录，返回记录 ID 便于调用方回填。
func (a *App) insertPending(ctx context.Context, keyword, category, sub string, result webResult) (string, error) {
	id := newRandomID()
	_, err := a.store.db.ExecContext(ctx,
		`INSERT INTO pending_docs (id, title, url, source, keyword, category, sub_category, snippet, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id,
		truncate(strings.TrimSpace(result.Title), 200),
		result.URL,
		result.Source,
		truncate(keyword, 120),
		normalizeCategory(category),
		sanitizeSegment(sub),
		truncate(result.Snippet, 1000),
		time.Now().UTC().Format(time.RFC3339),
	)
	return id, err
}

func (a *App) handleWebCollect(w http.ResponseWriter, r *http.Request) {
	if a.store == nil {
		fail(w, http.StatusServiceUnavailable, "本地库未初始化，无法保存搜集结果")
		return
	}
	var request struct {
		Keyword     string `json:"keyword"`
		Category    string `json:"category"`
		SubCategory string `json:"subCategory"`
		Limit       int    `json:"limit"`
	}
	if err := decodeJSON(r, &request); err != nil {
		fail(w, http.StatusBadRequest, "请求格式不正确")
		return
	}
	keyword := strings.TrimSpace(request.Keyword)
	if keyword == "" {
		fail(w, http.StatusBadRequest, "请输入搜索关键词")
		return
	}

	sites, err := a.loadSites(r.Context(), true)
	if err != nil {
		fail(w, http.StatusInternalServerError, "读取网站设置失败："+err.Error())
		return
	}
	results, provider, err := webSearchWithSites(r.Context(), keyword, request.Limit, sites)
	if err != nil {
		fail(w, http.StatusBadGateway, "外网搜集失败："+err.Error())
		return
	}

	saved := make([]PendingDoc, 0, len(results))
	for _, result := range results {
		id, err := a.insertPending(r.Context(), keyword, request.Category, request.SubCategory, result)
		if err != nil {
			fail(w, http.StatusInternalServerError, "保存搜集结果失败："+err.Error())
			return
		}
		saved = append(saved, PendingDoc{
			ID:          id,
			Title:       result.Title,
			URL:         result.URL,
			Source:      result.Source,
			Keyword:     keyword,
			Category:    normalizeCategory(request.Category),
			SubCategory: sanitizeSegment(request.SubCategory),
			Snippet:     result.Snippet,
			CreatedAt:   time.Now().UTC().Format(time.RFC3339),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"provider": provider,
		"count":    len(saved),
		"items":    saved,
		"pending":  a.pendingCount(r.Context()),
	})
}

func (a *App) handlePendingList(w http.ResponseWriter, r *http.Request) {
	if a.store == nil {
		writeJSON(w, http.StatusOK, map[string]any{"items": []PendingDoc{}, "total": 0})
		return
	}
	rows, err := a.store.db.QueryContext(r.Context(),
		`SELECT id, title, url, source, keyword, category, sub_category, snippet, created_at
		 FROM pending_docs ORDER BY created_at DESC LIMIT 200`)
	if err != nil {
		fail(w, http.StatusInternalServerError, "读取待审核资料失败："+err.Error())
		return
	}
	defer rows.Close()

	items := make([]PendingDoc, 0)
	for rows.Next() {
		var item PendingDoc
		if err := rows.Scan(&item.ID, &item.Title, &item.URL, &item.Source, &item.Keyword,
			&item.Category, &item.SubCategory, &item.Snippet, &item.CreatedAt); err != nil {
			fail(w, http.StatusInternalServerError, "解析待审核资料失败："+err.Error())
			return
		}
		items = append(items, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
}

// handlePendingApprove：确认入库——写成 Markdown 落到对应分类目录，并尝试加入知识库。
func (a *App) handlePendingApprove(w http.ResponseWriter, r *http.Request) {
	if a.store == nil {
		fail(w, http.StatusServiceUnavailable, "本地库未初始化")
		return
	}
	var request struct {
		ID string `json:"id"`
	}
	if err := decodeJSON(r, &request); err != nil || strings.TrimSpace(request.ID) == "" {
		fail(w, http.StatusBadRequest, "缺少待审核资料 ID")
		return
	}

	var doc PendingDoc
	err := a.store.db.QueryRowContext(r.Context(),
		`SELECT id, title, url, source, keyword, category, sub_category, snippet, created_at
		 FROM pending_docs WHERE id = ?`, request.ID).
		Scan(&doc.ID, &doc.Title, &doc.URL, &doc.Source, &doc.Keyword,
			&doc.Category, &doc.SubCategory, &doc.Snippet, &doc.CreatedAt)
	if err != nil {
		fail(w, http.StatusNotFound, "待审核资料不存在")
		return
	}

	root := a.resolveLibraryRoot()
	category := normalizeCategory(doc.Category)
	dir := filepath.Join(root, category)
	if sub := sanitizeSegment(doc.SubCategory); sub != "" {
		dir = filepath.Join(dir, sub)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fail(w, http.StatusInternalServerError, "创建分类目录失败："+err.Error())
		return
	}

	base := sanitizeSegment(doc.Title)
	if base == "" {
		base = "外网资料"
	}
	path := filepath.Join(dir, fmt.Sprintf("%s-%s.md", base, doc.ID[:6]))
	content := fmt.Sprintf("# %s\n\n- 来源：%s\n- 关键词：%s\n- 采集方式：外网搜集（%s）\n- 抓取时间：%s\n\n%s\n",
		doc.Title, doc.URL, doc.Keyword, doc.Source, doc.CreatedAt, doc.Snippet)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		fail(w, http.StatusInternalServerError, "写入资料文件失败："+err.Error())
		return
	}

	status := a.store.IndexFile(r.Context(), path)
	if _, err := a.store.db.ExecContext(r.Context(), `DELETE FROM pending_docs WHERE id = ?`, doc.ID); err != nil {
		fail(w, http.StatusInternalServerError, "清理待审核记录失败："+err.Error())
		return
	}

	rel, _ := filepath.Rel(root, path)
	writeJSON(w, http.StatusOK, map[string]any{
		"path":     path,
		"relPath":  rel,
		"category": category,
		"kb":       status,
	})
}

func (a *App) handlePendingReject(w http.ResponseWriter, r *http.Request) {
	if a.store == nil {
		fail(w, http.StatusServiceUnavailable, "本地库未初始化")
		return
	}
	var request struct {
		ID string `json:"id"`
	}
	if err := decodeJSON(r, &request); err != nil || strings.TrimSpace(request.ID) == "" {
		fail(w, http.StatusBadRequest, "缺少待审核资料 ID")
		return
	}
	if _, err := a.store.db.ExecContext(r.Context(), `DELETE FROM pending_docs WHERE id = ?`, request.ID); err != nil {
		fail(w, http.StatusInternalServerError, "忽略失败："+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": request.ID})
}
