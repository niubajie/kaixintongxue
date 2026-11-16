package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
)

//go:embed sites.json
var sitesJSON []byte

func loadSites() ([]SiteEntry, error) {
	var sites []SiteEntry
	if err := json.Unmarshal(sitesJSON, &sites); err != nil {
		return nil, fmt.Errorf("解析网站目录失败: %w", err)
	}
	return sites, nil
}

func findSite(sites []SiteEntry, id string) *SiteEntry {
	for i := range sites {
		if sites[i].ID == id {
			return &sites[i]
		}
	}
	return nil
}

// matchSite 返回文本命中的网站，关键词更长者优先，避免「微信」抢走「微信公众号」。
func matchSite(sites []SiteEntry, text string) *SiteEntry {
	var best *SiteEntry
	bestLength := 0
	for i := range sites {
		for _, keyword := range sites[i].Keywords {
			if keyword != "" && strings.Contains(text, keyword) && len(keyword) > bestLength {
				best = &sites[i]
				bestLength = len(keyword)
			}
		}
	}
	return best
}

func (a *App) handleSites(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"sites": a.sites})
}

// handleAuthOpen：用系统默认浏览器打开网站登录页，桌面端与网页端都可用。
func (a *App) handleAuthOpen(w http.ResponseWriter, r *http.Request) {
	var request struct {
		SiteID string `json:"siteId"`
	}
	if err := decodeJSON(r, &request); err != nil || request.SiteID == "" {
		fail(w, http.StatusBadRequest, "缺少网站标识")
		return
	}
	site := findSite(a.sites, request.SiteID)
	if site == nil {
		fail(w, http.StatusBadRequest, "未知网站："+request.SiteID)
		return
	}
	if err := openBrowser(site.LoginURL); err != nil {
		fail(w, http.StatusInternalServerError, "打开浏览器失败："+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"state": "opened", "url": site.LoginURL})
}

func openBrowser(url string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		return exec.Command("open", url).Start()
	default:
		return exec.Command("xdg-open", url).Start()
	}
}
