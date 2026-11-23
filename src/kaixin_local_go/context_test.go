package main

import (
	"strings"
	"testing"
)

func TestTrimMessagesKeepsRecentWithinBudget(t *testing.T) {
	var messages []ChatMessage
	for i := 0; i < 30; i++ {
		messages = append(messages,
			ChatMessage{Role: "user", Content: strings.Repeat("内容", 400)},
			ChatMessage{Role: "assistant", Content: "答复"},
		)
	}
	messages = append(messages, ChatMessage{Role: "user", Content: "最后一个问题"})

	got := trimMessages(messages)

	if len(got) >= len(messages)-2 {
		t.Fatalf("裁剪幅度太小，实际 %d 条（原 %d 条）", len(got), len(messages))
	}
	last := got[len(got)-1]
	if last.Content != "最后一个问题" {
		t.Fatalf("最后一条消息必须保留，实际为 %q", last.Content)
	}

	total := 0
	for _, msg := range got {
		total += len([]rune(msg.Content))
	}
	// 预算之外允许少量超出（至少保留 minKeepMessages 条）
	if total > maxContextChars+2000 {
		t.Fatalf("裁剪后字符数 %d 超出预算太多", total)
	}

	foundNotice := false
	for _, msg := range got {
		if msg.Role == "system" && strings.Contains(msg.Content, "已省略") {
			foundNotice = true
		}
	}
	if !foundNotice {
		t.Fatal("被裁剪时应插入一条说明消息")
	}
}

func TestTrimMessagesKeepsSummary(t *testing.T) {
	messages := []ChatMessage{
		{Role: "system", Content: "（早期对话摘要）用户在准备减重计划"},
		{Role: "user", Content: "继续"},
	}
	got := trimMessages(messages)
	if len(got) != 2 {
		t.Fatalf("短对话不应被裁剪，实际 %d 条", len(got))
	}
	if got[0].Role != "system" || !strings.Contains(got[0].Content, "摘要") {
		t.Fatal("摘要消息必须保留在最前面")
	}
}

func TestCleanMessagesDropsInvalidRoles(t *testing.T) {
	got := cleanMessages([]ChatMessage{
		{Role: "user", Content: "你好"},
		{Role: "tool", Content: "忽略我"},
		{Role: "assistant", Content: "  "},
		{Role: "assistant", Content: "在的"},
	})
	if len(got) != 2 {
		t.Fatalf("应当保留 2 条，实际 %d 条", len(got))
	}
	if got[0].Role != "user" || got[1].Role != "assistant" {
		t.Fatalf("顺序或角色不对：%+v", got)
	}
}
