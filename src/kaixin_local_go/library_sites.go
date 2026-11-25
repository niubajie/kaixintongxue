package main

import (
	"context"
	"net/http"
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
}

func (a *App) loadSites(ctx context.Context, onlyEnabled bool) ([]SearchSite, error) {
	query := `SELECT id, name, domain, enabled, created_at FROM search_sites`
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
		if err := rows.Scan(&item.ID, &item.Name, &item.Domain, &enabled, &item.CreatedAt); err != nil {
			return nil, err
		}
		item.Enabled = enabled == 1
		sites = append(sites, item)
	}
	return sites, rows.Err()
}

// normalizeDomain 只保留域名，去掉协议、路径与首尾空格。
func normalizeDomain(value string) string {
	domain := strings.TrimSpace(strings.ToLower(value))
	domain = strings.TrimPrefix(domain, "https://")
	domain = strings.TrimPrefix(domain, "http://")
	if index := strings.IndexAny(domain, "/?#"); index >= 0 {
		domain = domain[:index]
	}
	return strings.Trim(domain, " .")
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
