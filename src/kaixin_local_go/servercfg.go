package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// 服务端配置：地址存在本地库 settings 里；配置后的连通性测试与版本查询
// 都由 sidecar 代发，避免前端直连带来的 CORS 与混合内容问题。

const serverURLKey = "server.url"

type ServerConfig struct {
	URL     string `json:"url"`
	Saved   bool   `json:"saved"`
	Message string `json:"message"`
}

func (a *App) currentServerURL(ctx context.Context) string {
	if a.store == nil {
		return ""
	}
	value, err := a.store.GetSetting(ctx, serverURLKey)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(value)
}

// validateServerURL 只接受 http/https，并去掉尾部斜杠。
func validateServerURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("请填写服务器地址")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("地址格式不正确")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("地址必须以 http:// 或 https:// 开头")
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("地址缺少主机名")
	}
	return strings.TrimRight(raw, "/"), nil
}

func (a *App) handleServerConfigGet(w http.ResponseWriter, r *http.Request) {
	current := a.currentServerURL(r.Context())
	state := ServerConfig{URL: current, Saved: current != "", Message: "未配置服务器，使用内置目录与本地数据"}
	if current != "" {
		state.Message = "已配置服务器"
	}
	writeJSON(w, http.StatusOK, state)
}

func (a *App) handleServerConfigSet(w http.ResponseWriter, r *http.Request) {
	var request struct {
		URL string `json:"url"`
	}
	if err := decodeJSON(r, &request); err != nil {
		fail(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	if a.store == nil {
		fail(w, http.StatusServiceUnavailable, "本地库未初始化，无法保存配置")
		return
	}

	// 允许清空配置（退回纯本地模式）
	if strings.TrimSpace(request.URL) == "" {
		if err := a.store.SetSetting(r.Context(), serverURLKey, ""); err != nil {
			fail(w, http.StatusInternalServerError, "清除配置失败："+err.Error())
			return
		}
		writeJSON(w, http.StatusOK, ServerConfig{Message: "已清除服务器配置，回到纯本地模式"})
		return
	}

	normalized, err := validateServerURL(request.URL)
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := a.store.SetSetting(r.Context(), serverURLKey, normalized); err != nil {
		fail(w, http.StatusInternalServerError, "保存配置失败："+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, ServerConfig{URL: normalized, Saved: true, Message: "已保存服务器地址"})
}

// fetchJSON 用小超时代发一次 GET，供测试连接与版本查询复用。
func fetchJSON(ctx context.Context, target string) ([]byte, int, error) {
	reqCtx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, target, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return body, resp.StatusCode, nil
}

type ServerTestResult struct {
	OK        bool   `json:"ok"`
	Status    int    `json:"status"`
	Message   string `json:"message"`
	Body      string `json:"body"`
	LatencyMs int64  `json:"latencyMs"`
}

func (a *App) handleServerTest(w http.ResponseWriter, r *http.Request) {
	base := a.currentServerURL(r.Context())
	if base == "" {
		fail(w, http.StatusBadRequest, "还没有配置服务器地址")
		return
	}

	started := time.Now()
	body, status, err := fetchJSON(r.Context(), base+"/api/health")
	latency := time.Since(started).Milliseconds()
	if err != nil {
		writeJSON(w, http.StatusOK, ServerTestResult{
			OK: false, Status: 0, LatencyMs: latency,
			Message: "连接失败：" + err.Error(),
		})
		return
	}
	ok := status == http.StatusOK
	message := fmt.Sprintf("连接成功 · 延迟 %d ms", latency)
	if !ok {
		message = fmt.Sprintf("服务器返回 HTTP %d · 延迟 %d ms", status, latency)
	}
	writeJSON(w, http.StatusOK, ServerTestResult{
		OK: ok, Status: status, Message: message, Body: truncate(string(body), 200),
		LatencyMs: latency,
	})
}

type UpdateNote struct {
	Version string `json:"version"`
	Date    string `json:"date"`
	Note    string `json:"note"`
}

type UpdateCheckResult struct {
	Configured bool         `json:"configured"`
	OK         bool         `json:"ok"`
	Message    string       `json:"message"`
	Notes      []UpdateNote `json:"notes"`
}

// handleUpdateCheck：向服务端的 /api/version-notes 取版本说明。
// 这里只做「查得到什么」，真正的下载安装仍未接入（见 docs/13）。
func (a *App) handleUpdateCheck(w http.ResponseWriter, r *http.Request) {
	base := a.currentServerURL(r.Context())
	if base == "" {
		writeJSON(w, http.StatusOK, UpdateCheckResult{
			Configured: false,
			Message:    "未配置服务器，无法检查新版本",
		})
		return
	}

	body, status, err := fetchJSON(r.Context(), base+"/api/version-notes")
	if err != nil {
		writeJSON(w, http.StatusOK, UpdateCheckResult{
			Configured: true,
			Message:    "连接更新服务失败：" + err.Error(),
		})
		return
	}
	if status != http.StatusOK {
		writeJSON(w, http.StatusOK, UpdateCheckResult{
			Configured: true,
			Message:    fmt.Sprintf("更新服务返回 HTTP %d", status),
		})
		return
	}

	notes, parseErr := parseVersionNotes(body)
	if parseErr != nil {
		writeJSON(w, http.StatusOK, UpdateCheckResult{
			Configured: true,
			Message:    "版本说明解析失败：" + parseErr.Error(),
		})
		return
	}
	message := "已获取版本说明"
	if len(notes) == 0 {
		message = "服务端暂无版本说明"
	}
	writeJSON(w, http.StatusOK, UpdateCheckResult{
		Configured: true, OK: true, Message: message, Notes: notes,
	})
}

// rawNote 覆盖服务端实际字段（title / detail / publishedAt）与常见别名。
type rawNote struct {
	Version     string `json:"version"`
	Title       string `json:"title"`
	Detail      string `json:"detail"`
	Note        string `json:"note"`
	Content     string `json:"content"`
	Date        string `json:"date"`
	PublishedAt string `json:"publishedAt"`
	CreatedAt   string `json:"createdAt"`
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func (r rawNote) toNote() UpdateNote {
	return UpdateNote{
		Version: r.Version,
		Date:    firstNonEmpty(r.Date, r.PublishedAt, r.CreatedAt),
		Note:    firstNonEmpty(r.Note, r.Content, r.Detail, r.Title),
	}
}

// parseVersionNotes 兼容三种返回：裸数组、{data:[...]}、{notes:[...]}。
func parseVersionNotes(body []byte) ([]UpdateNote, error) {
	convert := func(items []rawNote) []UpdateNote {
		out := make([]UpdateNote, 0, len(items))
		for _, item := range items {
			out = append(out, item.toNote())
		}
		return out
	}

	var list []rawNote
	if err := json.Unmarshal(body, &list); err == nil && list != nil {
		return convert(list), nil
	}

	var env struct {
		Data  []rawNote `json:"data"`
		Notes []rawNote `json:"notes"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, err
	}
	if env.Notes != nil {
		return convert(env.Notes), nil
	}
	return convert(env.Data), nil
}
