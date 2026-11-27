package main

import (
	"context"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// 外网搜集的网站设置：默认给出常用站点，用户可编辑名称/域名、增删。
// 搜集时按这里的域名拼 `site:` 限定，避免结果散落在全站。
type SearchSite struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Domain    string `json:"domain"`
	Enabled   bool   `json:"enabled"`
	CreatedAt string `json:"createdAt"`
	// HasCookie 表示该站是否已通过「登录助手」保存了登录态。
	HasCookie bool `json:"hasCookie"`
}

func (a *App) loadSites(ctx context.Context, onlyEnabled bool) ([]SearchSite, error) {
	query := `SELECT id, name, domain, enabled, created_at, cookie FROM search_sites`
	if onlyEnabled {
		query += ` WHERE enabled = 1`
	}
	query += ` ORDER BY created_at ASC`

	rows, err := a.store.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	sites := []SearchSite{}
	for rows.Next() {
		var item SearchSite
		var enabled int
		var cookie string
		if err := rows.Scan(&item.ID, &item.Name, &item.Domain, &enabled, &item.CreatedAt, &cookie); err != nil {
			return nil, err
		}
		item.Enabled = enabled == 1
		item.HasCookie = strings.TrimSpace(cookie) != ""
		sites = append(sites, item)
	}
	return sites, rows.Err()
}

// normalizeDomain 规范化站点地址：去掉协议，保留主机、端口与路径。
// 例如 https://1.xiang810.cc:88/search → 1.xiang810.cc:88/search。
func normalizeDomain(value string) string {
	parsed, ok := parseSiteAddress(value)
	if !ok {
		return ""
	}
	return strings.ToLower(parsed.Host) + strings.TrimRight(parsed.EscapedPath(), "/")
}

func parseSiteAddress(value string) (*url.URL, bool) {
	raw := strings.TrimSpace(value)
	if raw == "" {
		return nil, false
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Hostname() == "" {
		return nil, false
	}
	if !strings.EqualFold(parsed.Scheme, "http") && !strings.EqualFold(parsed.Scheme, "https") {
		return nil, false
	}
	return parsed, true
}

// siteHostname 只取主机名（不含端口和路径），供 site: 检索与 Cookie 匹配使用。
func siteHostname(value string) string {
	parsed, ok := parseSiteAddress(value)
	if !ok {
		return ""
	}
	return strings.ToLower(parsed.Hostname())
}

func (a *App) handleSiteList(w http.ResponseWriter, r *http.Request) {
	if a.store == nil {
		fail(w, http.StatusServiceUnavailable, "本地库未初始化")
		return
	}
	sites, err := a.loadSites(r.Context(), false)
	if err != nil {
		fail(w, http.StatusInternalServerError, "读取网站设置失败："+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": sites})
}

func (a *App) handleSiteSave(w http.ResponseWriter, r *http.Request) {
	if a.store == nil {
		fail(w, http.StatusServiceUnavailable, "本地库未初始化")
		return
	}
	var request struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Domain  string `json:"domain"`
		Enabled *bool  `json:"enabled"`
	}
	if err := decodeJSON(r, &request); err != nil {
		fail(w, http.StatusBadRequest, "请求格式不正确")
		return
	}
	name := strings.TrimSpace(request.Name)
	domain := normalizeDomain(request.Domain)
	if name == "" || domain == "" {
		fail(w, http.StatusBadRequest, "网站名称与域名都不能为空")
		return
	}
	enabled := 1
	if request.Enabled != nil && !*request.Enabled {
		enabled = 0
	}

	id := strings.TrimSpace(request.ID)
	if id == "" {
		id = newRandomID()
		_, err := a.store.db.ExecContext(r.Context(),
			`INSERT INTO search_sites (id, name, domain, enabled, created_at) VALUES (?, ?, ?, ?, ?)`,
			id, name, domain, enabled, time.Now().UTC().Format(time.RFC3339))
		if err != nil {
			fail(w, http.StatusInternalServerError, "新增网站失败："+err.Error())
			return
		}
	} else {
		_, err := a.store.db.ExecContext(r.Context(),
			`UPDATE search_sites SET name = ?, domain = ?, enabled = ? WHERE id = ?`,
			name, domain, enabled, id)
		if err != nil {
			fail(w, http.StatusInternalServerError, "保存网站失败："+err.Error())
			return
		}
	}

	sites, err := a.loadSites(r.Context(), false)
	if err != nil {
		fail(w, http.StatusInternalServerError, "读取网站设置失败："+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": sites})
}

// verifySiteDownloadable 用真实检索验证网站能否拿到可下载资料。
// 只有真正解析出可下载文件才算通过；失败时返回原因与需要用户手动配合的方法。
// needLogin=true 表示该站未登录被拦，前端应弹出「登录助手」。
func verifySiteDownloadable(ctx context.Context, rawDomain, keyword string) (ok bool, message, hint string, needLogin bool) {
	domain := normalizeDomain(rawDomain)
	if domain == "" {
		return false, "域名为空", "", false
	}
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		keyword = "测试"
	}
	// 已保存过登录态的站点，失败时优先提示重新登录。
	knownLoginSite := siteCookieEnv(domain) != ""
	// 哔哩哔哩：检索视频并解析 mp4 直链，能拿到直链才算可下载。
	if strings.Contains(domain, "bilibili") {
		videos, err := bilibiliVideoSearch(ctx, keyword, 1)
		if err != nil || len(videos) == 0 {
			return false, "搜索接口没有返回视频", "换一个更常见的关键词再试，例如「按摩」「瑜伽」。", false
		}
		if _, ok := resolveBilibiliMedia(ctx, videos[0].URL); !ok {
			return false, "找到视频但解析不出下载地址（可能是付费或限制视频）", "换一个关键词；或手动在浏览器下载后放入「文档\\开心同学资料\\分类」目录。", false
		}
		return true, "可搜到视频并解析出下载地址", "", false
	}
	// 抖音 / 小红书 / 公众号的正文完全由脚本渲染，HTTP 抓不到；
	// 这类站只能校验「登录态是否有效」：有登录 Cookie 且站点可访问即视为可用。
	if isJSRenderedSite(domain) {
		if memoryCookie(domain) == "" {
			return false, "该站内容需要登录后才能访问（页面由脚本渲染，无法直接抓取）", "", true
		}
		status := pageStatusCode(ctx, siteHomeURL(domain))
		if status == http.StatusOK {
			return true, "登录态有效（该站页面由脚本渲染，验证只检查登录状态）", "", false
		}
		if status == http.StatusUnauthorized || status == http.StatusForbidden || status == 451 || status == 521 {
			return false, "已保存的登录信息已失效，需要重新登录", "", true
		}
		return false, "该站暂时无法访问（HTTP " + strconv.Itoa(status) + "），请稍后再试", "", true
	}
	// 其他站点走当前搜索源真实检索，能拿到该站资料才算通过。
	if resolveSearchProvider() == "demo" {
		return false,
			"当前处于演示搜索模式，只能拿到演示数据，无法下载真实资料",
			"去掉环境变量 KAIXIN_SEARCH_PROVIDER 中的 demo（或配置 BING_SEARCH_KEY），重启应用后再验证；也可手动下载资料后放入「文档\\开心同学资料\\分类」目录。",
			false
	}
	var candidates []webResult
	if isBaiduDomain(domain) {
		results, _, err := webSearch(ctx, keyword, 8)
		if err == nil {
			candidates = results
		}
	} else if isCSDNDomain(domain) {
		// CSDN 有公开搜索接口，最可靠。
		results, err := csdnSearch(ctx, keyword, 8)
		if err == nil {
			candidates = results
		}
	} else {
		// 百度对 `site:` 语法风控严格，改为「关键词 + 站名」检索后按域名过滤。
		for _, query := range []string{keyword + " " + siteAlias(domain), keyword} {
			results, _, err := webSearch(ctx, query, 10)
			if err != nil {
				continue
			}
			for _, item := range results {
				if urlMatchesDomain(item.URL, domain) {
					candidates = append(candidates, item)
				}
			}
			if len(candidates) > 0 {
				break
			}
		}
	}
	if len(candidates) == 0 {
		// 有些自定义域名只是“最新地址发布页”，并非真正的内容站，不能误判为百度风控。
		if message, ok := diagnoseAddressReleasePage(ctx, domain); ok {
			return false, message, "请把网站设置里的域名改成发布页中列出的实际站点地址，再验证。", false
		}
		// 需要登录的站点往往连检索结果都进不去，直接引导登录。
		if knownLoginSite {
			return false,
				"没有检索到该站内容：该站不对外提供检索结果，需要登录后手动搜索",
				"",
				true
		}
		return false,
			"百度没有检索到该站的相关内容（该站可能限制外部检索，或百度临时风控）",
			manualCookieHint(domain) + "如果刚连续验证过多个网站，百度可能临时限制，请隔几分钟再试。",
			false
	}
	// 只探前几条，避免验证请求过慢。
	blockedStatus := 0
	for index, item := range candidates {
		if index >= 3 {
			break
		}
		if probeDownloadable(ctx, item.URL) {
			return true, "已找到可下载资料：" + cutText(item.Title, 30), "", false
		}
		// 内容平台没有文件，但正文能抽出来同样算可用。
		if _, _, ok := extractArticle(ctx, item.URL); ok {
			return true, "已找到可采集的文章：" + cutText(item.Title, 30), "", false
		}
		if blockedStatus == 0 {
			if status := pageStatusCode(ctx, item.URL); status == http.StatusUnauthorized || status == http.StatusForbidden || status == 451 || status == 521 {
				blockedStatus = status
			}
		}
	}
	if blockedStatus != 0 || knownLoginSite {
		// 百度本身无需登录；搜索结果的 403 是目标页反爬，不能误报为“百度需要登录”。
		if isBaiduDomain(domain) {
			return false, "已拿到百度搜索结果，但前几条结果的正文暂时无法读取（目标网站反爬）", "可以换一个关键词再试；也可以点「在浏览器打开网站」手动检索。", false
		}
		message := "检索到了该站内容，但需要登录后才能读取"
		if blockedStatus != 0 {
			message = "该站拒绝未登录访问（HTTP " + strconv.Itoa(blockedStatus) + "），需要登录后才能读取"
		}
		// 已经保存过登录态还读不到，说明登录可能过期或被风控。
		if memoryCookie(domain) != "" {
			message = "已保存登录信息，但该站仍拒绝读取（登录可能已过期，或站点风控），请重新登录后再试"
		}
		return false, message, "", true
	}
	return false,
		"检索到了该站内容，但正文抓取被拦截（站点反爬限制）",
		manualCookieHint(domain),
		false
}

// diagnoseAddressReleasePage 识别只有跳转链接、没有搜索与资料内容的“地址发布页”。
func diagnoseAddressReleasePage(ctx context.Context, domain string) (string, bool) {
	target := "https://" + domain
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return "", false
	}
	request.Header.Set("User-Agent", searchUserAgent)
	client := &http.Client{Timeout: 15 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return "", false
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", false
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return "", false
	}
	page := string(body)
	// 部分发布页把完整 HTML 放在 document.write(decodeURIComponent("...")) 中。
	if start := strings.Index(page, "document.write(decodeURIComponent(\""); start >= 0 {
		start += len("document.write(decodeURIComponent(\"")
		if end := strings.Index(page[start:], "\")"); end > 0 {
			if decoded, decodeErr := url.PathUnescape(page[start : start+end]); decodeErr == nil {
				page += decoded
			}
		}
	}
	if strings.Contains(page, "地址发布页") || strings.Contains(page, "最新地址发布") {
		return "该域名只是地址发布/跳转页，本身没有关键词搜索或可下载资料", true
	}
	return "", false
}

// manualCookieHint 给出反爬网站的手动配合方法。
func manualCookieHint(domain string) string {
	if env := siteCookieEnv(domain); env != "" {
		return "在浏览器登录该站，按 F12 → 网络，复制任意请求里的 Cookie，设置为系统环境变量 " + env + " 后重启应用即可采集；也可手动保存资料到「文档\\开心同学资料\\分类」目录。"
	}
	return "在浏览器打开该站手动保存资料，或把文件放入「文档\\开心同学资料\\分类」目录。"
}

// isJSRenderedSite 页面内容完全靠脚本渲染、HTTP 抓不到正文的站点。
func isJSRenderedSite(domain string) bool {
	lowered := strings.ToLower(domain)
	for _, key := range []string{"douyin.com", "xiaohongshu.com", "mp.weixin.qq.com"} {
		if strings.Contains(lowered, key) {
			return true
		}
	}
	return false
}

// siteHomeURL 站点首页，用于校验登录态是否有效。
func siteHomeURL(domain string) string {
	lowered := strings.ToLower(domain)
	switch {
	case strings.Contains(lowered, "douyin.com"):
		return "https://www.douyin.com"
	case strings.Contains(lowered, "xiaohongshu.com"):
		return "https://www.xiaohongshu.com"
	case strings.Contains(lowered, "mp.weixin.qq.com"):
		return "https://mp.weixin.qq.com"
	}
	return "https://" + domain
}

// siteAlias 把域名换成中文站名，提升百度命中率。
func siteAlias(domain string) string {
	aliases := map[string]string{
		"zhihu.com":        "知乎",
		"xiaohongshu.com":  "小红书",
		"douyin.com":       "抖音",
		"mp.weixin.qq.com": "微信公众号",
		"blog.csdn.net":    "CSDN",
		"csdn.net":         "CSDN",
		"bilibili.com":     "哔哩哔哩",
		"weibo.com":        "微博",
		"jianshu.com":      "简书",
	}
	for key, alias := range aliases {
		if strings.Contains(domain, key) {
			return alias
		}
	}
	// 认不出来就用域名主体（去掉常见前缀、数字子域和后缀）。
	trimmed := strings.TrimPrefix(siteHostname(domain), "www.")
	labels := strings.Split(trimmed, ".")
	if len(labels) > 2 {
		if _, err := strconv.Atoi(labels[0]); err == nil {
			trimmed = strings.Join(labels[1:], ".")
		}
	}
	if dot := strings.Index(trimmed, "."); dot > 0 {
		trimmed = trimmed[:dot]
	}
	return trimmed
}

// urlMatchesDomain 判断结果链接是否属于指定站点。
func urlMatchesDomain(target, domain string) bool {
	parsed, err := url.Parse(target)
	if err != nil {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	main := strings.TrimPrefix(siteHostname(domain), "www.")
	if host == "" || main == "" {
		return false
	}
	return host == main || strings.HasSuffix(host, "."+main)
}

// probeDownloadable 判断链接是否指向可下载文件：视频页解析直链，其他看 Content-Type。
func probeDownloadable(ctx context.Context, target string) bool {
	if bvidPattern.MatchString(target) {
		_, ok := resolveBilibiliMedia(ctx, target)
		return ok
	}
	client := &http.Client{Timeout: 12 * time.Second}
	attempt := func(method string) bool {
		request, err := http.NewRequestWithContext(ctx, method, target, nil)
		if err != nil {
			return false
		}
		request.Header.Set("User-Agent", searchUserAgent)
		if method == http.MethodGet {
			request.Header.Set("Range", "bytes=0-2047")
		}
		response, err := client.Do(request)
		if err != nil {
			return false
		}
		defer response.Body.Close()
		if response.StatusCode >= http.StatusBadRequest {
			return false
		}
		mediaType, _, _ := mime.ParseMediaType(response.Header.Get("Content-Type"))
		if mediaType == "" || mediaType == "text/html" || mediaType == "application/xhtml+xml" {
			return false
		}
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 2048))
		return true
	}
	if attempt(http.MethodHead) {
		return true
	}
	return attempt(http.MethodGet)
}

// handleSiteVerify 验证某个网站是否真能下载资料，返回原因与手动配合建议。
func (a *App) handleSiteVerify(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Domain  string `json:"domain"`
		Keyword string `json:"keyword"`
	}
	if err := decodeJSON(r, &request); err != nil {
		fail(w, http.StatusBadRequest, "请求格式不正确")
		return
	}
	domain := normalizeDomain(request.Domain)
	if domain == "" {
		fail(w, http.StatusBadRequest, "请先填写域名")
		return
	}
	// 整体限时：HTTP 被风控后会回退到无头浏览器，比普通请求多几秒。
	ctx, cancel := context.WithTimeout(r.Context(), 110*time.Second)
	defer cancel()
	ok, message, hint, needLogin := verifySiteDownloadable(ctx, domain, request.Keyword)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":        ok,
		"message":   message,
		"hint":      hint,
		"domain":    domain,
		"needLogin": needLogin,
		"loginUrl":  loginEntryURL(domain),
	})
}

func (a *App) handleSiteDelete(w http.ResponseWriter, r *http.Request) {
	if a.store == nil {
		fail(w, http.StatusServiceUnavailable, "本地库未初始化")
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		fail(w, http.StatusBadRequest, "缺少网站 ID")
		return
	}
	if _, err := a.store.db.ExecContext(r.Context(), `DELETE FROM search_sites WHERE id = ?`, id); err != nil {
		fail(w, http.StatusInternalServerError, "删除网站失败："+err.Error())
		return
	}
	sites, err := a.loadSites(r.Context(), false)
	if err != nil {
		fail(w, http.StatusInternalServerError, "读取网站设置失败："+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": sites})
}
