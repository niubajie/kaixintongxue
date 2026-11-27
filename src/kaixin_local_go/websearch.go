package main

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/chromedp"
)

// 外网搜索适配层：页面只认「关键词 → 结果」，具体供应商由环境变量切换。
// - KAIXIN_SEARCH_PROVIDER=bing|demo 显式指定；
// - 未指定时，有 BING_SEARCH_KEY 走必应，否则用演示源（便于无 Key 环境跑通流程）。
type webResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
	Source  string `json:"source"`
	// Kind 为“视频”时只允许保存真实视频文件，不能退化成网页 Markdown。
	Kind string `json:"kind,omitempty"`
	// 修复向导已在受控浏览器中抽取的正文，恢复任务时直接落盘，避免重复抓取失败。
	ArticleTitle    string `json:"-"`
	ArticleMarkdown string `json:"-"`
}

func resolveSearchProvider() string {
	if custom := strings.ToLower(strings.TrimSpace(os.Getenv("KAIXIN_SEARCH_PROVIDER"))); custom != "" {
		return custom
	}
	if os.Getenv("BING_SEARCH_KEY") != "" {
		return "bing"
	}
	// 没有必应 Key 时抓百度搜索结果页，仍能拿到真实资料链接。
	return "baidu"
}

func webSearch(ctx context.Context, keyword string, limit int) ([]webResult, string, error) {
	switch resolveSearchProvider() {
	case "bing":
		results, err := bingSearch(ctx, keyword, limit)
		return results, "bing", err
	case "baidu":
		results, err := baiduSearch(ctx, keyword, limit)
		return results, "baidu", err
	default:
		return demoSearch(keyword, limit), "demo", nil
	}
}

// webSearchWithSites 按「网站设置」限定来源：对每个启用站点拼 `site:` 查询后合并。
// 百度站点本身当作全网入口，不加 site: 限定。
func webSearchWithSites(ctx context.Context, keyword string, limit int, sites []SearchSite) ([]webResult, string, error) {
	provider := resolveSearchProvider()
	if provider == "demo" {
		return demoSiteSearch(keyword, limit, sites), "demo", nil
	}
	if limit <= 0 || limit > 10 {
		limit = 5
	}
	search := func(site SearchSite) ([]webResult, error) {
		if isCSDNDomain(site.Domain) {
			return csdnSearch(ctx, keyword, limit)
		}
		query := keyword
		if !isBaiduDomain(site.Domain) {
			query += " site:" + siteHostname(site.Domain)
		}
		if provider == "bing" {
			return bingSearch(ctx, query, limit)
		}
		return baiduSearch(ctx, query, limit)
	}
	if len(sites) == 0 {
		if provider == "bing" {
			results, err := bingSearch(ctx, keyword, limit)
			return results, provider, err
		}
		results, err := baiduSearch(ctx, keyword, limit)
		return results, provider, err
	}
	per := limit / len(sites)
	if per < 1 {
		per = 1
	}
	merged := make([]webResult, 0, limit)
	seen := make(map[string]bool, limit)
	for _, site := range sites {
		results, err := search(site)
		if err != nil {
			continue
		}
		for _, item := range results {
			if seen[item.URL] || len(merged) >= limit {
				continue
			}
			seen[item.URL] = true
			item.Source = site.Name
			merged = append(merged, item)
			if len(merged) >= limit {
				break
			}
		}
		if len(merged) >= limit {
			break
		}
	}
	if len(merged) == 0 {
		return nil, provider, fmt.Errorf("在已选的网站中没有搜到结果")
	}
	return merged, provider, nil
}

// isBaiduDomain 判断是不是百度自身，作为全网搜索入口时不需要 site: 限定。
func isBaiduDomain(domain string) bool {
	return strings.Contains(strings.ToLower(domain), "baidu.com")
}

func demoSiteSearch(keyword string, limit int, sites []SearchSite) []webResult {
	if len(sites) == 0 {
		return demoSearch(keyword, limit)
	}
	if limit <= 0 || limit > 10 {
		limit = 3
	}
	// 演示源按索引轮换内容主题，让同一批结果的标题各不相同（真实源直接用网页标题）。
	topics := []string{"入门指南", "实践案例", "资料合集", "常见问题", "进阶技巧", "行业报告"}
	results := make([]webResult, 0, limit)
	for index := 0; index < limit; index++ {
		site := sites[index%len(sites)]
		base := strings.TrimSpace(keyword)
		// 检索语句里可能带 site: 限定，标题里去掉，只保留关键词本身。
		if cut := strings.Index(base, "site:"); cut > 0 {
			base = strings.TrimSpace(base[:cut])
		}
		if base == "" || strings.HasPrefix(base, "site:") {
			base = site.Name + " 全部资料"
		}
		topic := topics[index%len(topics)]
		// 有站点域名就指向该站点；全网检索（无域名）回落到必应搜索页，避免出现无效的 https://。
		target := "https://www.bing.com/search?q=" + url.QueryEscape(base+" "+topic)
		if site.Domain != "" {
			target = "https://" + site.Domain
		}
		results = append(results, webResult{
			Title:   base + " · " + topic,
			URL:     target,
			Snippet: "（演示数据）来自「" + site.Name + "」的《" + base + " " + topic + "》：整理了核心要点、操作步骤与常见注意事项。配置 BING_SEARCH_KEY 后返回真实内容。",
			Source:  site.Name,
		})
	}
	return results
}

func bingSearch(ctx context.Context, keyword string, limit int) ([]webResult, error) {
	return bingSearchFreshness(ctx, keyword, limit, "")
}

// bingSearchFreshness 在必应基础上加发布时间过滤：Day / Week / Month。
func bingSearchFreshness(ctx context.Context, keyword string, limit int, freshness string) ([]webResult, error) {
	key := os.Getenv("BING_SEARCH_KEY")
	if key == "" {
		return nil, fmt.Errorf("未配置 BING_SEARCH_KEY")
	}
	if limit <= 0 || limit > 10 {
		limit = 5
	}
	values := url.Values{
		"q":     {keyword},
		"count": {fmt.Sprintf("%d", limit)},
		"mkt":   {"zh-CN"},
	}
	if freshness != "" {
		values.Set("freshness", freshness)
	}
	endpoint := "https://api.bing.microsoft.com/v7.0/search?" + values.Encode()

	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Ocp-Apim-Subscription-Key", key)

	response, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("搜索请求失败：%w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("搜索服务返回 %d", response.StatusCode)
	}

	var payload struct {
		WebPages struct {
			Value []struct {
				Name    string `json:"name"`
				URL     string `json:"url"`
				Snippet string `json:"snippet"`
			} `json:"value"`
		} `json:"webPages"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("解析搜索结果失败：%w", err)
	}
	results := make([]webResult, 0, len(payload.WebPages.Value))
	for _, item := range payload.WebPages.Value {
		results = append(results, webResult{
			Title:   strings.TrimSpace(item.Name),
			URL:     item.URL,
			Snippet: strings.TrimSpace(item.Snippet),
			Source:  "bing",
		})
	}
	return results, nil
}

// demoSearch 返回明确标注的演示数据，保证没有 Key 时也能验证「搜集→待审核→入库」全流程。
func demoSearch(keyword string, limit int) []webResult {
	if limit <= 0 || limit > 10 {
		limit = 3
	}
	escaped := url.QueryEscape(keyword)
	templates := []webResult{
		{
			Title:   keyword + " · 入门指南",
			URL:     "https://www.bing.com/search?q=" + escaped,
			Snippet: "（演示数据）围绕「" + keyword + "」的基础概念、常用工具与上手步骤整理。配置 BING_SEARCH_KEY 后返回真实结果。",
			Source:  "demo",
		},
		{
			Title:   keyword + " · 实践案例",
			URL:     "https://www.bing.com/search?q=" + escaped,
			Snippet: "（演示数据）三个可复用的实践案例，含操作流程与常见问题。配置 BING_SEARCH_KEY 后返回真实结果。",
			Source:  "demo",
		},
		{
			Title:   keyword + " · 资料合集",
			URL:     "https://www.bing.com/search?q=" + escaped,
			Snippet: "（演示数据）延伸阅读与参考清单。配置 BING_SEARCH_KEY 后返回真实结果。",
			Source:  "demo",
		},
	}
	if limit > len(templates) {
		limit = len(templates)
	}
	return templates[:limit]
}

// csdnSearch 走 CSDN 公开搜索接口，免 Key 拿到文章和可下载资源。
func csdnSearch(ctx context.Context, keyword string, limit int) ([]webResult, error) {
	if limit <= 0 || limit > 30 {
		limit = 10
	}
	endpoint := "https://so.csdn.net/api/v3/search?q=" + url.QueryEscape(keyword) + "&t=all&p=1"
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", searchUserAgent)
	request.Header.Set("Referer", "https://so.csdn.net/")
	client := &http.Client{Timeout: 20 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("CSDN 搜索请求失败：%w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("CSDN 搜索失败：HTTP %d", response.StatusCode)
	}
	var payload struct {
		Results []struct {
			Title  string `json:"title"`
			URL    string `json:"url"`
			Digest string `json:"digest"`
		} `json:"result_vos"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 8<<20)).Decode(&payload); err != nil {
		return nil, err
	}
	results := make([]webResult, 0, limit)
	for _, item := range payload.Results {
		if len(results) >= limit {
			break
		}
		title := stripHTMLTags(item.Title)
		link := item.URL
		if cut := strings.Index(link, "?"); cut > 0 {
			link = link[:cut]
		}
		if title == "" || link == "" {
			continue
		}
		results = append(results, webResult{
			Title:   title,
			URL:     link,
			Snippet: stripHTMLTags(item.Digest),
			Source:  "CSDN",
		})
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("CSDN 没有搜到「%s」的内容", keyword)
	}
	return results, nil
}

func isCSDNDomain(domain string) bool {
	return strings.Contains(strings.ToLower(domain), "csdn.net")
}

// baiduSearch 免 Key 抓取百度搜索结果页，解析出真实目标地址（跟随 /link?url= 跳转）。
// 只在没有必应 Key 时使用，作为全网检索入口。
// baiduSearch 优先走轻量 HTTP；遇到百度安全验证时，用无头浏览器读取真实结果页。
// 真实浏览器无需登录即可搜索，纯 HTTP 请求则容易被风控，两种通道必须兜底。
func baiduSearch(ctx context.Context, keyword string, limit int) ([]webResult, error) {
	results, httpErr := baiduHTTPSearch(ctx, keyword, limit)
	if httpErr == nil && len(results) > 0 {
		return results, nil
	}
	browserResults, browserErr := baiduBrowserSearch(ctx, keyword, limit)
	if browserErr == nil && len(browserResults) > 0 {
		return browserResults, nil
	}
	if httpErr != nil && browserErr != nil {
		return nil, fmt.Errorf("%v；无头浏览器检索也失败：%v", httpErr, browserErr)
	}
	if httpErr != nil {
		return nil, httpErr
	}
	return nil, browserErr
}

func baiduHTTPSearch(ctx context.Context, keyword string, limit int) ([]webResult, error) {
	if limit <= 0 || limit > 20 {
		limit = 10
	}
	endpoint := "https://www.baidu.com/s?wd=" + url.QueryEscape(keyword) + "&rn=20&ie=utf-8"
	body, status, err := baiduGet(ctx, endpoint)
	if err != nil {
		return nil, fmt.Errorf("百度搜索请求失败：%w", err)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("百度搜索失败：HTTP %d", status)
	}
	page := string(body)
	if strings.Contains(page, "百度安全验证") {
		return nil, fmt.Errorf("百度触发安全验证，请稍后重试或更换关键词")
	}
	matches := baiduResultPattern.FindAllStringSubmatch(page, -1)
	results := make([]webResult, 0, limit)
	seen := make(map[string]bool, limit)
	for _, match := range matches {
		if len(results) >= limit {
			break
		}
		if ctx.Err() != nil {
			break
		}
		title := stripHTMLTags(match[2])
		if title == "" {
			continue
		}
		realURL, ok := resolveBaiduRedirect(ctx, match[1])
		if !ok || seen[realURL] {
			continue
		}
		seen[realURL] = true
		results = append(results, webResult{
			Title:   title,
			URL:     realURL,
			Snippet: "来源：百度搜索",
			Source:  "百度",
		})
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("百度没有搜到「%s」的结果", keyword)
	}
	return results, nil
}

// baiduBrowserSearch 用本机 Edge 的无头模式读取百度搜索结果。
// 仅在 HTTP 通道被风控时调用；串行复用一个独立浏览器配置目录，避免并发抢 profile。
var baiduBrowserMu sync.Mutex

func baiduBrowserSearch(ctx context.Context, keyword string, limit int) ([]webResult, error) {
	browser := browserExecPath()
	if browser == "" {
		return nil, fmt.Errorf("未找到 Edge 或 Chrome")
	}
	if limit <= 0 || limit > 20 {
		limit = 10
	}
	baiduBrowserMu.Lock()
	defer baiduBrowserMu.Unlock()

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(browser),
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-first-run", true),
		chromedp.Flag("no-default-browser-check", true),
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
		chromedp.UserDataDir(filepath.Join(os.TempDir(), "kaixin-search-browser")),
		chromedp.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36 Edg/131.0.0.0"),
	)
	timeoutCtx, timeoutCancel := context.WithTimeout(ctx, 50*time.Second)
	defer timeoutCancel()
	allocCtx, allocCancel := chromedp.NewExecAllocator(timeoutCtx, opts...)
	defer allocCancel()
	browserCtx, browserCancel := chromedp.NewContext(allocCtx)
	defer browserCancel()

	endpoint := "https://www.baidu.com/s?wd=" + url.QueryEscape(keyword) + "&rn=20&ie=utf-8"
	var raw []struct {
		Title string `json:"title"`
		URL   string `json:"url"`
	}
	var bodyText string
	script := `(() => {
		const seen = new Set();
		const items = [];
		for (const a of document.querySelectorAll('#content_left h3 a, .result h3 a, h3.t a')) {
			const title = (a.innerText || a.textContent || '').trim();
			const url = a.href || '';
			if (!title || !url || seen.has(url)) continue;
			seen.add(url);
			items.push({title, url});
		}
		return items;
	})()`
	if err := chromedp.Run(browserCtx,
		chromedp.Navigate(endpoint),
		chromedp.WaitReady("body"),
		chromedp.Sleep(1500*time.Millisecond),
		chromedp.Evaluate(script, &raw),
		chromedp.Evaluate(`document.body ? document.body.innerText : ''`, &bodyText),
	); err != nil {
		return nil, err
	}
	if strings.Contains(bodyText, "百度安全验证") {
		return nil, fmt.Errorf("无头浏览器也触发了百度安全验证")
	}
	results := make([]webResult, 0, limit)
	seen := make(map[string]bool, limit)
	for _, item := range raw {
		if len(results) >= limit {
			break
		}
		title := strings.TrimSpace(item.Title)
		target := strings.TrimSpace(item.URL)
		if title == "" || target == "" {
			continue
		}
		if strings.Contains(target, "baidu.com/link?url=") || strings.Contains(target, "baidu.com/baidu.php?") {
			resolved, ok := resolveBaiduRedirect(ctx, target)
			if !ok {
				continue
			}
			target = resolved
		}
		if seen[target] {
			continue
		}
		seen[target] = true
		results = append(results, webResult{Title: title, URL: target, Snippet: "来源：百度搜索（浏览器）", Source: "百度"})
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("浏览器结果页没有解析到搜索结果")
	}
	return results, nil
}

// resolveBaiduRedirect 百度结果链接是跳转地址，跟随一次重定向拿真实 URL。
func resolveBaiduRedirect(ctx context.Context, link string) (string, bool) {
	client := &http.Client{
		Timeout: 10 * time.Second,
		Jar:     baiduJarCookie(),
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, link, nil)
	if err != nil {
		return "", false
	}
	request.Header.Set("User-Agent", searchUserAgent)
	response, err := client.Do(request)
	if err != nil {
		return "", false
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	response.Body.Close()
	location := strings.TrimSpace(response.Header.Get("Location"))
	if location == "" {
		return "", false
	}
	return location, true
}

func baiduJarCookie() http.CookieJar {
	baiduJarOnce.Do(func() {
		if jar, err := cookiejar.New(nil); err == nil {
			baiduJar = jar
		}
	})
	return baiduJar
}

func baiduClient() *http.Client {
	return &http.Client{Timeout: 20 * time.Second, Jar: baiduJarCookie()}
}

// baiduWarmup 预热首页拿访客 Cookie，降低被安全验证拦截的概率。
func baiduWarmup(ctx context.Context) {
	baiduWarm.Do(func() {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://www.baidu.com/", nil)
		if err != nil {
			return
		}
		request.Header.Set("User-Agent", searchUserAgent)
		response, err := baiduClient().Do(request)
		if err != nil {
			return
		}
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
		response.Body.Close()
	})
}

func baiduGet(ctx context.Context, target string) ([]byte, int, error) {
	baiduWarmup(ctx)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, 0, err
	}
	request.Header.Set("User-Agent", searchUserAgent)
	request.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	request.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
	response, err := baiduClient().Do(request)
	if err != nil {
		return nil, 0, err
	}
	defer response.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(response.Body, 3<<20))
	if readErr != nil {
		return nil, response.StatusCode, readErr
	}
	return body, response.StatusCode, nil
}

// bilibiliVideoSearch 视频类型专用：走哔哩哔哩公开搜索接口拿真实视频条目。
// 普通网页搜索结果都是 HTML 页面、无法作为视频文件下载，所以视频统一在这里检索。
// 分页一直翻到没有更多（或达到 limit 上限），保证「把网站搜到底」。
func bilibiliVideoSearch(ctx context.Context, keyword string, limit int, checkActive ...func() bool) ([]webResult, error) {
	if limit <= 0 {
		limit = 100
	}
	isActive := func() bool { return true }
	if len(checkActive) > 0 && checkActive[0] != nil {
		isActive = checkActive[0]
	}
	const pageSize = 20
	results := make([]webResult, 0, limit)
	seen := make(map[string]bool, limit)
	maxPages := (limit + pageSize - 1) / pageSize
	for page := 1; page <= maxPages; page++ {
		if !isActive() {
			break
		}
		endpoint := fmt.Sprintf(
			"https://api.bilibili.com/x/web-interface/search/type?search_type=video&page=%d&keyword=%s",
			page, url.QueryEscape(keyword))
		body, status, err := bilibiliGet(ctx, endpoint)
		if err != nil {
			if page == 1 {
				return nil, err
			}
			break
		}
		if status != http.StatusOK {
			if page == 1 {
				return nil, fmt.Errorf("哔哩哔哩搜索失败：HTTP %d", status)
			}
			break
		}
		var payload struct {
			Code int `json:"code"`
			Data struct {
				Result []struct {
					BVID        string `json:"bvid"`
					Title       string `json:"title"`
					Description string `json:"description"`
					Author      string `json:"author"`
				} `json:"result"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			if page == 1 {
				return nil, err
			}
			break
		}
		if payload.Code != 0 {
			if page == 1 {
				return nil, fmt.Errorf("哔哩哔哩搜索返回错误码 %d", payload.Code)
			}
			break
		}
		pageCount := len(payload.Data.Result)
		for _, item := range payload.Data.Result {
			if len(results) >= limit || item.BVID == "" || seen[item.BVID] {
				continue
			}
			title := stripHTMLTags(item.Title)
			if title == "" {
				continue
			}
			seen[item.BVID] = true
			snippet := stripHTMLTags(item.Description)
			if snippet == "" {
				snippet = "UP主：" + stripHTMLTags(item.Author)
			}
			results = append(results, webResult{
				Title:   title,
				URL:     "https://www.bilibili.com/video/" + item.BVID,
				Snippet: snippet,
				Source:  "哔哩哔哩",
			})
		}
		// 一页不足 20 条说明已经翻到最后一页，提前结束。
		if pageCount < pageSize || len(results) >= limit {
			break
		}
		select {
		case <-ctx.Done():
			return results, nil
		case <-time.After(300 * time.Millisecond):
		}
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("哔哩哔哩没有搜到「%s」的视频", keyword)
	}
	return results, nil
}

// resolveBilibiliMedia 把视频页面解析成可下载的 mp4 直链（清晰度 360P）。
func resolveBilibiliMedia(ctx context.Context, pageURL string) (string, bool) {
	match := bvidPattern.FindString(pageURL)
	if match == "" {
		return "", false
	}
	call := func(target string) ([]byte, bool) {
		body, status, err := bilibiliGet(ctx, target)
		return body, err == nil && status == http.StatusOK
	}
	viewBody, ok := call("https://api.bilibili.com/x/web-interface/view?bvid=" + match)
	if !ok {
		return "", false
	}
	var view struct {
		Code int `json:"code"`
		Data struct {
			Pages []struct {
				Cid int64 `json:"cid"`
			} `json:"pages"`
		} `json:"data"`
	}
	if err := json.Unmarshal(viewBody, &view); err != nil || view.Code != 0 || len(view.Data.Pages) == 0 {
		return "", false
	}
	playBody, ok := call(fmt.Sprintf(
		"https://api.bilibili.com/x/player/playurl?bvid=%s&cid=%d&qn=16&fnval=1",
		match, view.Data.Pages[0].Cid))
	if !ok {
		return "", false
	}
	var play struct {
		Code int `json:"code"`
		Data struct {
			Durl []struct {
				URL string `json:"url"`
			} `json:"durl"`
		} `json:"data"`
	}
	if err := json.Unmarshal(playBody, &play); err != nil || play.Code != 0 || len(play.Data.Durl) == 0 {
		return "", false
	}
	media := strings.TrimSpace(play.Data.Durl[0].URL)
	return media, media != ""
}

// stripHTMLTags 去掉搜索结果里的高亮标签并还原转义字符。
func stripHTMLTags(value string) string {
	cleaned := htmlTagPattern.ReplaceAllString(value, "")
	return strings.TrimSpace(html.UnescapeString(cleaned))
}

const searchUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36"

var (
	bvidPattern        = regexp.MustCompile(`BV[0-9A-Za-z]{10}`)
	htmlTagPattern     = regexp.MustCompile(`<[^>]+>`)
	baiduResultPattern = regexp.MustCompile(`(?s)<a[^>]+href="(https?://(?:www\.)?baidu\.com/link\?url=[^"]+)"[^>]*>(.*?)</a>`)
)

var (
	bilibiliJarOnce sync.Once
	bilibiliJar     http.CookieJar
	bilibiliWarm    sync.Once

	baiduJarOnce sync.Once
	baiduJar     http.CookieJar
	baiduWarm    sync.Once
)

func bilibiliClient() *http.Client {
	bilibiliJarOnce.Do(func() {
		if jar, err := cookiejar.New(nil); err == nil {
			bilibiliJar = jar
		}
	})
	return &http.Client{Timeout: 30 * time.Second, Jar: bilibiliJar}
}

// bilibiliHome 预热：哔哩哔哩接口要求带访客 cookie，否则直接返回 412。
func bilibiliHome(ctx context.Context) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://www.bilibili.com/", nil)
	if err != nil {
		return err
	}
	request.Header.Set("User-Agent", searchUserAgent)
	response, err := bilibiliClient().Do(request)
	if err != nil {
		return err
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
	return response.Body.Close()
}

func bilibiliWarmup(ctx context.Context) {
	bilibiliWarm.Do(func() { _ = bilibiliHome(ctx) })
}

// bilibiliGet 带访客 cookie 请求；遇到 412 重新预热后重试一次。
func bilibiliGet(ctx context.Context, target string) ([]byte, int, error) {
	bilibiliWarmup(ctx)
	fetch := func() ([]byte, int, error) {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
		if err != nil {
			return nil, 0, err
		}
		request.Header.Set("User-Agent", searchUserAgent)
		request.Header.Set("Referer", "https://www.bilibili.com/")
		request.Header.Set("Accept", "application/json, text/plain, */*")
		request.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
		response, err := bilibiliClient().Do(request)
		if err != nil {
			return nil, 0, err
		}
		body, readErr := io.ReadAll(io.LimitReader(response.Body, 4<<20))
		response.Body.Close()
		return body, response.StatusCode, readErr
	}
	body, status, err := fetch()
	if err == nil && status == http.StatusPreconditionFailed {
		_ = bilibiliHome(ctx)
		body, status, err = fetch()
	}
	return body, status, err
}
