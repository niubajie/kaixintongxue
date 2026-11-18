package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// 上下文管理：
//   1. 滑动窗口兜底（后端强制，防止页面把整段历史全量发过来）
//   2. 主动压缩（前端在对话变长时调用 /api/chat/summarize，把早期对话换成摘要）
//
// 这样长对话不会越聊越慢、越聊越贵。

const (
	// 单次请求最多带多少字符的上下文（约 2000~3000 token）
	maxContextChars = 6000
	// 无论长短，至少保留最近多少轮消息（user + assistant 各算一条）
	minKeepMessages = 6
)

// trimMessages 从后往前保留消息，直到超出字符预算；
// 被丢掉的部分用一条 system 消息说明，避免模型莫名其妙。
func trimMessages(messages []ChatMessage) []ChatMessage {
	if len(messages) == 0 {
		return messages
	}

	// 已经有摘要时，摘要那条 system 永远保留
	var system []ChatMessage
	var rest []ChatMessage
	for _, msg := range messages {
		if msg.Role == "system" {
			system = append(system, msg)
			continue
		}
		rest = append(rest, msg)
	}

	used := 0
	for _, msg := range system {
		used += len([]rune(msg.Content))
	}

	kept := make([]ChatMessage, 0, len(rest))
	for i := len(rest) - 1; i >= 0; i-- {
		size := len([]rune(rest[i].Content))
		if len(kept) >= minKeepMessages && used+size > maxContextChars {
			break
		}
		kept = append(kept, rest[i])
		used += size
	}

	// 反转回正常顺序
	for left, right := 0, len(kept)-1; left < right; left, right = left+1, right-1 {
		kept[left], kept[right] = kept[right], kept[left]
	}

	dropped := len(rest) - len(kept)
	// 只丢一两条就插一条说明，反而更占上下文，不划算
	if dropped < 2 {
		return messages
	}

	notice := ChatMessage{
		Role:    "system",
		Content: fmt.Sprintf("（更早的 %d 条对话已省略，请基于最近的内容回答）", dropped),
	}
	out := make([]ChatMessage, 0, len(system)+1+len(kept))
	out = append(out, system...)
	out = append(out, notice)
	out = append(out, kept...)
	return out
}

const summarizePrompt = `下面是用户与助手的早期对话。请用不超过 200 字的中文，概括：用户的目标或诉求、已确认的关键信息、还没解决的问题。` +
	`只输出摘要正文，不要加标题或解释。`

// handleSummarize 把一段对话压成摘要，供前端替换早期消息。
func (a *App) handleSummarize(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Messages []ChatMessage `json:"messages"`
	}
	if err := decodeJSON(r, &body); err != nil {
		fail(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	cleaned := cleanMessages(body.Messages)
	if len(cleaned) == 0 {
		fail(w, http.StatusBadRequest, "没有可压缩的对话")
		return
	}

	var builder strings.Builder
	for _, msg := range cleaned {
		role := "用户"
		if msg.Role == "assistant" {
			role = "助手"
		}
		builder.WriteString(role + "：" + msg.Content + "\n")
	}
	text := builder.String()
	if len([]rune(text)) > 8000 {
		runes := []rune(text)
		text = string(runes[len(runes)-8000:])
	}

	ctx, cancel := context.WithTimeout(r.Context(), 180*time.Second)
	defer cancel()

	result, err := a.chatOnce(ctx, []ChatMessage{
		{Role: "user", Content: summarizePrompt + "\n\n" + text},
	})
	if err != nil {
		fail(w, http.StatusBadGateway, err.Error())
		return
	}
	summary := strings.TrimSpace(result.Text)
	if summary == "" {
		fail(w, http.StatusBadGateway, "模型没有返回摘要")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"summary":   summary,
		"model":     result.Model,
		"elapsedMs": result.ElapsedMs,
	})
}
