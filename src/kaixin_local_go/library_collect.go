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
	"strconv"
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

// categoryHints 用于审核入库时的自动归类，命中词最多的大类胜出；全部未命中则归入「未分类」。
var categoryHints = []struct {
	Name  string
	Words []string
}{
	{"工作", []string{"工作", "运营", "营销", "方案", "客户", "项目", "会议", "汇报", "简历", "招聘", "销售", "职场", "策划", "管理", "绩效"}},
	{"生活", []string{"生活", "健康", "美食", "旅行", "旅游", "健身", "育儿", "装修", "理财", "养生", "家居", "菜谱", "亲子", "日常"}},
	{"学习", []string{"学习", "教程", "课程", "考试", "英语", "编程", "代码", "算法", "论文", "知识", "考研", "笔记", "入门", "技巧"}},
	{"爱好", []string{"摄影", "电影", "音乐", "游戏", "动漫", "绘画", "手工", "运动", "书法", "咖啡", "乐器", "收藏", "徒步", "露营"}},
}

// inferCategory 按关键词、标题与摘要推断所属大类。
func inferCategory(text string) string {
	best, bestScore := "未分类", 0
	for _, hint := range categoryHints {
		score := 0
		for _, word := range hint.Words {
			if strings.Contains(text, word) {
				score++
			}
		}
		if score > bestScore {
			best, bestScore = hint.Name, score
		}
	}
	return best
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
	if err := a.store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pending_docs WHERE status = 'pending'`).Scan(&count); err != nil {
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
	// ScheduleID 是产生这条资料的定时任务；为空表示手动外网搜集。
	ScheduleID string `json:"scheduleId"`
	// LocalPath 是下载到本地临时资料库的文件路径，预览直接打开它。
	LocalPath string `json:"localPath"`
	CreatedAt string `json:"createdAt"`
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

// insertPending 写入一条待审核记录：采集时先查黑名单与重复，再把文件直接存进分类目录。
// maxBytes 是本轮剩余可下载字节（<=0 表示不允许下载）；返回实际写入的字节数。
// 返回 false 表示黑名单 / 重复 / 没有可下载文件 / 超出大小限制，跳过。
func (a *App) insertPending(ctx context.Context, scheduleID, keyword, category, sub string, result webResult, maxBytes int64) (PendingDoc, int64, bool, error) {
	fingerprint := resultFingerprint(result)
	// 先看是否被拉黑，再看是否已经收过同样的资料。
	if a.pendingBlocked(ctx, result.URL, fingerprint) || a.pendingDuplicate(ctx, result.URL, fingerprint) {
		return PendingDoc{}, 0, false, nil
	}
	id := newRandomID()
	title := strings.TrimSpace(result.Title)
	if title == "" {
		title = buildPendingTitle(keyword, result)
	}
	// 采集时就按内容归类，文件直接落到对应分类目录。
	category = normalizeCategory(category)
	if strings.TrimSpace(category) == "" || category == "未分类" {
		category = inferCategory(keyword + "\n" + result.Title + "\n" + result.Snippet)
	}
	subCategory := sanitizeSegment(sub)
	dir := filepath.Join(a.resolveLibraryRoot(), category)
	if subCategory != "" {
		dir = filepath.Join(dir, subCategory)
	}
	// 单条资料还受全局单文件上限约束，取两者较小值。
	limit := maxBytes
	if limit < 0 || limit > maxPendingFileBytes {
		limit = maxPendingFileBytes
	}
	localPath, size, ok := a.downloadIntoLibrary(ctx, dir, id, title, result, limit)
	if !ok {
		return PendingDoc{}, 0, false, nil
	}
	doc := PendingDoc{
		ID:          id,
		Title:       truncate(title, 200),
		URL:         result.URL,
		Source:      result.Source,
		Keyword:     truncate(keyword, 120),
		Category:    category,
		SubCategory: subCategory,
		Snippet:     truncate(result.Snippet, 1000),
		ScheduleID:  scheduleID,
		LocalPath:   localPath,
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
	}
	_, err := a.store.db.ExecContext(ctx,
		`INSERT INTO pending_docs (id, title, url, source, keyword, category, sub_category, snippet, schedule_id, local_path, fingerprint, status, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'pending', ?)`,
		doc.ID, doc.Title, doc.URL, doc.Source, doc.Keyword, doc.Category,
		doc.SubCategory, doc.Snippet, doc.ScheduleID, doc.LocalPath, fingerprint, doc.CreatedAt,
	)
	return doc, size, err == nil, err
}

// insertTextPending 把一批文字资料（如留言）按内容去重后写进同一个本地文档。
// 返回的待审核记录指向该文档，预览时直接打开。
func (a *App) insertTextPending(ctx context.Context, scheduleID, keyword, category, sub, docType string, chunks []string) (PendingDoc, int, error) {
	unique := make([]string, 0, len(chunks))
	seen := make(map[string]bool, len(chunks))
	for _, chunk := range chunks {
		chunk = strings.TrimSpace(chunk)
		if chunk == "" || seen[chunk] {
			continue
		}
		seen[chunk] = true
		unique = append(unique, chunk)
	}
	if len(unique) == 0 {
		return PendingDoc{}, 0, nil
	}
	// 汇总文档也先去重：同一关键词、同一类型、同一批内容只收一次。
	fingerprint := textFingerprint(keyword, docType, unique)
	if a.pendingBlocked(ctx, "", fingerprint) || a.pendingDuplicate(ctx, "", fingerprint) {
		return PendingDoc{}, 0, nil
	}
	category = normalizeCategory(category)
	if strings.TrimSpace(category) == "" || category == "未分类" {
		category = inferCategory(keyword + "\n" + docType)
	}
	dir := a.ensureTextDir(category)
	if dir == "" {
		return PendingDoc{}, 0, fmt.Errorf("创建分类目录失败")
	}
	title := fmt.Sprintf("%s · %s合集（%d 条）", keyword, docType, len(unique))
	var builder strings.Builder
	builder.WriteString("# " + title + "\n\n")
	builder.WriteString("- 关键词：" + keyword + "\n")
	builder.WriteString("- 资料类型：" + docType + "\n")
	builder.WriteString("- 整理时间：" + time.Now().UTC().Format(time.RFC3339) + "\n")
	builder.WriteString("- 条数：" + strconv.Itoa(len(unique)) + "\n\n")
	for index, chunk := range unique {
		builder.WriteString(fmt.Sprintf("## %d\n%s\n\n", index+1, chunk))
	}
	// 文件名用简洁的「关键词 · 类型合集」，条数只体现在标题里。
	path, err := writeTextDocument(dir, fmt.Sprintf("%s · %s合集", keyword, docType), builder.String())
	if err != nil {
		return PendingDoc{}, 0, err
	}
	doc := PendingDoc{
		ID:          newRandomID(),
		Title:       title,
		Source:      docType + "合集",
		Keyword:     truncate(keyword, 120),
		Category:    category,
		SubCategory: sanitizeSegment(sub),
		Snippet:     truncate(unique[0], 1000),
		ScheduleID:  scheduleID,
		LocalPath:   path,
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
	}
	_, err = a.store.db.ExecContext(ctx,
		`INSERT INTO pending_docs (id, title, url, source, keyword, category, sub_category, snippet, schedule_id, local_path, fingerprint, status, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'pending', ?)`,
		doc.ID, doc.Title, doc.URL, doc.Source, doc.Keyword, doc.Category,
		doc.SubCategory, doc.Snippet, doc.ScheduleID, doc.LocalPath, fingerprint, doc.CreatedAt,
	)
	return doc, len(unique), err
}

// buildPendingTitle 没有现成标题时，用关键词 + 摘要首句拼一个可读标题。
func buildPendingTitle(keyword string, result webResult) string {
	snippet := strings.TrimSpace(result.Snippet)
	if snippet != "" {
		snippet = strings.TrimPrefix(snippet, "（演示数据）")
		for _, cut := range []string{"。", "！", "？", "\n", ". "} {
			if index := strings.Index(snippet, cut); index > 6 {
				snippet = snippet[:index]
				break
			}
		}
		snippet = truncate(strings.TrimSpace(snippet), 40)
	}
	parts := make([]string, 0, 3)
	if keyword != "" {
		parts = append(parts, keyword)
	}
	if snippet != "" {
		parts = append(parts, snippet)
	}
	if result.Source != "" {
		parts = append(parts, result.Source)
	}
	if len(parts) == 0 {
		return "未命名资料"
	}
	return strings.Join(parts, " · ")
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
		doc, _, ok, err := a.insertPending(r.Context(), "", keyword, request.Category, request.SubCategory, result, -1)
		if err != nil {
			fail(w, http.StatusInternalServerError, "保存搜集结果失败："+err.Error())
			return
		}
		if !ok {
			// 没有真实可下载文件的网页结果不进入待审核区。
			continue
		}
		saved = append(saved, doc)
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
	// 支持按定时任务筛选：任务卡点开只看本任务搜集到的资料。
	scheduleID := strings.TrimSpace(r.URL.Query().Get("scheduleId"))
	query := `SELECT id, title, url, source, keyword, category, sub_category, snippet, schedule_id, local_path, created_at
		 FROM pending_docs WHERE status = 'pending'`
	args := make([]any, 0, 1)
	if scheduleID != "" {
		query += ` AND schedule_id = ?`
		args = append(args, scheduleID)
	}
	query += ` ORDER BY created_at DESC LIMIT 200`
	rows, err := a.store.db.QueryContext(r.Context(), query, args...)
	if err != nil {
		fail(w, http.StatusInternalServerError, "读取待审核资料失败："+err.Error())
		return
	}
	defer rows.Close()

	items := make([]PendingDoc, 0)
	for rows.Next() {
		var item PendingDoc
		if err := rows.Scan(&item.ID, &item.Title, &item.URL, &item.Source, &item.Keyword,
			&item.Category, &item.SubCategory, &item.Snippet, &item.ScheduleID, &item.LocalPath, &item.CreatedAt); err != nil {
			fail(w, http.StatusInternalServerError, "解析待审核资料失败："+err.Error())
			return
		}
		items = append(items, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
}

// handlePendingApprove：审核通过——文件已经落在分类目录，只修改标注并加入知识库。
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
		`SELECT id, title, url, source, keyword, category, sub_category, snippet, local_path, created_at
		 FROM pending_docs WHERE id = ? AND status = 'pending'`, request.ID).
		Scan(&doc.ID, &doc.Title, &doc.URL, &doc.Source, &doc.Keyword,
			&doc.Category, &doc.SubCategory, &doc.Snippet, &doc.LocalPath, &doc.CreatedAt)
	if err != nil {
		fail(w, http.StatusNotFound, "待审核资料不存在")
		return
	}
	if doc.LocalPath == "" {
		fail(w, http.StatusConflict, "这条资料没有本地文件，无法通过")
		return
	}
	if _, statErr := os.Stat(doc.LocalPath); statErr != nil {
		fail(w, http.StatusNotFound, "资料文件已不存在："+doc.LocalPath)
		return
	}

	status := a.store.IndexFile(r.Context(), doc.LocalPath)
	if _, err := a.store.db.ExecContext(r.Context(),
		`UPDATE pending_docs SET status = 'approved' WHERE id = ?`, doc.ID); err != nil {
		fail(w, http.StatusInternalServerError, "更新审核状态失败："+err.Error())
		return
	}

	root := a.resolveLibraryRoot()
	rel, _ := filepath.Rel(root, doc.LocalPath)
	writeJSON(w, http.StatusOK, map[string]any{
		"path":     doc.LocalPath,
		"relPath":  rel,
		"category": doc.Category,
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
	var doc PendingDoc
	var fingerprint string
	if err := a.store.db.QueryRowContext(r.Context(),
		`SELECT url, title, local_path, fingerprint FROM pending_docs WHERE id = ?`, request.ID).
		Scan(&doc.URL, &doc.Title, &doc.LocalPath, &fingerprint); err == nil {
		// 审核不通过：删除资料文件，并登记黑名单，后续采集不再收录。
		removePendingFile(doc.LocalPath)
		a.addToBlacklist(r.Context(), doc.URL, doc.Title, fingerprint)
	}
	if _, err := a.store.db.ExecContext(r.Context(), `DELETE FROM pending_docs WHERE id = ?`, request.ID); err != nil {
		fail(w, http.StatusInternalServerError, "忽略失败："+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": request.ID})
}
