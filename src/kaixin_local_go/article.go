package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown"
	"github.com/chromedp/chromedp"
	readability "github.com/go-shiori/go-readability"
)

// 正文少于这个字数视为没有有效内容（登录页、验证页、纯导航页）。
const minArticleRunes = 300

// extractArticle 抓取网页并抽取正文，转成 Markdown。
// 先用轻量 HTTP；遇到 403、脚本渲染页等情况，再用无头浏览器读取 DOM。
// 内容平台（知乎 / 公众号 / CSDN 等）没有可下载文件，靠正文抽取也能把资料收进本地。
func extractArticle(ctx context.Context, target string) (string, string, bool) {
	if title, markdown, ok := extractArticleHTTP(ctx, target); ok {
		return title, markdown, true
	}
	return extractArticleBrowser(ctx, target)
}

func extractArticleHTTP(ctx context.Context, target string) (string, string, bool) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return "", "", false
	}
	request.Header.Set("User-Agent", searchUserAgent)
	request.Header.Set("Accept", "text/html,application/xhtml+xml")
	request.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
	// 反爬站点（知乎 403 / CSDN 521）需要登录后才放行，按站点读取用户配置的 Cookie。
	if cookie := siteCookie(target); cookie != "" {
		request.Header.Set("Cookie", cookie)
	}
	client := &http.Client{Timeout: 25 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return "", "", false
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", "", false
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		return "", "", false
	}
	return articleFromHTML(target, body)
}

var articleBrowserMu sync.Mutex

func extractArticleBrowser(ctx context.Context, target string) (string, string, bool) {
	browser := browserExecPath()
	if browser == "" {
		return "", "", false
	}
	articleBrowserMu.Lock()
	defer articleBrowserMu.Unlock()
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(browser),
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-first-run", true),
		chromedp.Flag("no-default-browser-check", true),
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
		chromedp.UserDataDir(filepath.Join(os.TempDir(), "kaixin-article-browser")),
		chromedp.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36 Edg/131.0.0.0"),
	)
	timeoutCtx, timeoutCancel := context.WithTimeout(ctx, 40*time.Second)
	defer timeoutCancel()
	allocCtx, allocCancel := chromedp.NewExecAllocator(timeoutCtx, opts...)
	defer allocCancel()
	browserCtx, browserCancel := chromedp.NewContext(allocCtx)
	defer browserCancel()
	var pageHTML string
	if err := chromedp.Run(browserCtx,
		chromedp.Navigate(target),
		chromedp.WaitReady("body"),
		chromedp.Sleep(1500*time.Millisecond),
		chromedp.Evaluate(`document.documentElement.outerHTML`, &pageHTML),
	); err != nil || strings.TrimSpace(pageHTML) == "" {
		return "", "", false
	}
	return articleFromHTML(target, []byte(pageHTML))
}

func articleFromHTML(target string, body []byte) (string, string, bool) {
	pageURL, err := url.Parse(target)
	if err != nil {
		return "", "", false
	}
	article, err := readability.FromReader(bytes.NewReader(body), pageURL)
	if err != nil {
		return "", "", false
	}
	text := strings.TrimSpace(article.TextContent)
	if len([]rune(text)) < minArticleRunes {
		return "", "", false
	}
	markdown := ""
	if converter := htmltomarkdown.NewConverter("", true, nil); converter != nil {
		if converted, err := converter.ConvertString(article.Content); err == nil {
			markdown = strings.TrimSpace(converted)
		}
	}
	if markdown == "" {
		markdown = text
	}
	title := strings.TrimSpace(article.Title)
	if title == "" {
		title = pageURL.Host
	}
	return title, markdown, true
}

// cutText 按字符截断，避免提示过长或文件名超出 Windows 路径限制。
func cutText(value string, limit int) string {
	trimmed := strings.TrimSpace(value)
	runes := []rune(trimmed)
	if limit <= 0 || len(runes) <= limit {
		return trimmed
	}
	return string(runes[:limit]) + "…"
}

// siteCookieEnv 反爬站点对应的环境变量名，用户把浏览器 Cookie 填进去即可。
func siteCookieEnv(domain string) string {
	lowered := strings.ToLower(domain)
	switch {
	case strings.Contains(lowered, "zhihu.com"):
		return "KAIXIN_COOKIE_ZHIHU"
	case strings.Contains(lowered, "csdn.net"):
		return "KAIXIN_COOKIE_CSDN"
	case strings.Contains(lowered, "xiaohongshu.com"):
		return "KAIXIN_COOKIE_XHS"
	case strings.Contains(lowered, "douyin.com"):
		return "KAIXIN_COOKIE_DOUYIN"
	case strings.Contains(lowered, "mp.weixin.qq.com"):
		return "KAIXIN_COOKIE_WECHAT"
	case strings.Contains(lowered, "weibo.com"):
		return "KAIXIN_COOKIE_WEIBO"
	}
	return ""
}

// siteCookie 按目标地址所属站点读取登录 Cookie：
// 优先用「登录助手」保存到本地库的 Cookie，其次回落到环境变量（兼容旧的手动配置方式）。
func siteCookie(target string) string {
	parsed, err := url.Parse(target)
	if err != nil {
		return ""
	}
	host := parsed.Hostname()
	if saved := memoryCookie(host); saved != "" {
		return saved
	}
	name := siteCookieEnv(host)
	if name == "" {
		return ""
	}
	return strings.TrimSpace(os.Getenv(name))
}

// pageStatusCode 探测页面 HTTP 状态码，用于判断是否因未登录被拦（403 / 401 / 521）。
// 网络失败返回 0。
func pageStatusCode(ctx context.Context, target string) int {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return 0
	}
	request.Header.Set("User-Agent", searchUserAgent)
	request.Header.Set("Accept", "text/html,application/xhtml+xml")
	request.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
	if cookie := siteCookie(target); cookie != "" {
		request.Header.Set("Cookie", cookie)
	}
	client := &http.Client{Timeout: 15 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return 0
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1024))
	return response.StatusCode
}

// saveArticlePage 把网页正文存成本地 Markdown 资料，返回路径与实际字节数。
func (a *App) saveArticlePage(ctx context.Context, dir, id string, result webResult, maxBytes int64) (string, int64, bool) {
	title, markdown, ok := result.ArticleTitle, result.ArticleMarkdown, result.ArticleMarkdown != ""
	if !ok {
		title, markdown, ok = extractArticle(ctx, result.URL)
	}
	if !ok {
		return "", 0, false
	}
	if title == "" {
		title = result.Title
	}
	title = cutText(title, 40)
	content := "# " + title + "\n\n> 来源：" + result.URL + "\n\n" + markdown + "\n"
	size := int64(len(content))
	if size > maxBytes {
		return "", 0, false
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", 0, false
	}
	path := filepath.Join(dir, pendingFileName(title, id, ".md"))
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", 0, false
	}
	return path, size, true
}
