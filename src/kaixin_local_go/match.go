package main

import (
	"net/http"
	"sort"
	"strings"
)

const matchThreshold = 3

type MatchCandidate struct {
	SoftwareID string   `json:"softwareId"`
	Name       string   `json:"name"`
	Score      int      `json:"score"`
	Reasons    []string `json:"reasons"`
}

type MatchLine struct {
	Text       string           `json:"text"`
	SoftwareID string           `json:"softwareId"` // 无匹配时为空
	Software   string           `json:"software"`   // 无匹配时为「无」
	Score      int              `json:"score"`
	Reasons    []string         `json:"reasons"`
	SiteID     string           `json:"siteId,omitempty"`     // 命中指定网站时填写，用于授权
	Site       string           `json:"site,omitempty"`       // 命中网站名称，如「抖音」
	SiteURL    string           `json:"siteUrl,omitempty"`    // 命中网站网址，追加在技能说明后面
	Candidates []MatchCandidate `json:"candidates,omitempty"` // 前 3 个候选，便于手动改选
}

type MatchResult struct {
	Lines   []MatchLine `json:"lines"`
	Content string      `json:"content"` // 每行末尾带【软件名】，无匹配为【无】
}

// scoreSoftware：关键词命中 ×2 + 能力标签命中 ×3，附可解释理由。
func scoreSoftware(entry SoftwareEntry, text string) (int, []string) {
	score := 0
	reasons := []string{}
	for _, keyword := range entry.Keywords {
		if keyword != "" && strings.Contains(text, keyword) {
			score += 2
			reasons = append(reasons, "命中关键词「"+keyword+"」")
		}
	}
	for _, capability := range entry.Capabilities {
		if capability != "" && strings.Contains(text, capability) {
			score += 3
			reasons = append(reasons, "命中能力「"+capability+"」")
		}
	}
	return score, reasons
}

func matchLine(entries []SoftwareEntry, text string) MatchLine {
	line := MatchLine{Text: text, Software: "无"}
	candidates := []MatchCandidate{}
	for _, entry := range entries {
		score, reasons := scoreSoftware(entry, text)
		if score >= matchThreshold {
			candidates = append(candidates, MatchCandidate{SoftwareID: entry.ID, Name: entry.Name, Score: score, Reasons: reasons})
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].Score > candidates[j].Score })
	if len(candidates) == 0 {
		return line
	}
	best := candidates[0]
	line.SoftwareID = best.SoftwareID
	line.Software = best.Name
	line.Score = best.Score
	line.Reasons = best.Reasons
	if len(candidates) > 3 {
		candidates = candidates[:3]
	}
	line.Candidates = candidates
	return line
}

// MatchContent：逐行匹配；已带【...】后缀或网址的行先去掉旧标记再重新匹配，保证可以反复点击「配置 AI 软件」。
func MatchContent(entries []SoftwareEntry, content string) MatchResult {
	return MatchContentWithSites(entries, nil, content)
}

// MatchContentWithSites：在软件匹配的基础上识别技能涉及的网站，并把网址追加到该行末尾。
func MatchContentWithSites(entries []SoftwareEntry, sites []SiteEntry, content string) MatchResult {
	result := MatchResult{}
	for _, raw := range strings.Split(content, "\n") {
		text := stripDecorations(strings.TrimSpace(raw))
		if text == "" {
			continue
		}
		line := matchLine(entries, text)
		if site := matchSite(sites, text); site != nil {
			line.SiteID = site.ID
			line.Site = site.Name
			line.SiteURL = site.URL
		}
		result.Lines = append(result.Lines, line)
		result.Content += decorateLine(text, line) + "\n"
	}
	result.Content = strings.TrimRight(result.Content, "\n")
	return result
}

// decorateLine 在技能要求后面追加【软件名】与网站网址。
func decorateLine(text string, line MatchLine) string {
	decorated := text + "【" + line.Software + "】"
	if line.SiteURL != "" {
		decorated += " " + line.SiteURL
	}
	return decorated
}

// stripDecorations 去掉行尾自动追加的网址与【软件名】，便于重新匹配。
func stripDecorations(text string) string {
	text = strings.TrimSpace(text)
	for {
		if idx := strings.LastIndex(text, "【"); idx > 0 && strings.HasSuffix(text, "】") {
			text = strings.TrimSpace(text[:idx])
			continue
		}
		fields := strings.Fields(text)
		if len(fields) > 1 {
			last := fields[len(fields)-1]
			if strings.HasPrefix(last, "http://") || strings.HasPrefix(last, "https://") {
				text = strings.TrimSpace(strings.TrimSuffix(text, last))
				continue
			}
		}
		return text
	}
}

func (a *App) handleMatchSoftware(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Content string `json:"content"`
	}
	if err := decodeJSON(r, &request); err != nil {
		fail(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	if strings.TrimSpace(request.Content) == "" {
		fail(w, http.StatusBadRequest, "请先填写技能内容，再配置 AI 软件。")
		return
	}
	result := MatchContentWithSites(a.catalog, a.sites, request.Content)
	writeJSON(w, http.StatusOK, result)
}

func (a *App) handleCatalog(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"software": a.catalog})
}
