package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestDuplexStatus(t *testing.T) {
	t.Setenv("MAP_API_KEY", "")
	app := &App{}
	req := httptest.NewRequest(http.MethodGet, "/api/duplex/status", nil)
	resp := httptest.NewRecorder()
	app.handleDuplexStatus(resp, req)
	var status DuplexStatus
	if err := json.Unmarshal(resp.Body.Bytes(), &status); err != nil {
		t.Fatalf("状态响应不是 JSON: %v", err)
	}
	if status.Configured || status.Model != duplexModel {
		t.Fatalf("未配置时状态错误: %+v", status)
	}

	t.Setenv("MAP_API_KEY", "test-only-key")
	resp = httptest.NewRecorder()
	app.handleDuplexStatus(resp, req)
	if err := json.Unmarshal(resp.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if !status.Configured {
		t.Fatal("配置 MAP_API_KEY 后应返回 configured=true")
	}
}

func TestDuplexRejectsInvalidConfiguration(t *testing.T) {
	app := &App{}
	t.Setenv("MAP_API_KEY", "test-only-key")
	req := httptest.NewRequest(http.MethodGet, "/api/duplex?mode=video", nil)
	resp := httptest.NewRecorder()
	app.handleDuplex(resp, req)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("错误 mode 应返回 400，实际 %d", resp.Code)
	}

	t.Setenv("MAP_API_KEY", "")
	req = httptest.NewRequest(http.MethodGet, "/api/duplex?mode=audio", nil)
	resp = httptest.NewRecorder()
	app.handleDuplex(resp, req)
	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("缺少密钥应返回 503，实际 %d", resp.Code)
	}
}

func TestDuplexRelaysProtocolMessages(t *testing.T) {
	upstreamErr := make(chan error, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamErr <- func() error {
			if r.Header.Get("Authorization") != "Bearer test-only-key" {
				return fmt.Errorf("上游未收到正确鉴权头")
			}
			conn, err := websocket.Accept(w, r, nil)
			if err != nil {
				return err
			}
			defer conn.CloseNow()
			ctx := r.Context()
			if err := conn.Write(ctx, websocket.MessageText, []byte(`{"type":"session.queue_done"}`)); err != nil {
				return err
			}
			_, initMessage, err := conn.Read(ctx)
			if err != nil {
				return err
			}
			if !strings.Contains(string(initMessage), `"session.init"`) {
				return fmt.Errorf("缺少 session.init")
			}
			if err := conn.Write(ctx, websocket.MessageText, []byte(`{"type":"session.created"}`)); err != nil {
				return err
			}
			_, appendMessage, err := conn.Read(ctx)
			if err != nil {
				return err
			}
			if !strings.Contains(string(appendMessage), `"input.append"`) || !strings.Contains(string(appendMessage), `"audio"`) {
				return fmt.Errorf("缺少音频输入")
			}
			if err := conn.Write(ctx, websocket.MessageText, []byte(`{"type":"response.output.delta","kind":"audio","audio":"AAAA"}`)); err != nil {
				return err
			}
			if _, _, err := conn.Read(ctx); err != nil {
				return err
			}
			if err := conn.Write(ctx, websocket.MessageText, []byte(`{"type":"session.closed","reason":"user_stop"}`)); err != nil {
				return err
			}
			return conn.Close(websocket.StatusNormalClosure, "done")
		}()
	}))
	defer upstream.Close()

	app := &App{}
	sessionDone := make(chan struct{})
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		app.serveDuplex(w, r, wsURL(upstream.URL), "test-only-key")
		close(sessionDone)
	}))
	defer local.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, _, err := websocket.Dial(ctx, wsURL(local.URL), nil)
	if err != nil {
		t.Fatalf("连接本地语音服务失败: %v", err)
	}
	defer client.CloseNow()
	readDuplexType(t, ctx, client, "session.queue_done")
	if err := client.Write(ctx, websocket.MessageText, []byte(`{"type":"session.init","payload":{}}`)); err != nil {
		t.Fatal(err)
	}
	readDuplexType(t, ctx, client, "session.created")
	if err := client.Write(ctx, websocket.MessageText, []byte(`{"type":"input.append","input":{"audio":"AAAA"}}`)); err != nil {
		t.Fatal(err)
	}
	readDuplexType(t, ctx, client, "response.output.delta")
	if err := client.Write(ctx, websocket.MessageText, []byte(`{"type":"session.close","reason":"user_stop"}`)); err != nil {
		t.Fatal(err)
	}
	readDuplexType(t, ctx, client, "session.closed")

	select {
	case err := <-upstreamErr:
		if err != nil && websocket.CloseStatus(err) != websocket.StatusNormalClosure {
			t.Fatalf("假上游会话失败: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("等待假上游退出超时")
	}
	select {
	case <-sessionDone:
	case <-ctx.Done():
		t.Fatal("本地语音 handler 未退出")
	}
}

func TestDuplexRejectsNonJSONAndCleansUp(t *testing.T) {
	upstreamDone := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(upstreamDone)
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()
		_, _, _ = conn.Read(r.Context())
	}))
	defer upstream.Close()

	app := &App{}
	sessionDone := make(chan struct{})
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		app.serveDuplex(w, r, wsURL(upstream.URL), "test-only-key")
		close(sessionDone)
	}))
	defer local.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client, _, err := websocket.Dial(ctx, wsURL(local.URL), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.CloseNow()
	if err := client.Write(ctx, websocket.MessageText, []byte("not-json")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := client.Read(ctx); err == nil {
		t.Fatal("非法 JSON 后应关闭客户端连接")
	}
	select {
	case <-sessionDone:
	case <-ctx.Done():
		t.Fatal("非法消息后本地 handler 未退出")
	}
	select {
	case <-upstreamDone:
	case <-ctx.Done():
		t.Fatal("非法消息后上游连接未关闭")
	}
}

func TestDuplexWebSocketRequiresLANToken(t *testing.T) {
	cfg := listenConfig{lan: true, tokenHash: sha256.Sum256([]byte("test-token"))}
	local := httptest.NewServer(cfg.middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = websocket.Accept(w, r, nil)
	})))
	defer local.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, resp, err := websocket.Dial(ctx, wsURL(local.URL), nil)
	if conn != nil {
		_ = conn.CloseNow()
		t.Fatal("缺少令牌时不应建立 WebSocket")
	}
	if err == nil || resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("缺少令牌应返回 401: resp=%v err=%v", resp, err)
	}
}

func wsURL(httpURL string) string {
	return "ws" + strings.TrimPrefix(httpURL, "http")
}

func readDuplexType(t *testing.T, ctx context.Context, conn *websocket.Conn, want string) {
	t.Helper()
	_, payload, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("读取 %s 失败: %v", want, err)
	}
	var event struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(payload, &event); err != nil {
		t.Fatalf("事件不是 JSON: %v", err)
	}
	if event.Type != want {
		t.Fatalf("事件类型 = %s，期望 %s", event.Type, want)
	}
}
