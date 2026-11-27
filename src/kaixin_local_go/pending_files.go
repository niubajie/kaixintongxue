package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

// 资料直接落到资料库分类目录：
// - 采集时先查黑名单与重复，再下载；
// - 审核通过只改状态，不移动文件；
// - 审核不通过删除文件并登记黑名单。

const maxPendingFileBytes = 200 << 20 // 单条资料最大 200MB

// pendingFilesDir 仅用于兼容旧数据（早期版本把待审核资料放在临时目录）。
func (a *App) pendingFilesDir() string {
	if a.store == nil {
		return ""
	}
	return filepath.Join(filepath.Dir(a.store.path), "pending_files")
}

// resultFingerprint 用「规范化链接 + 标题」生成去重指纹。
func resultFingerprint(result webResult) string {
	raw := normalizeURL(result.URL) + "|" + strings.TrimSpace(result.Title)
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:16])
}

func normalizeURL(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if index := strings.Index(value, "#"); index >= 0 {
		value = value[:index]
	}
	value = strings.TrimSuffix(value, "/")
	if parsed, err := url.Parse(value); err == nil {
		parsed.Host = strings.ToLower(parsed.Host)
		return parsed.String()
	}
	return strings.ToLower(value)
}

// pendingBlocked 是否命中资料黑名单（按链接或指纹）。
func (a *App) pendingBlocked(ctx context.Context, rawURL, fingerprint string) bool {
	if a.store == nil {
		return false
	}
	var count int
	err := a.store.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM pending_blacklist
		 WHERE (url != '' AND url = ?) OR (fingerprint != '' AND fingerprint = ?)`,
		rawURL, fingerprint).Scan(&count)
	return err == nil && count > 0
}

// pendingDuplicate 是否已经收集过（待审核或已通过都算）。
func (a *App) pendingDuplicate(ctx context.Context, rawURL, fingerprint string) bool {
	if a.store == nil {
		return false
	}
	var count int
	err := a.store.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM pending_docs
		 WHERE (url != '' AND url = ?) OR (fingerprint != '' AND fingerprint = ?)`,
		rawURL, fingerprint).Scan(&count)
	return err == nil && count > 0
}

// addToBlacklist 记录被拒绝的资料，后续采集不会再进来。
func (a *App) addToBlacklist(ctx context.Context, rawURL, title, fingerprint string) {
	if a.store == nil {
		return
	}
	if fingerprint == "" {
		fingerprint = resultFingerprint(webResult{URL: rawURL, Title: title})
	}
	_, _ = a.store.db.ExecContext(ctx,
		`INSERT INTO pending_blacklist (id, url, title, fingerprint, created_at) VALUES (?, ?, ?, ?, ?)`,
		newRandomID(), rawURL, truncate(title, 200), fingerprint, time.Now().UTC().Format(time.RFC3339))
}

// downloadIntoLibrary 把结果直接下载到分类目录，返回本地路径与实际字节数。
// 文件按原始格式保存；网页由 HTTP 或无头浏览器抽取正文后保存为 Markdown。
// maxBytes 是本轮剩余可下载字节（<=0 表示不允许下载），超过上限的文件直接跳过。
func (a *App) downloadIntoLibrary(ctx context.Context, dir, id, title string, result webResult, maxBytes int64) (string, int64, bool) {
	if maxBytes <= 0 {
		return "", 0, false
	}
	if resolveSearchProvider() == "demo" {
		// 演示源没有真实资料，只有视频能走哔哩哔哩下载真实文件。
		if !bvidPattern.MatchString(result.URL) {
			return "", 0, false
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", 0, false
	}
	// 视频页面先解析成 mp4 直链，再带 Referer 下载。
	target := result.URL
	referer := ""
	if bvidPattern.MatchString(target) {
		media, ok := resolveBilibiliMedia(ctx, target)
		if !ok {
			return "", 0, false
		}
		target = media
		referer = "https://www.bilibili.com/"
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return "", 0, false
	}
	request.Header.Set("User-Agent", searchUserAgent)
	if referer != "" {
		request.Header.Set("Referer", referer)
	}
	client := &http.Client{Timeout: 120 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		// 普通网页即使 HTTP 通道失败，也交给正文抓取走无头浏览器兜底。
		if result.Kind != "视频" && !bvidPattern.MatchString(result.URL) {
			return a.saveArticlePage(ctx, dir, id, result, maxBytes)
		}
		return "", 0, false
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		// 百度百科等页面会对纯 HTTP 返回 403，但无头浏览器可以正常读取。
		if result.Kind != "视频" && !bvidPattern.MatchString(result.URL) {
			return a.saveArticlePage(ctx, dir, id, result, maxBytes)
		}
		return "", 0, false
	}
	mediaType, _, _ := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if mediaType == "" || mediaType == "text/html" || mediaType == "application/xhtml+xml" {
		// 网页类结果：抽取正文存成 Markdown，覆盖知乎、公众号这类没有文件的平台。
		if result.Kind == "视频" || bvidPattern.MatchString(result.URL) {
			return "", 0, false
		}
		return a.saveArticlePage(ctx, dir, id, result, maxBytes)
	}
	// 拿到 Content-Length 时先判定，避免下一个注定超预算的大文件。
	if response.ContentLength > 0 && response.ContentLength > maxBytes {
		return "", 0, false
	}
	path := filepath.Join(dir, pendingFileName(title, id, extensionForMedia(mediaType, result.URL)))
	file, err := os.Create(path)
	if err != nil {
		return "", 0, false
	}
	// 多写 1 字节用于检测「文件比剩余预算还大」，超了就丢弃，避免产生截断文件。
	written, err := io.Copy(file, io.LimitReader(response.Body, maxBytes+1))
	if err != nil {
		file.Close()
		_ = os.Remove(path)
		return "", 0, false
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return "", 0, false
	}
	if written > maxBytes {
		_ = os.Remove(path)
		return "", 0, false
	}
	return path, written, true
}

// extensionForMedia 按 MIME 推断扩展名；识别不了再看原始 URL 的扩展名。
func extensionForMedia(mediaType, rawURL string) string {
	switch mediaType {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	case "image/bmp":
		return ".bmp"
	case "video/mp4":
		return ".mp4"
	case "video/webm":
		return ".webm"
	case "video/quicktime":
		return ".mov"
	case "application/pdf":
		return ".pdf"
	}
	if ext := strings.ToLower(filepath.Ext(rawURL)); ext != "" && len(ext) <= 6 {
		return ext
	}
	return ".bin"
}

func pendingFileName(title, id, extension string) string {
	base := sanitizeSegment(title)
	if base == "" {
		base = "未命名资料"
	}
	if utf8.RuneCountInString(base) > 40 {
		base = string([]rune(base)[:40])
	}
	suffix := id
	if len(suffix) > 6 {
		suffix = suffix[:6]
	}
	return base + "-" + suffix + extension
}

// writeTextDocument 把同类文字资料（如留言）合并写成一个本地文档，返回路径。
func writeTextDocument(dir, name, content string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, pendingFileName(name, newRandomID(), ".md"))
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// removePendingFile 删除资料文件；文件不存在时静默跳过。
func removePendingFile(path string) {
	if strings.TrimSpace(path) == "" {
		return
	}
	_ = os.Remove(path)
}

// textFingerprint 汇总文档按「关键词 + 类型 + 内容」去重。
func textFingerprint(keyword, docType string, unique []string) string {
	raw := keyword + "|" + docType + "|" + strings.Join(unique, "\n")
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:16])
}

// ensureTextDir 返回文字资料应落到的分类目录。
func (a *App) ensureTextDir(category string) string {
	dir := filepath.Join(a.resolveLibraryRoot(), normalizeCategory(category))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ""
	}
	return dir
}
