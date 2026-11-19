package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// 微信注册与登录（FR-CLD-02）。
//
// 真实流程需要微信开放平台「网站应用」的 AppID/AppSecret 与已备案回调域名；
// 没配置时接口明确返回 503，不假装可用。
// 本地联调可设 KAIXIN_WECHAT_MOCK=1，用内置的模拟确认页跑通整条链路。

const wechatStateTTL = 5 * time.Minute

func (a *App) ensureWechatSchema(ctx context.Context) error {
	_, err := a.db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS wechat_auth_requests (
			id BIGSERIAL PRIMARY KEY,
			state TEXT NOT NULL UNIQUE,
			status TEXT NOT NULL DEFAULT 'pending',
			open_id TEXT NOT NULL DEFAULT '',
			nickname TEXT NOT NULL DEFAULT '',
			user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
			created_at TEXT NOT NULL,
			expires_at TEXT NOT NULL
		)`)
	if err != nil {
		return err
	}
	_, err = a.db.Exec(ctx, `CREATE INDEX IF NOT EXISTS idx_wechat_state ON wechat_auth_requests(state)`)
	return err
}

func wechatConfig() (appID, appSecret, redirectBase string, mock bool) {
	return strings.TrimSpace(os.Getenv("KAIXIN_WECHAT_APPID")),
		strings.TrimSpace(os.Getenv("KAIXIN_WECHAT_SECRET")),
		strings.TrimSpace(os.Getenv("KAIXIN_PUBLIC_BASE")),
		os.Getenv("KAIXIN_WECHAT_MOCK") == "1"
}

func randomState() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}

// handleWechatStart 生成一次扫码登录请求，返回前端要打开的地址。
func (a *App) handleWechatStart(w http.ResponseWriter, r *http.Request) {
	appID, secret, base, mock := wechatConfig()
	// 真实模式三个变量缺一不可：缺了 Secret 换不到 openid，
	// 缺了 PUBLIC_BASE 拼出的是相对回调地址，微信会直接拒绝。
	if !mock {
		var missing []string
		if appID == "" {
			missing = append(missing, "KAIXIN_WECHAT_APPID")
		}
		if secret == "" {
			missing = append(missing, "KAIXIN_WECHAT_SECRET")
		}
		if base == "" {
			missing = append(missing, "KAIXIN_PUBLIC_BASE")
		}
		if len(missing) > 0 {
			fail(w, http.StatusServiceUnavailable, "未配置微信登录，缺少 "+strings.Join(missing, " / "))
			return
		}
	}

	state := randomState()
	now := time.Now()
	_, err := a.db.Exec(r.Context(),
		`INSERT INTO wechat_auth_requests(state, status, created_at, expires_at) VALUES($1, 'pending', $2, $3)`,
		state, now.Format(time.RFC3339), now.Add(wechatStateTTL).Format(time.RFC3339))
	if err != nil {
		fail(w, http.StatusInternalServerError, "创建扫码请求失败")
		return
	}

	if mock {
		writeJSON(w, http.StatusOK, map[string]any{
			"state":     state,
			"mode":      "mock",
			"qrUrl":     "/auth/wechat/mock?state=" + url.QueryEscape(state),
			"expiresIn": int(wechatStateTTL.Seconds()),
		})
		return
	}

	redirect := strings.TrimRight(base, "/") + "/api/auth/wechat/callback"
	target := "https://open.weixin.qq.com/connect/qrconnect?" + url.Values{
		"appid":         {appID},
		"redirect_uri":  {redirect},
		"response_type": {"code"},
		"scope":         {"snsapi_login"},
		"state":         {state},
	}.Encode() + "#wechat_redirect"

	writeJSON(w, http.StatusOK, map[string]any{
		"state":     state,
		"mode":      "wechat",
		"qrUrl":     target,
		"expiresIn": int(wechatStateTTL.Seconds()),
	})
}

// handleWechatPoll 前端轮询：等到用户确认后建立会话。
func (a *App) handleWechatPoll(w http.ResponseWriter, r *http.Request) {
	state := strings.TrimSpace(r.URL.Query().Get("state"))
	if state == "" {
		fail(w, http.StatusBadRequest, "缺少 state")
		return
	}

	var status string
	var userID *int64
	var expiresAt string
	err := a.db.QueryRow(r.Context(),
		`SELECT status, user_id, expires_at FROM wechat_auth_requests WHERE state = $1`, state).
		Scan(&status, &userID, &expiresAt)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"status": "unknown"})
		return
	}
	if expires, parseErr := time.Parse(time.RFC3339, expiresAt); parseErr == nil && time.Now().After(expires) && status == "pending" {
		writeJSON(w, http.StatusOK, map[string]any{"status": "expired"})
		return
	}
	if status != "authorized" || userID == nil {
		writeJSON(w, http.StatusOK, map[string]any{"status": status})
		return
	}

	user, err := a.userByID(r.Context(), *userID)
	if err != nil || user == nil {
		fail(w, http.StatusInternalServerError, "读取账号失败")
		return
	}
	if user.Status != "active" {
		fail(w, http.StatusForbidden, "账号已停用")
		return
	}
	token, err := a.createSession(r.Context(), user.ID, r.UserAgent())
	if err != nil {
		fail(w, http.StatusInternalServerError, "建立会话失败")
		return
	}
	a.touchLogin(r.Context(), user.ID)
	a.setSessionCookie(w, token)
	writeJSON(w, http.StatusOK, map[string]any{"status": "authorized", "user": toUserDTO(*user)})
}

// completeWechatLogin 让一次扫码请求变成「已授权」，必要时自动建号。
func (a *App) completeWechatLogin(ctx context.Context, state, openID, nickname string) error {
	if strings.TrimSpace(openID) == "" {
		return fmt.Errorf("微信返回的 openid 为空")
	}
	// 先确认 state 真实存在且未过期，否则会凭伪造的 state 白建一个账号。
	var reqStatus, expiresAt string
	if err := a.db.QueryRow(ctx,
		`SELECT status, expires_at FROM wechat_auth_requests WHERE state = $1`, state).
		Scan(&reqStatus, &expiresAt); err != nil {
		return fmt.Errorf("扫码请求无效，请重新发起")
	}
	if expires, parseErr := time.Parse(time.RFC3339, expiresAt); parseErr == nil && time.Now().After(expires) {
		return fmt.Errorf("二维码已过期，请重新发起")
	}
	var userID int64
	err := a.db.QueryRow(ctx, `SELECT id FROM users WHERE wechat_open_id = $1`, openID).Scan(&userID)
	if err != nil {
		// 首次微信登录：自动注册为 Member，账号名用 wx_ + openid 前 8 位
		suffix := openID
		if len(suffix) > 8 {
			suffix = suffix[:8]
		}
		account := "wx_" + suffix
		display := strings.TrimSpace(nickname)
		if display == "" {
			display = "微信用户"
		}
		var newID int64
		insertErr := a.db.QueryRow(ctx,
			`INSERT INTO users(account, password_hash, password_salt, display_name, wechat_open_id, role, created_at)
			 VALUES($1, '', '', $2, $3, 'Member', $4) RETURNING id`,
			account, display, openID, nowText()).Scan(&newID)
		if insertErr != nil {
			return fmt.Errorf("创建微信账号失败：%w", insertErr)
		}
		userID = newID
		a.audit(ctx, account, "wechat_register", display)
	}

	_, err = a.db.Exec(ctx,
		`UPDATE wechat_auth_requests SET status = 'authorized', open_id = $2, nickname = $3, user_id = $4 WHERE state = $1`,
		state, openID, nickname, userID)
	return err
}

// handleWechatMockConfirm 仅联调模式可用：模拟「用户在微信里点了确认」。
func (a *App) handleWechatMockConfirm(w http.ResponseWriter, r *http.Request) {
	_, _, _, mock := wechatConfig()
	if !mock {
		fail(w, http.StatusForbidden, "未开启联调模式")
		return
	}
	var body struct {
		State    string `json:"state"`
		OpenID   string `json:"openid"`
		Nickname string `json:"nickname"`
	}
	if err := decodeJSON(r, &body); err != nil || body.State == "" {
		fail(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	if body.OpenID == "" {
		body.OpenID = "mock_openid_001"
	}
	if err := a.completeWechatLogin(r.Context(), body.State, body.OpenID, body.Nickname); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"message": "已模拟微信确认"})
}

// handleWechatMockPage 联调用的确认页，浏览器打开即可点「确认登录」。
func (a *App) handleWechatMockPage(w http.ResponseWriter, r *http.Request) {
	_, _, _, mock := wechatConfig()
	if !mock {
		http.NotFound(w, r)
		return
	}
	state := r.URL.Query().Get("state")
	// state 来自查询串，必须转义后再拼进脚本，避免被构造成可执行代码。
	stateJSON, err := json.Marshal(state)
	if err != nil {
		http.Error(w, "参数格式错误", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(`<!doctype html><meta charset="utf-8">
<title>微信登录（联调）</title>
<body style="font-family:system-ui;padding:40px;text-align:center">
<h2>微信扫码登录（联调模式）</h2>
<p style="color:#666">真实环境这里会是微信的二维码页面</p>
<button onclick="confirmLogin()" style="padding:10px 24px;font-size:15px;cursor:pointer">确认登录</button>
<p id="msg" style="color:#0a7;margin-top:16px"></p>
<script>
async function confirmLogin(){
  const r = await fetch('/api/auth/wechat/mock-confirm',{method:'POST',headers:{'Content-Type':'application/json'},
    body:JSON.stringify({state:` + string(stateJSON) + `,openid:'mock_openid_001',nickname:'微信测试用户'})});
  document.getElementById('msg').textContent = r.ok ? '已确认，可以回到应用了' : '确认失败';
}
</script>`))
}

// handleWechatCallback 真实模式：微信回调带 code，换 openid 后完成登录。
func (a *App) handleWechatCallback(w http.ResponseWriter, r *http.Request) {
	appID, secret, _, _ := wechatConfig()
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	state := strings.TrimSpace(r.URL.Query().Get("state"))
	if appID == "" || secret == "" || code == "" || state == "" {
		fail(w, http.StatusBadRequest, "回调参数不完整")
		return
	}

	tokenURL := "https://api.weixin.qq.com/sns/oauth2/access_token?" + url.Values{
		"appid":      {appID},
		"secret":     {secret},
		"code":       {code},
		"grant_type": {"authorization_code"},
	}.Encode()

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(tokenURL)
	if err != nil {
		fail(w, http.StatusBadGateway, "请求微信接口失败")
		return
	}
	defer resp.Body.Close()

	payload, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var token struct {
		AccessToken string `json:"access_token"`
		OpenID      string `json:"openid"`
		UnionID     string `json:"unionid"`
		ErrCode     int    `json:"errcode"`
		ErrMsg      string `json:"errmsg"`
	}
	if err := json.Unmarshal(payload, &token); err != nil || token.OpenID == "" {
		fail(w, http.StatusBadGateway, "微信未返回 openid："+token.ErrMsg)
		return
	}

	nickname := ""
	if token.AccessToken != "" {
		infoURL := "https://api.weixin.qq.com/sns/userinfo?" + url.Values{
			"access_token": {token.AccessToken},
			"openid":       {token.OpenID},
		}.Encode()
		if infoResp, err := client.Get(infoURL); err == nil {
			defer infoResp.Body.Close()
			infoBody, _ := io.ReadAll(io.LimitReader(infoResp.Body, 1<<20))
			var info struct {
				Nickname string `json:"nickname"`
			}
			if json.Unmarshal(infoBody, &info) == nil {
				nickname = info.Nickname
			}
		}
	}

	if err := a.completeWechatLogin(r.Context(), state, token.OpenID, nickname); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(`<!doctype html><meta charset="utf-8"><body style="font-family:system-ui;padding:40px;text-align:center">
<h2>登录成功</h2><p>可以回到应用继续使用。</p></body>`))
}

// handleWechatBind 已登录用户绑定微信；解绑需要密码确认。
func (a *App) handleWechatBind(w http.ResponseWriter, r *http.Request) {
	user, ok := a.requireLogin(w, r, "auth.wechat.bind")
	if !ok {
		return
	}
	var body struct {
		OpenID   string `json:"openid"`
		Unbind   bool   `json:"unbind"`
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &body); err != nil {
		fail(w, http.StatusBadRequest, "请求格式错误")
		return
	}

	if body.Unbind {
		hash, salt := a.getPassword(r.Context(), user.Account)
		if hash == "" || !verifyPassword(body.Password, hash, salt) {
			fail(w, http.StatusUnauthorized, "密码不正确，无法解绑")
			return
		}
		_, _ = a.db.Exec(r.Context(), `UPDATE users SET wechat_open_id = NULL WHERE id = $1`, user.ID)
		a.audit(r.Context(), user.Account, "wechat_unbind", "")
		writeMessage(w, http.StatusOK, "已解绑微信")
		return
	}

	openID := strings.TrimSpace(body.OpenID)
	if openID == "" {
		fail(w, http.StatusBadRequest, "缺少 openid")
		return
	}
	var exists int64
	_ = a.db.QueryRow(r.Context(), `SELECT id FROM users WHERE wechat_open_id = $1`, openID).Scan(&exists)
	if exists != 0 && exists != user.ID {
		fail(w, http.StatusConflict, "该微信已绑定其他账号")
		return
	}
	_, _ = a.db.Exec(r.Context(), `UPDATE users SET wechat_open_id = $2 WHERE id = $1`, user.ID, openID)
	a.audit(r.Context(), user.Account, "wechat_bind", "")
	writeMessage(w, http.StatusOK, "已绑定微信")
}
