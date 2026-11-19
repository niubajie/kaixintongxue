package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// 微信扫码登录的客户端代理（桌面端）。
// 页面不直接连服务端，也不接触会话 Cookie：
//   1. 页面调 /api/account/wechat/start 拿二维码地址
//   2. 用户扫码确认
//   3. 页面轮询 /api/account/wechat/poll，sidecar 代发请求并把会话 Cookie 存本地

// 复用 servercfg.go 里同一个设置键，避免两处各存一份服务器地址。
func (a *App) serverBase(ctx context.Context) string {
	return strings.TrimRight(a.settingOr(ctx, serverURLKey, ""), "/")
}

func (a *App) handleWechatStartProxy(w http.ResponseWriter, r *http.Request) {
	base := a.serverBase(r.Context())
	if base == "" {
		fail(w, http.StatusServiceUnavailable, "尚未配置服务器地址，请先到「系统设置 → 服务器」填写")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/auth/wechat/start", nil)
	if err != nil {
		fail(w, http.StatusInternalServerError, "构造请求失败")
		return
	}
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		fail(w, http.StatusBadGateway, "连接服务器失败："+err.Error())
		return
	}
	defer resp.Body.Close()

	var payload struct {
		State     string `json:"state"`
		Mode      string `json:"mode"`
		QRURL     string `json:"qrUrl"`
		ExpiresIn int    `json:"expiresIn"`
		Message   string `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		fail(w, http.StatusBadGateway, "服务器返回格式异常")
		return
	}
	if resp.StatusCode != http.StatusOK {
		message := payload.Message
		if message == "" {
			message = fmt.Sprintf("服务器返回 HTTP %d", resp.StatusCode)
		}
		fail(w, resp.StatusCode, message)
		return
	}

	qr := payload.QRURL
	if strings.HasPrefix(qr, "/") {
		qr = base + qr
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"state":     payload.State,
		"mode":      payload.Mode,
		"qrUrl":     qr,
		"expiresIn": payload.ExpiresIn,
	})
}

// handleWechatPollProxy 轮询状态；一旦授权就把会话 Cookie 收进本地。
func (a *App) handleWechatPollProxy(w http.ResponseWriter, r *http.Request) {
	base := a.serverBase(r.Context())
	if base == "" {
		fail(w, http.StatusServiceUnavailable, "尚未配置服务器地址")
		return
	}
	state := strings.TrimSpace(r.URL.Query().Get("state"))
	if state == "" {
		fail(w, http.StatusBadRequest, "缺少 state")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		base+"/api/auth/wechat/poll?state="+state, nil)
	if err != nil {
		fail(w, http.StatusInternalServerError, "构造请求失败")
		return
	}
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		fail(w, http.StatusBadGateway, "连接服务器失败")
		return
	}
	defer resp.Body.Close()

	var payload struct {
		Status  string     `json:"status"`
		User    *UserBrief `json:"user"`
		Message string     `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		fail(w, http.StatusBadGateway, "服务器返回格式异常")
		return
	}
	// 服务端的业务错误（账号停用、参数不合法等）要原样透出，
	// 否则前端只会看到空状态，一直轮询到超时。
	if resp.StatusCode != http.StatusOK {
		message := payload.Message
		if message == "" {
			message = fmt.Sprintf("服务器返回 HTTP %d", resp.StatusCode)
		}
		fail(w, resp.StatusCode, message)
		return
	}

	for _, cookie := range resp.Cookies() {
		if cookie.Name == "kaixin_session" && cookie.Value != "" {
			a.saveSessionToken(ctx, cookie.Value)
		}
	}

	out := map[string]any{"status": payload.Status}
	if payload.User != nil {
		out["user"] = payload.User
		out["message"] = "登录成功"
	}
	if payload.Message != "" && payload.User == nil {
		out["message"] = payload.Message
	}
	writeJSON(w, http.StatusOK, out)
}

// UserBrief 微信登录返回的用户信息
type UserBrief struct {
	ID          int64  `json:"id"`
	Account     string `json:"account"`
	DisplayName string `json:"displayName"`
	Role        string `json:"role"`
}
