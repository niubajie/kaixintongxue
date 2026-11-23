package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/coder/websocket"
)

const (
	duplexDialTimeout  = 15 * time.Second
	duplexWriteTimeout = 15 * time.Second
	duplexMaxFrameSize = 4 << 20
)

type duplexCopyResult struct {
	fromUpstream bool
	err          error
}

// serveDuplex 只负责接入与生命周期；MAP 密钥和上游地址不会暴露给页面。
func (a *App) serveDuplex(w http.ResponseWriter, r *http.Request, upstreamURL, apiKey string) {
	// net/http 的整体读写超时不适用于长连接，升级前清除已有 deadline。
	controller := http.NewResponseController(w)
	_ = controller.SetReadDeadline(time.Time{})
	_ = controller.SetWriteDeadline(time.Time{})

	dialCtx, cancelDial := context.WithTimeout(r.Context(), duplexDialTimeout)
	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+apiKey)
	upstream, _, err := websocket.Dial(dialCtx, upstreamURL, &websocket.DialOptions{HTTPHeader: headers})
	cancelDial()
	if err != nil {
		fail(w, http.StatusBadGateway, "无法连接 MiniCPM-o 实时语音服务")
		return
	}
	defer upstream.CloseNow()

	client, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{"localhost", "127.0.0.1", "::1", "tauri.localhost"},
	})
	if err != nil {
		return
	}
	defer client.CloseNow()
	client.SetReadLimit(duplexMaxFrameSize)
	upstream.SetReadLimit(duplexMaxFrameSize)

	started := time.Now()
	ctx, cancel := context.WithCancel(r.Context())
	results := make(chan duplexCopyResult, 2)
	go func() {
		results <- duplexCopyResult{fromUpstream: false, err: copyDuplexMessages(ctx, client, upstream)}
	}()
	go func() {
		results <- duplexCopyResult{fromUpstream: true, err: copyDuplexMessages(ctx, upstream, client)}
	}()

	first := <-results
	if first.fromUpstream && !normalDuplexClose(first.err) {
		writeCtx, cancelWrite := context.WithTimeout(context.WithoutCancel(r.Context()), 2*time.Second)
		_ = client.Write(writeCtx, websocket.MessageText, []byte(`{"type":"error","error":{"code":"upstream_closed","message":"实时语音服务连接已中断","type":"server_error"}}`))
		cancelWrite()
	}
	cancel()
	_ = client.CloseNow()
	_ = upstream.CloseNow()
	<-results

	// 会话 context 已取消，保留其值并单独限制用量记录的写入时间。
	usageCtx, cancelUsage := context.WithTimeout(context.WithoutCancel(r.Context()), 5*time.Second)
	_ = a.recordUsage(usageCtx, ChatResult{
		Model:     duplexModel,
		ElapsedMs: time.Since(started).Milliseconds(),
	})
	cancelUsage()
}

func copyDuplexMessages(ctx context.Context, source, target *websocket.Conn) error {
	for {
		messageType, payload, err := source.Read(ctx)
		if err != nil {
			return err
		}
		if messageType != websocket.MessageText || !json.Valid(payload) {
			return fmt.Errorf("实时语音协议只接受 JSON 文本帧")
		}
		writeCtx, cancel := context.WithTimeout(ctx, duplexWriteTimeout)
		err = target.Write(writeCtx, messageType, payload)
		cancel()
		if err != nil {
			return err
		}
	}
}

func normalDuplexClose(err error) bool {
	status := websocket.CloseStatus(err)
	return status == websocket.StatusNormalClosure || status == websocket.StatusGoingAway
}
