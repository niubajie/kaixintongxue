package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// 外网搜索适配层：页面只认「关键词 → 结果」，具体供应商由环境变量切换。
// - KAIXIN_SEARCH_PROVIDER=bing|demo 显式指定；
// - 未指定时，有 BING_SEARCH_KEY 走必应，否则用演示源（便于无 Key 环境跑通流程）。
type webResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
	Source  string `json:"source"`
}

func resolveSearchProvider() string {
	if custom := strings.ToLower(strings.TrimSpace(os.Getenv("KAIXIN_SEARCH_PROVIDER"))); custom != "" {
		return custom
	}
	if os.Getenv("BING_SEARCH_KEY") != "" {
		return "bing"
	}
	return "demo"
}

func webSearch(ctx context.Context, keyword string, limit int) ([]webResult, string, error) {
	provider := resolveSearchProvider()
	if provider == "bing" {
		results, err := bingSearch(ctx, keyword, limit)
		return results, "bing", err
	}
	return demoSearch(keyword, limit), "demo", nil
}

// webSearchWithSites 按「网站设置」限定来源：对每个启用站点拼 `site:` 查询后合并。
// 没有启用任何站点时退回全网搜索。
func webSearchWithSites(ctx context.Context, keyword string, limit int, sites []SearchSite) ([]webResult, string, error) {
	if resolveSearchProvider() != "bing" {
		return demoSiteSearch(keyword, limit, sites), "demo", nil
	}
	if len(sites) == 0 {
		results, err := bingSearch(ctx, keyword, limit)
		return results, "bing", err
	}
	if limit <= 0 || limit > 10 {
		limit = 5
	}
	per := limit / len(sites)
	if per < 1 {
		per = 1
	}
	merged := make([]webResult, 0, limit)
	for _, site := range sites {
		results, err := bingSearch(ctx, keyword+" site:"+site.Domain, per)
		if err != nil {
			continue
		}
		for _, item := range results {
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
		return nil, "bing", fmt.Errorf("在已选的网站中没有搜到结果")
	}
	return merged, "bing", nil
}

func demoSiteSearch(keyword string, limit int, sites []SearchSite) []webResult {
	if len(sites) == 0 {
		return demoSearch(keyword, limit)
	}
	if limit <= 0 || limit > 10 {
		limit = 3
	}
	results := make([]webResult, 0, limit)
	for index := 0; index < limit; index++ {
		site := sites[index%len(sites)]
		results = append(results, webResult{
			Title:   keyword + " · " + site.Name,
			URL:     "https://" + site.Domain,
			Snippet: "（演示数据）来自「" + site.Name + "」的结果。配置 BING_SEARCH_KEY 后按 site:" + site.Domain + " 返回真实内容。",
			Source:  site.Name,
		})
	}
	return results
}

func bingSearch(ctx context.Context, keyword string, limit int) ([]webResult, error) {
	key := os.Getenv("BING_SEARCH_KEY")
	if key == "" {
		return nil, fmt.Errorf("未配置 BING_SEARCH_KEY")
	}
	if limit <= 0 || limit > 10 {
		limit = 5
	}
	endpoint := "https://api.bing.microsoft.com/v7.0/search?" + url.Values{
		"q":     {keyword},
		"count": {fmt.Sprintf("%d", limit)},
		"mkt":   {"zh-CN"},
	}.Encode()

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
