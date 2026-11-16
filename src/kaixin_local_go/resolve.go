package main

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
)

// SitePage 是站点里一个「能解决某类需求」的具体页面。
// 页面地图不是穷举所有页面，只登记高频路径；没登记的由站内搜索兜底。
type SitePage struct {
	Key          string   `json:"key"`
	Title        string   `json:"title"`
	URL          string   `json:"url"`
	Keywords     []string `json:"keywords"`
	SuccessWhen  string   `json:"successWhen"`
	RequiresAuth bool     `json:"requiresAuth"`
}

type SiteEntry struct {
	ID           string     `json:"id"`
	Name         string     `json:"name"`
	Category     string     `json:"category"`
	Keywords     []string   `json:"keywords"`
	URL          string     `json:"url"`
	LoginURL     string     `json:"loginUrl"`
	Note         string     `json:"note"`
	Capabilities []string   `json:"capabilities"`
	SearchURL    string     `json:"searchUrl"`
	Pages        []SitePage `json:"pages"`
}

// ---------- 需求解析 ----------

type NeedIntent string

const (
	IntentAuth      NeedIntent = "auth"      // 只是想登录/授权
	IntentRead      NeedIntent = "read"      // 读数据
	IntentPublish   NeedIntent = "publish"   // 发内容
	IntentConfigure NeedIntent = "configure" // 配置类
)

// 动作词表：先判发布（互斥性最强），再判授权，最后判读取。
var publishWords = []string{"发布", "上传", "投稿", "发表", "发视频", "发笔记", "发商品"}
var authWords = []string{"登录", "授权", "扫码", "绑定"}
var readWords = []string{"查看", "统计", "多少", "销量", "订单", "评价", "评论", "播放", "点赞", "粉丝", "数据", "日报", "报表"}

// 数据对象词表：用于在站点内搜索时构造查询词。
var objectWords = []string{
	"销量", "订单", "评价", "评论", "播放量", "点赞量", "完播率", "粉丝", "库存", "退款", "售后",
	"播放", "点赞", "商品", "短视频", "笔记",
}

func detectIntent(need string) NeedIntent {
	contains := func(words []string) bool {
		for _, word := range words {
			if strings.Contains(need, word) {
				return true
			}
		}
		return false
	}
	read := contains(readWords)
	publish := contains(publishWords)
	auth := contains(authWords)

	// 「我发布的视频有多少播放量」里同时出现"发布"和"播放量"，
	// 这里必须判成"读数据"——看的是已经发出去的内容，不是要发新内容。
	switch {
	case read:
		return IntentRead
	case publish:
		return IntentPublish
	case auth:
		return IntentAuth
	default:
		return IntentConfigure
	}
}

// extractObjects 提取需求里出现的数据对象，并去掉被更长词包含的短词。
// 例：命中「播放量」后不再保留「播放」，否则站内搜索词会变成「播放量 播放」。
func extractObjects(need string) []string {
	matched := map[string]bool{}
	for _, word := range objectWords {
		if word != "" && strings.Contains(need, word) {
			matched[word] = true
		}
	}
	out := []string{}
	for _, word := range objectWords {
		if !matched[word] {
			continue
		}
		covered := false
		for _, other := range objectWords {
			if other != word && matched[other] && strings.Contains(other, word) {
				covered = true
				break
			}
		}
		if !covered {
			out = append(out, word)
		}
	}
	return out
}

// ---------- 结果结构 ----------

type SiteBrief struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	URL      string `json:"url"`
	Category string `json:"category"`
}

type ResolveStep struct {
	Order        int    `json:"order"`
	Kind         string `json:"kind"` // login / open-page / search / manual
	Title        string `json:"title"`
	URL          string `json:"url"`
	Why          string `json:"why"`
	SuccessWhen  string `json:"successWhen,omitempty"`
	RequiresAuth bool   `json:"requiresAuth"`
}

type ResolveResult struct {
	Need            string        `json:"need"`
	Intent          string        `json:"intent"`
	Objects         []string      `json:"objects"`
	Site            *SiteBrief    `json:"site,omitempty"`
	MatchedKeywords []string      `json:"matchedKeywords,omitempty"`
	Steps           []ResolveStep `json:"steps"`
	Alternatives    []SiteBrief   `json:"alternatives,omitempty"`
	Advice          string        `json:"advice,omitempty"`
}

// pageScore 计算页面与需求的匹配度：关键词命中按字数加权，意图不符直接淘汰。
func pageScore(page SitePage, need string, intent NeedIntent, objects []string) (int, []string) {
	// 发布意图只认发布页；其他意图排除发布页。互斥，避免"发布"和"看数据"互抢。
	isPublishPage := page.Key == "publish"
	if intent == IntentPublish && !isPublishPage {
		return 0, nil
	}
	if intent != IntentPublish && isPublishPage {
		return 0, nil
	}

	score := 0
	hits := []string{}
	for _, keyword := range page.Keywords {
		if keyword != "" && strings.Contains(need, keyword) {
			score += len([]rune(keyword)) + 2
			hits = append(hits, keyword)
		}
	}
	for _, object := range objects {
		for _, keyword := range page.Keywords {
			if strings.Contains(keyword, object) {
				score++
			}
		}
	}

	// 发布意图下，发布页本身就是答案：用户说"发布到抖音"时不会逐字说出页面关键词。
	if intent == IntentPublish && isPublishPage && score == 0 {
		score = 3
		hits = append(hits, "发布")
	}
	return score, hits
}

func siteBrief(site *SiteEntry) *SiteBrief {
	if site == nil {
		return nil
	}
	return &SiteBrief{ID: site.ID, Name: site.Name, URL: site.URL, Category: site.Category}
}

// ResolveNeed 把一句需求解析成「去哪个站点、打开哪个页面、怎么算到位」。
// 原则：已登记页面优先；没登记就用站内搜索定位；连搜索都没有就回首页并给出人工提示。
func ResolveNeed(sites []SiteEntry, need string) ResolveResult {
	need = strings.TrimSpace(need)
	result := ResolveResult{Need: need, Steps: []ResolveStep{}}
	if need == "" {
		result.Advice = "需求为空：请描述你想完成的事，例如「查看店铺每天的销量和评论」。"
		return result
	}

	intent := detectIntent(need)
	objects := extractObjects(need)
	result.Intent = string(intent)
	result.Objects = objects

	site := pickSite(sites, need, intent, objects)
	if site == nil {
		result.Advice = "没有识别出对应网站：请在需求里写明平台，例如「抖店」「抖音创作服务平台」「小红书」。"
		return result
	}
	result.Site = siteBrief(site)
	result.MatchedKeywords = matchedKeywords(site, need)

	// 选最匹配的页面
	bestScore := 0
	var best *SitePage
	var bestHits []string
	for i := range site.Pages {
		score, hits := pageScore(site.Pages[i], need, intent, objects)
		if score > bestScore {
			bestScore = score
			page := site.Pages[i]
			best = &page
			bestHits = hits
		}
	}

	order := 1
	// 需要登录的页面，先给一步登录
	needsAuth := best != nil && best.RequiresAuth
	if needsAuth || intent == IntentAuth {
		result.Steps = append(result.Steps, ResolveStep{
			Order:       order,
			Kind:        "login",
			Title:       "登录" + site.Name,
			URL:         site.LoginURL,
			Why:         "该页面需要登录态，程序只能把页面打开，账号密码由你自己输入",
			SuccessWhen: "页面出现账号头像或进入首页",
		})
		order++
	}

	switch {
	case best != nil:
		result.Steps = append(result.Steps, ResolveStep{
			Order:        order,
			Kind:         "open-page",
			Title:        site.Name + " · " + best.Title,
			URL:          best.URL,
			Why:          "命中关键词：" + strings.Join(bestHits, "、"),
			SuccessWhen:  best.SuccessWhen,
			RequiresAuth: best.RequiresAuth,
		})
	case site.SearchURL != "":
		query := need
		if len(objects) > 0 {
			query = strings.Join(objects, " ")
		}
		url := strings.ReplaceAll(site.SearchURL, "{q}", query)
		result.Steps = append(result.Steps, ResolveStep{
			Order:       order,
			Kind:        "search",
			Title:       "在" + site.Name + "站内搜索「" + query + "」",
			URL:         url,
			Why:         "该站没有登记对应页面，用站内搜索定位最接近的入口",
			SuccessWhen: "搜索结果页出现相关入口，点进去即可",
		})
		order++
		result.Advice = "进入搜索结果后，选择标题最接近「" + query + "」的入口；如果常用，可以把它登记成页面，下次直接跳转。"
	default:
		result.Steps = append(result.Steps, ResolveStep{
			Order:       order,
			Kind:        "manual",
			Title:       "打开" + site.Name,
			URL:         site.URL,
			Why:         "该站没有登记页面，也没有站内搜索可用",
			SuccessWhen: "手动找到目标入口后，可登记为页面以便下次直达",
		})
		result.Advice = "这个站点还需要人工找一次入口。找到后告诉我页面地址，我可以登记成页面地图。"
	}

	// 备选站点：能力覆盖面重合的其他站
	for i := range sites {
		other := &sites[i]
		if other.ID == site.ID {
			continue
		}
		if len(other.Pages) == 0 {
			continue
		}
		if sharesCapability(other, site, need) {
			result.Alternatives = append(result.Alternatives, *siteBrief(other))
		}
		if len(result.Alternatives) >= 3 {
			break
		}
	}
	return result
}

// pickSite 选站点，分三层权重（从强到弱）：
//
//	① 平台身份命中：关键词出现在站点名里（"哔哩哔哩""抖音""抖店"）——最强，用户点名了平台；
//	② 能力词命中：关键词只是能力（播放量、销量），说明用户提的是需求不是平台；
//	③ 页面覆盖：该站点有能解决此意图的页面，加分，用来在同分站点里选更合适的那个。
//
// 例：「在哔哩哔哩看播放量」应当去 B 站，而不是因为"播放量"去了抖音创作平台。
func pickSite(sites []SiteEntry, need string, intent NeedIntent, objects []string) *SiteEntry {
	var best *SiteEntry
	bestScore := 0
	for i := range sites {
		site := &sites[i]
		identity, capability := 0, 0
		for _, keyword := range site.Keywords {
			if keyword == "" || !strings.Contains(need, keyword) {
				continue
			}
			runes := len([]rune(keyword))
			if strings.Contains(site.Name, keyword) {
				if runes > identity {
					identity = runes
				}
			} else if runes > capability {
				capability = runes
			}
		}
		if identity == 0 && capability == 0 {
			continue
		}
		score := identity*100 + capability*10 + 5

		// 页面覆盖加分
		covered := false
		for _, page := range site.Pages {
			if intent == IntentPublish && page.Key == "publish" {
				covered = true
				break
			}
			if value, _ := pageScore(page, need, intent, objects); value > 0 {
				covered = true
				break
			}
		}
		if covered {
			score += 100
		}
		if score > bestScore {
			bestScore = score
			best = site
		}
	}
	return best
}

func matchedKeywords(site *SiteEntry, need string) []string {
	hits := []string{}
	for _, keyword := range site.Keywords {
		if keyword != "" && strings.Contains(need, keyword) {
			hits = append(hits, keyword)
		}
	}
	sort.SliceStable(hits, func(i, j int) bool { return len(hits[i]) > len(hits[j]) })
	if len(hits) > 5 {
		hits = hits[:5]
	}
	return hits
}

func sharesCapability(candidate, chosen *SiteEntry, need string) bool {
	for _, keyword := range candidate.Keywords {
		if keyword != "" && strings.Contains(need, keyword) {
			return true
		}
	}
	return false
}

// ---------- HTTP ----------

func (a *App) handleSitesResolve(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Need  string `json:"need"`
		Skill string `json:"skill"`
	}
	if err := decodeJSON(r, &request); err != nil {
		fail(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	need := strings.TrimSpace(request.Need)
	if need == "" {
		need = strings.TrimSpace(request.Skill)
	}
	result := ResolveNeed(a.sites, need)
	writeJSON(w, http.StatusOK, result)
}

// handleSiteOpenPage：按站点 + 页面 key 打开浏览器，供前端「去这里」按钮调用。
func (a *App) handleSiteOpenPage(w http.ResponseWriter, r *http.Request) {
	var request struct {
		SiteID string `json:"siteId"`
		Page   string `json:"page"`
		URL    string `json:"url"`
	}
	if err := decodeJSON(r, &request); err != nil {
		fail(w, http.StatusBadRequest, "请求格式错误")
		return
	}

	target := strings.TrimSpace(request.URL)
	if target == "" {
		site := findSite(a.sites, request.SiteID)
		if site == nil {
			fail(w, http.StatusBadRequest, "未知网站："+request.SiteID)
			return
		}
		target = site.LoginURL
		if request.Page != "" {
			for _, page := range site.Pages {
				if page.Key == request.Page {
					target = page.URL
					break
				}
			}
		}
	}
	// 只允许打开 http/https，避免被当成任意协议处理器。
	if !strings.HasPrefix(target, "https://") && !strings.HasPrefix(target, "http://") {
		fail(w, http.StatusBadRequest, "只支持打开 http/https 地址")
		return
	}
	if err := openBrowser(target); err != nil {
		fail(w, http.StatusInternalServerError, fmt.Sprintf("打开浏览器失败：%v", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"state": "opened", "url": target})
}
