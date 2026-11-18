package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// 模型网关：所有模型调用都从这里出（对齐 AGENTS 第 5 节的约定）。
//
// 支持两类后端：
//   1. 本地 Ollama（http://127.0.0.1:11434/v1，无需密钥）
//   2. 任何 OpenAI 兼容接口（需要 Base URL + API Key）
//
// 密钥只存在本机库的设置里，不下发到页面。

const (
	modelBaseURLKey  = "model.baseUrl"
	modelAPIKeyKey   = "model.apiKey"
	modelNameKey     = "model.name"
	modelPriceInKey  = "model.inputPriceCents"
	modelPriceOutKey = "model.outputPriceCents"
)

type ModelConfig struct {
	BaseURL          string   `json:"baseUrl"`
	Model            string   `json:"model"`
	HasAPIKey        bool     `json:"hasApiKey"`
	InputPriceCents  int64    `json:"inputPriceCents"`
	OutputPriceCents int64    `json:"outputPriceCents"`
	LocalModels      []string `json:"localModels"`
	Available        bool     `json:"available"`
	Message          string   `json:"message"`
}

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatResult struct {
	Text             string `json:"text"`
	Model            string `json:"model"`
	PromptTokens     int64  `json:"promptTokens"`
	CompletionTokens int64  `json:"completionTokens"`
	CostCents        int64  `json:"costCents"`
	ElapsedMs        int64  `json:"elapsedMs"`
	Note             string `json:"note"`
}

func (a *App) settingOr(ctx context.Context, key, fallback string) string {
	if a.store == nil {
		return fallback
	}
	value, err := a.store.GetSetting(ctx, key)
	if err != nil || strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func (a *App) settingInt(ctx context.Context, key string) int64 {
	raw := a.settingOr(ctx, key, "0")
	var value int64
	_, _ = fmt.Sscanf(raw, "%d", &value)
	return value
}

// currentModelConfig 汇总当前配置，并探测本机 Ollama 有哪些模型。
func (a *App) currentModelConfig(ctx context.Context) ModelConfig {
	cfg := ModelConfig{
		BaseURL:          a.settingOr(ctx, modelBaseURLKey, "http://127.0.0.1:11434/v1"),
		Model:            a.settingOr(ctx, modelNameKey, "qwen2.5:3b"),
		InputPriceCents:  a.settingInt(ctx, modelPriceInKey),
		OutputPriceCents: a.settingInt(ctx, modelPriceOutKey),
	}
	if apiKey := a.settingOr(ctx, modelAPIKeyKey, ""); apiKey != "" {
		cfg.HasAPIKey = true
	}
	cfg.LocalModels = detectOllamaModels()
	cfg.Available = cfg.Model != "" && (cfg.BaseURL != "")
	switch {
	case !cfg.Available:
		cfg.Message = "尚未配置模型地址"
	case strings.Contains(cfg.BaseURL, "127.0.0.1") || strings.Contains(cfg.BaseURL, "localhost"):
		if len(cfg.LocalModels) == 0 {
			cfg.Message = "没检测到本机 Ollama 模型，请先 ollama pull"
		} else {
			cfg.Message = "使用本机模型（无 API 费用）"
		}
	default:
		if !cfg.HasAPIKey {
			cfg.Message = "云端口需要填写 API Key"
		} else {
			cfg.Message = "使用云端模型"
		}
	}
	return cfg
}

// detectOllamaModels 查本机 Ollama 的模型清单；服务没起时返回空。
func detectOllamaModels() []string {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://127.0.0.1:11434/api/tags")
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	var payload struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload); err != nil {
		return nil
	}
	out := make([]string, 0, len(payload.Models))
	for _, item := range payload.Models {
		out = append(out, item.Name)
	}
	return out
}

// chatOnce 调用一次对话补全（非流式，先保证可用）。
func (a *App) chatOnce(ctx context.Context, messages []ChatMessage) (ChatResult, error) {
	cfg := a.currentModelConfig(ctx)
	if cfg.Model == "" || cfg.BaseURL == "" {
		return ChatResult{}, fmt.Errorf("尚未配置模型，请到「系统设置 → 模型接入」填写")
	}

	body := map[string]any{
		"model":       cfg.Model,
		"messages":    messages,
		"temperature": 0.7,
		"stream":      false,
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return ChatResult{}, err
	}

	endpoint := strings.TrimRight(cfg.BaseURL, "/") + "/chat/completions"
	reqCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return ChatResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if key := a.settingOr(ctx, modelAPIKeyKey, ""); key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}

	started := time.Now()
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return ChatResult{}, fmt.Errorf("连接模型服务失败：%w", err)
	}
	defer resp.Body.Close()

	payload, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return ChatResult{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return ChatResult{}, fmt.Errorf("模型服务返回 HTTP %d：%s", resp.StatusCode, truncate(string(payload), 200))
	}

	var parsed struct {
		Model   string `json:"model"`
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int64 `json:"prompt_tokens"`
			CompletionTokens int64 `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(payload, &parsed); err != nil {
		return ChatResult{}, fmt.Errorf("模型返回格式无法解析：%w", err)
	}
	if len(parsed.Choices) == 0 {
		return ChatResult{}, fmt.Errorf("模型没有返回内容")
	}

	elapsed := time.Since(started).Milliseconds()
	cost := (parsed.Usage.PromptTokens*cfg.InputPriceCents +
		parsed.Usage.CompletionTokens*cfg.OutputPriceCents) / 1_000_000

	result := ChatResult{
		Text:             strings.TrimSpace(parsed.Choices[0].Message.Content),
		Model:            cfg.Model,
		PromptTokens:     parsed.Usage.PromptTokens,
		CompletionTokens: parsed.Usage.CompletionTokens,
		CostCents:        cost,
		ElapsedMs:        elapsed,
	}
	if cfg.InputPriceCents == 0 && cfg.OutputPriceCents == 0 {
		result.Note = "未配置单价，费用按 0 计（本机模型无 API 费用）"
	}
	_ = a.recordUsage(ctx, result)
	return result, nil
}

// recordUsage 每次调用写一条用量记录，供「结果回显」回溯（FR-HOME-07）。
func (a *App) recordUsage(ctx context.Context, result ChatResult) error {
	if a.store == nil {
		return nil
	}
	_, err := a.store.db.ExecContext(ctx,
		`INSERT INTO usage_records(model, prompt_tokens, completion_tokens, cost_cents, elapsed_ms, created_at)
		 VALUES(?, ?, ?, ?, ?, ?)`,
		result.Model, result.PromptTokens, result.CompletionTokens, result.CostCents,
		result.ElapsedMs, time.Now().UTC().Format(time.RFC3339))
	return err
}

// ---------- HTTP ----------

func (a *App) handleModelConfigGet(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, a.currentModelConfig(r.Context()))
}

func (a *App) handleModelConfigSave(w http.ResponseWriter, r *http.Request) {
	if a.store == nil {
		fail(w, http.StatusServiceUnavailable, "本地库未初始化")
		return
	}
	var body struct {
		BaseURL          string `json:"baseUrl"`
		Model            string `json:"model"`
		APIKey           string `json:"apiKey"`
		InputPriceCents  int64  `json:"inputPriceCents"`
		OutputPriceCents int64  `json:"outputPriceCents"`
	}
	if err := decodeJSON(r, &body); err != nil {
		fail(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	baseURL := strings.TrimSpace(body.BaseURL)
	if baseURL != "" && !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
		fail(w, http.StatusBadRequest, "模型地址必须以 http:// 或 https:// 开头")
		return
	}
	ctx := r.Context()
	_ = a.store.SetSetting(ctx, modelBaseURLKey, baseURL)
	_ = a.store.SetSetting(ctx, modelNameKey, strings.TrimSpace(body.Model))
	_ = a.store.SetSetting(ctx, modelPriceInKey, fmt.Sprintf("%d", body.InputPriceCents))
	_ = a.store.SetSetting(ctx, modelPriceOutKey, fmt.Sprintf("%d", body.OutputPriceCents))
	// 空字符串表示「不改密钥」，避免页面把密钥清空
	if strings.TrimSpace(body.APIKey) != "" {
		_ = a.store.SetSetting(ctx, modelAPIKeyKey, strings.TrimSpace(body.APIKey))
	}
	writeJSON(w, http.StatusOK, a.currentModelConfig(ctx))
}

func (a *App) handleModelTest(w http.ResponseWriter, r *http.Request) {
	result, err := a.chatOnce(r.Context(), []ChatMessage{{Role: "user", Content: "只回复两个字：可用"}})
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "message": "模型可用", "reply": result.Text,
		"elapsedMs": result.ElapsedMs, "model": result.Model,
	})
}

func (a *App) handleChat(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Messages []ChatMessage `json:"messages"`
	}
	if err := decodeJSON(r, &body); err != nil {
		fail(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	cleaned := make([]ChatMessage, 0, len(body.Messages))
	for _, msg := range body.Messages {
		role := strings.TrimSpace(msg.Role)
		if role != "user" && role != "assistant" && role != "system" {
			continue
		}
		if strings.TrimSpace(msg.Content) == "" {
			continue
		}
		cleaned = append(cleaned, ChatMessage{Role: role, Content: msg.Content})
	}
	if len(cleaned) == 0 {
		fail(w, http.StatusBadRequest, "请先输入内容")
		return
	}

	result, err := a.chatOnce(r.Context(), cleaned)
	if err != nil {
		fail(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (a *App) handleUsageList(w http.ResponseWriter, r *http.Request) {
	if a.store == nil {
		fail(w, http.StatusServiceUnavailable, "本地库未初始化")
		return
	}
	rows, err := a.store.db.QueryContext(r.Context(),
		`SELECT model, prompt_tokens, completion_tokens, cost_cents, elapsed_ms, created_at
		   FROM usage_records ORDER BY id DESC LIMIT 50`)
	if err != nil {
		fail(w, http.StatusInternalServerError, "读取用量失败："+err.Error())
		return
	}
	defer rows.Close()

	type usageItem struct {
		Model            string `json:"model"`
		PromptTokens     int64  `json:"promptTokens"`
		CompletionTokens int64  `json:"completionTokens"`
		CostCents        int64  `json:"costCents"`
		ElapsedMs        int64  `json:"elapsedMs"`
		CreatedAt        string `json:"createdAt"`
	}
	items := []usageItem{}
	for rows.Next() {
		var item usageItem
		if err := rows.Scan(&item.Model, &item.PromptTokens, &item.CompletionTokens,
			&item.CostCents, &item.ElapsedMs, &item.CreatedAt); err != nil {
			fail(w, http.StatusInternalServerError, "读取用量失败")
			return
		}
		items = append(items, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}
