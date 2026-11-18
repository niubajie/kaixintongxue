package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// 流式对话：边生成边下发，避免用户对着空白等几十秒。
// 走 SSE（text/event-stream），一行一个 chunk：
//   data: {"delta":"文本"}
//   data: {"done":true,"model":..,"promptTokens":..,"completionTokens":..,"costCents":..,"elapsedMs":..}
//   data: {"error":"原因"}

type streamChunk struct {
	Delta            string `json:"delta,omitempty"`
	Done             bool   `json:"done,omitempty"`
	Error            string `json:"error,omitempty"`
	Model            string `json:"model,omitempty"`
	PromptTokens     int64  `json:"promptTokens,omitempty"`
	CompletionTokens int64  `json:"completionTokens,omitempty"`
	CostCents        int64  `json:"costCents,omitempty"`
	ElapsedMs        int64  `json:"elapsedMs,omitempty"`
	Estimated        bool   `json:"estimated,omitempty"`
}

func (a *App) handleChatStream(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Messages []ChatMessage `json:"messages"`
	}
	if err := decodeJSON(r, &body); err != nil {
		fail(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	cleaned := cleanMessages(body.Messages)
	if len(cleaned) == 0 {
		fail(w, http.StatusBadRequest, "请先输入内容")
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		fail(w, http.StatusInternalServerError, "当前环境不支持流式输出")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	send := func(chunk streamChunk) {
		payload, err := json.Marshal(chunk)
		if err != nil {
			return
		}
		_, _ = fmt.Fprintf(w, "data: %s\n\n", payload)
		flusher.Flush()
	}

	if err := a.streamChat(r.Context(), cleaned, send); err != nil {
		send(streamChunk{Error: err.Error()})
	}
}

func cleanMessages(messages []ChatMessage) []ChatMessage {
	out := make([]ChatMessage, 0, len(messages))
	for _, msg := range messages {
		role := strings.TrimSpace(msg.Role)
		if role != "user" && role != "assistant" && role != "system" {
			continue
		}
		if strings.TrimSpace(msg.Content) == "" {
			continue
		}
		out = append(out, ChatMessage{Role: role, Content: msg.Content})
	}
	return out
}

func (a *App) streamChat(ctx context.Context, messages []ChatMessage, send func(streamChunk)) error {
	cfg := a.currentModelConfig(ctx)
	if cfg.Model == "" || cfg.BaseURL == "" {
		return fmt.Errorf("尚未配置模型，请到「系统设置 → 模型接入」填写")
	}

	body := map[string]any{
		"model":       cfg.Model,
		"messages":    messages,
		"temperature": 0.7,
		"stream":      true,
		// OpenAI 兼容接口需要显式要 usage；不认识该字段的服务端会忽略
		"stream_options": map[string]any{"include_usage": true},
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}

	endpoint := strings.TrimRight(cfg.BaseURL, "/") + "/chat/completions"
	reqCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	if key := a.settingOr(ctx, modelAPIKeyKey, ""); key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}

	started := time.Now()
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return fmt.Errorf("连接模型服务失败：%w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		buf := make([]byte, 512)
		n, _ := resp.Body.Read(buf)
		return fmt.Errorf("模型服务返回 HTTP %d：%s", resp.StatusCode, truncate(string(buf[:n]), 200))
	}

	var (
		textOut   strings.Builder
		promptTok int64
		complTok  int64
		estimated bool
	)

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "[DONE]" {
			break
		}

		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
			Usage *struct {
				PromptTokens     int64 `json:"prompt_tokens"`
				CompletionTokens int64 `json:"completion_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			continue
		}
		if chunk.Usage != nil {
			promptTok = chunk.Usage.PromptTokens
			complTok = chunk.Usage.CompletionTokens
		}
		for _, choice := range chunk.Choices {
			if choice.Delta.Content == "" {
				continue
			}
			textOut.WriteString(choice.Delta.Content)
			send(streamChunk{Delta: choice.Delta.Content})
		}
	}
	if err := scanner.Err(); err != nil && textOut.Len() == 0 {
		return fmt.Errorf("读取模型输出失败：%w", err)
	}

	// 服务端没给 usage 时按字符粗估，并明确标注是估算值
	if promptTok == 0 && complTok == 0 {
		estimated = true
		for _, msg := range messages {
			promptTok += int64(len([]rune(msg.Content)))
		}
		promptTok = promptTok / 2
		complTok = int64(len([]rune(textOut.String()))) / 2
	}

	elapsed := time.Since(started).Milliseconds()
	cost := (promptTok*cfg.InputPriceCents + complTok*cfg.OutputPriceCents) / 1_000_000

	result := ChatResult{
		Text: textOut.String(), Model: cfg.Model,
		PromptTokens: promptTok, CompletionTokens: complTok,
		CostCents: cost, ElapsedMs: elapsed,
	}
	_ = a.recordUsage(ctx, result)

	send(streamChunk{
		Done: true, Model: cfg.Model,
		PromptTokens: promptTok, CompletionTokens: complTok,
		CostCents: cost, ElapsedMs: elapsed, Estimated: estimated,
	})

	if strings.TrimSpace(result.Text) == "" {
		return fmt.Errorf("模型没有返回内容")
	}
	return nil
}
