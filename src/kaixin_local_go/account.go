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

// 账号：客户端不直接连服务端，也不在页面里持有 Token。
// 登录成功后会话 Cookie 由 sidecar 保管（存本地库 settings），
// 后续需要鉴权的请求由 sidecar 代发并带上它。这样页面 JS 拿不到 token。

const sessionSettingKey = "server.session"

type AccountState struct {
	LoggedIn    bool   `json:"loggedIn"`
	ID          int64  `json:"id"`
	Account     string `json:"account"`
	DisplayName string `json:"displayName"`
	Role        string `json:"role"`
	Message     string `json:"message"`
}

func (a *App) sessionToken(ctx context.Context) string {
	if a.store == nil {
		return ""
	}
	value, err := a.store.GetSetting(ctx, sessionSettingKey)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(value)
}

func (a *App) saveSessionToken(ctx context.Context, token string) {
	if a.store == nil {
		return
	}
	_ = a.store.SetSetting(ctx, sessionSettingKey, token)
}

// httpJSON 代发一次 JSON 请求，返回状态码与响应体。
func httpJSON(ctx context.Context, method, target, cookie string, payload any) ([]byte, int, string, error) {
	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return nil, 0, "", err
		}
		body = bytes.NewReader(raw)
	}

	reqCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, method, target, body)
	if err != nil {
		return nil, 0, "", err
	}
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if cookie != "" {
		req.Header.Set("Cookie", "kaixin_session="+cookie)
	}

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return nil, 0, "", err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, resp.StatusCode, "", err
	}

	// 取回会话 Cookie（服务端登录/注册时下发）
	newToken := ""
	for _, item := range resp.Header.Values("Set-Cookie") {
		if strings.HasPrefix(item, "kaixin_session=") {
			value := strings.TrimPrefix(item, "kaixin_session=")
			if idx := strings.IndexByte(value, ';'); idx >= 0 {
				value = value[:idx]
			}
			newToken = strings.TrimSpace(value)
		}
	}
	return raw, resp.StatusCode, newToken, nil
}

func parseUserResponse(raw []byte) AccountState {
	var payload struct {
		User struct {
			ID          int64  `json:"id"`
			Account     string `json:"account"`
			DisplayName string `json:"displayName"`
			Role        string `json:"role"`
		} `json:"user"`
		Message string `json:"message"`
	}
	state := AccountState{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return state
	}
	if payload.User.Account != "" {
		state.LoggedIn = true
		state.ID = payload.User.ID
		state.Account = payload.User.Account
		state.DisplayName = payload.User.DisplayName
		state.Role = payload.User.Role
	}
	state.Message = payload.Message
	return state
}

func (a *App) handleAccountLogin(w http.ResponseWriter, r *http.Request) {
	base := a.currentServerURL(r.Context())
	if base == "" {
		fail(w, http.StatusBadRequest, "还没有配置服务器地址，无法登录")
		return
	}
	var request struct {
		Account     string `json:"account"`
		Password    string `json:"password"`
		Register    bool   `json:"register"`
		DisplayName string `json:"displayName"`
		Phone       string `json:"phone"`
	}
	if err := decodeJSON(r, &request); err != nil {
		fail(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	if strings.TrimSpace(request.Account) == "" || request.Password == "" {
		fail(w, http.StatusBadRequest, "请填写账号与密码")
		return
	}

	target := base + "/api/auth/login"
	payload := map[string]any{"account": request.Account, "password": request.Password}
	if request.Register {
		target = base + "/api/auth/register"
		payload["displayName"] = request.DisplayName
		payload["phone"] = request.Phone
	}

	raw, status, token, err := httpJSON(r.Context(), http.MethodPost, target, "", payload)
	if err != nil {
		fail(w, http.StatusBadGateway, "连接服务器失败："+err.Error())
		return
	}
	if status != http.StatusOK {
		var message struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(raw, &message)
		text := message.Message
		if text == "" {
			text = fmt.Sprintf("服务器返回 HTTP %d", status)
		}
		fail(w, status, text)
		return
	}
	if token == "" {
		fail(w, http.StatusBadGateway, "服务器没有下发会话，请检查服务端版本")
		return
	}
	a.saveSessionToken(r.Context(), token)

	state := parseUserResponse(raw)
	state.LoggedIn = true
	writeJSON(w, http.StatusOK, state)
}

func (a *App) handleAccountMe(w http.ResponseWriter, r *http.Request) {
	base := a.currentServerURL(r.Context())
	token := a.sessionToken(r.Context())
	if base == "" || token == "" {
		writeJSON(w, http.StatusOK, AccountState{LoggedIn: false, Message: "未登录"})
		return
	}

	raw, status, newToken, err := httpJSON(r.Context(), http.MethodGet, base+"/api/auth/me", token, nil)
	if err != nil {
		writeJSON(w, http.StatusOK, AccountState{LoggedIn: false, Message: "连接服务器失败：" + err.Error()})
		return
	}
	if newToken != "" {
		a.saveSessionToken(r.Context(), newToken)
	}
	if status == http.StatusUnauthorized {
		a.saveSessionToken(r.Context(), "")
		writeJSON(w, http.StatusOK, AccountState{LoggedIn: false, Message: "登录已失效，请重新登录"})
		return
	}
	if status != http.StatusOK {
		writeJSON(w, http.StatusOK, AccountState{LoggedIn: false, Message: fmt.Sprintf("读取账号失败（HTTP %d）", status)})
		return
	}

	state := parseUserResponse(raw)
	state.Message = "已登录"
	writeJSON(w, http.StatusOK, state)
}

func (a *App) handleAccountLogout(w http.ResponseWriter, r *http.Request) {
	base := a.currentServerURL(r.Context())
	token := a.sessionToken(r.Context())
	if base != "" && token != "" {
		_, _, _, _ = httpJSON(r.Context(), http.MethodPost, base+"/api/auth/logout", token, nil)
	}
	a.saveSessionToken(r.Context(), "")
	writeJSON(w, http.StatusOK, AccountState{LoggedIn: false, Message: "已退出登录"})
}
