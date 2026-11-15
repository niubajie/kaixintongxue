package main

import (
	"net/http"
	"strings"
)

const sessionCookie = "kaixin_session"

type loginRequest struct {
	Account  string `json:"account"`
	Password string `json:"password"`
}

type registerRequest struct {
	Account     string `json:"account"`
	Password    string `json:"password"`
	DisplayName string `json:"displayName"`
	Phone       string `json:"phone"`
}

func (a *App) setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   30 * 24 * 3600,
	})
}

func (a *App) currentUser(r *http.Request) (*User, error) {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil {
		return nil, nil
	}
	return a.userByToken(r.Context(), cookie.Value)
}

// requireAdmin 返回当前管理员；失败时已写好响应体。
func (a *App) requireAdmin(w http.ResponseWriter, r *http.Request) (*User, bool) {
	user, err := a.currentUser(r)
	if err != nil {
		fail(w, http.StatusInternalServerError, "读取登录状态失败。")
		return nil, false
	}
	if user == nil {
		fail(w, http.StatusUnauthorized, "请先登录管理员账号。")
		return nil, false
	}
	if user.Role != "Owner" && user.Role != "Admin" {
		fail(w, http.StatusForbidden, "当前账号没有管理员权限。")
		return nil, false
	}
	return user, true
}

func (a *App) handleMe(w http.ResponseWriter, r *http.Request) {
	user, err := a.currentUser(r)
	if err != nil {
		fail(w, http.StatusInternalServerError, "读取登录状态失败。")
		return
	}
	if user == nil {
		fail(w, http.StatusUnauthorized, "未登录。")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user": map[string]any{
			"id":          user.ID,
			"account":     user.Account,
			"displayName": user.DisplayName,
			"role":        user.Role,
		},
	})
}

func (a *App) handleLogin(w http.ResponseWriter, r *http.Request) {
	var body loginRequest
	if err := decodeJSON(r, &body); err != nil {
		fail(w, http.StatusBadRequest, "请求格式不正确。")
		return
	}
	account := strings.TrimSpace(body.Account)
	if account == "" || body.Password == "" {
		fail(w, http.StatusBadRequest, "请填写账号和密码。")
		return
	}

	ctx := r.Context()
	user, err := a.findUserByAccount(ctx, account)
	if err != nil {
		fail(w, http.StatusInternalServerError, "登录失败，请稍后再试。")
		return
	}
	if user == nil {
		fail(w, http.StatusUnauthorized, "账号或密码不正确。")
		return
	}
	if user.Status != "active" {
		fail(w, http.StatusForbidden, "该账号已被停用，请联系管理员。")
		return
	}
	hash, salt := a.getPassword(ctx, account)
	if !verifyPassword(body.Password, hash, salt) {
		a.audit(ctx, account, "login_failed", "密码不正确")
		fail(w, http.StatusUnauthorized, "账号或密码不正确。")
		return
	}

	token, err := a.createSession(ctx, user.ID, r.UserAgent())
	if err != nil {
		fail(w, http.StatusInternalServerError, "登录失败，请稍后再试。")
		return
	}
	a.touchLogin(ctx, user.ID)
	a.audit(ctx, account, "login", "登录成功")
	a.setSessionCookie(w, token)
	writeJSON(w, http.StatusOK, map[string]any{
		"message": "登录成功。",
		"user": map[string]any{
			"id":          user.ID,
			"account":     user.Account,
			"displayName": user.DisplayName,
			"role":        user.Role,
		},
	})
}

func (a *App) handleRegister(w http.ResponseWriter, r *http.Request) {
	var body registerRequest
	if err := decodeJSON(r, &body); err != nil {
		fail(w, http.StatusBadRequest, "请求格式不正确。")
		return
	}
	account := strings.TrimSpace(body.Account)
	displayName := strings.TrimSpace(body.DisplayName)
	if account == "" || body.Password == "" {
		fail(w, http.StatusBadRequest, "请填写账号和密码。")
		return
	}
	if len([]rune(body.Password)) < 6 {
		fail(w, http.StatusBadRequest, "密码至少 6 位。")
		return
	}
	if displayName == "" {
		displayName = account
	}
	if body.Phone != "" && !validPhone(body.Phone) {
		fail(w, http.StatusBadRequest, "手机号格式不正确。")
		return
	}

	ctx := r.Context()
	existing, err := a.findUserByAccount(ctx, account)
	if err != nil {
		fail(w, http.StatusInternalServerError, "注册失败，请稍后再试。")
		return
	}
	if existing != nil {
		fail(w, http.StatusConflict, "该账号已存在。")
		return
	}
	user, err := a.createUser(ctx, account, body.Password, displayName, strings.TrimSpace(body.Phone))
	if err != nil {
		fail(w, http.StatusInternalServerError, "注册失败，请稍后再试。")
		return
	}

	token, err := a.createSession(ctx, user.ID, r.UserAgent())
	if err != nil {
		fail(w, http.StatusInternalServerError, "注册成功，但登录状态创建失败。")
		return
	}
	a.audit(ctx, account, "register", "新用户注册")
	a.setSessionCookie(w, token)
	writeJSON(w, http.StatusOK, map[string]any{
		"message": "注册成功。",
		"user": map[string]any{
			"id":          user.ID,
			"account":     user.Account,
			"displayName": user.DisplayName,
			"role":        user.Role,
		},
	})
}

func (a *App) handleLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookie); err == nil {
		a.deleteSession(r.Context(), cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true})
	writeMessage(w, http.StatusOK, "已退出登录。")
}
