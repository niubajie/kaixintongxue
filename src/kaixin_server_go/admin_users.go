package main

import (
	"net/http"
	"strconv"
	"strings"
)

// handleAdminUsers 成员列表：登录即可读，但成员只能看到自己一条（SYS-02）。
func (a *App) handleAdminUsers(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.requireLogin(w, r, "admin.users.list")
	if !ok {
		return
	}
	query := r.URL.Query()
	paging := parseMemberPage(r)

	ctx := r.Context()
	users, total, err := a.listUsers(ctx, strings.TrimSpace(query.Get("keyword")), query.Get("role"),
		paging.Size, (paging.Page-1)*paging.Size)
	if err != nil {
		fail(w, http.StatusInternalServerError, "读取用户失败。")
		return
	}

	visible := visibleUsers(actor, users)
	items := make([]UserDTO, 0, len(visible))
	for _, user := range visible {
		items = append(items, toUserDTO(user))
	}
	// 成员被过滤后，总数按实际可见条数返回，避免泄漏他人数量
	if len(visible) != len(users) {
		total = len(visible)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"total": total, "page": paging.Page, "size": paging.Size, "items": items,
	})
}

func (a *App) handleUserRole(w http.ResponseWriter, r *http.Request) {
	admin, ok := a.requireLogin(w, r, "admin.users.role")
	if !ok {
		return
	}
	id, valid := parseInt64(r.PathValue("id"))
	if !valid {
		fail(w, http.StatusBadRequest, "用户不存在。")
		return
	}
	var body struct {
		Role string `json:"role"`
	}
	if err := decodeJSON(r, &body); err != nil {
		fail(w, http.StatusBadRequest, "请求格式不正确。")
		return
	}
	role := strings.TrimSpace(body.Role)
	if role != "Owner" && role != "Admin" && role != "Member" {
		fail(w, http.StatusBadRequest, "角色只能是 Owner / Admin / Member。")
		return
	}

	ctx := r.Context()
	target, err := a.findUserByID(ctx, id)
	if err != nil || target == nil {
		fail(w, http.StatusNotFound, "用户不存在。")
		return
	}
	if allowed, reason := canManageUser(admin, target); !allowed {
		a.denyWithAudit(w, r, admin, "admin.users.role", target.Account, reason)
		return
	}
	if allowed, reason := canAssignRole(admin, role); !allowed {
		a.denyWithAudit(w, r, admin, "admin.users.role", target.Account, reason)
		return
	}
	if target.Role == "Owner" && role != "Owner" && a.countOwners(ctx) <= 1 {
		fail(w, http.StatusBadRequest, "至少需要保留一个 Owner。")
		return
	}
	if err := a.setUserRole(ctx, id, role); err != nil {
		fail(w, http.StatusInternalServerError, "调整角色失败。")
		return
	}
	a.audit(ctx, admin.Account, "user_role", target.Account+" -> "+role)
	writeMessage(w, http.StatusOK, "已把 "+target.Account+" 调整为 "+role+"。")
}

func (a *App) handleUserStatus(w http.ResponseWriter, r *http.Request) {
	admin, ok := a.requireLogin(w, r, "admin.users.status")
	if !ok {
		return
	}
	id, valid := parseInt64(r.PathValue("id"))
	if !valid {
		fail(w, http.StatusBadRequest, "用户不存在。")
		return
	}
	var body struct {
		Status string `json:"status"`
	}
	if err := decodeJSON(r, &body); err != nil {
		fail(w, http.StatusBadRequest, "请求格式不正确。")
		return
	}
	status := strings.TrimSpace(body.Status)
	if status != "active" && status != "disabled" {
		fail(w, http.StatusBadRequest, "状态不合法。")
		return
	}

	ctx := r.Context()
	target, err := a.findUserByID(ctx, id)
	if err != nil || target == nil {
		fail(w, http.StatusNotFound, "用户不存在。")
		return
	}
	if allowed, reason := canManageUser(admin, target); !allowed {
		a.denyWithAudit(w, r, admin, "admin.users.status", target.Account, reason)
		return
	}
	if err := a.setUserStatus(ctx, id, status); err != nil {
		fail(w, http.StatusInternalServerError, "操作失败。")
		return
	}
	action := "user_enable"
	message := "已启用 " + target.Account + "。"
	if status == "disabled" {
		action = "user_disable"
		message = "已停用 " + target.Account + "，该账号已退出登录。"
	}
	a.audit(ctx, admin.Account, action, target.Account)
	writeMessage(w, http.StatusOK, message)
}

func (a *App) handleUserDelete(w http.ResponseWriter, r *http.Request) {
	admin, ok := a.requireLogin(w, r, "admin.users.delete")
	if !ok {
		return
	}
	id, valid := parseInt64(r.PathValue("id"))
	if !valid {
		fail(w, http.StatusBadRequest, "用户不存在。")
		return
	}

	ctx := r.Context()
	target, err := a.findUserByID(ctx, id)
	if err != nil || target == nil {
		fail(w, http.StatusNotFound, "用户不存在。")
		return
	}
	if allowed, reason := canManageUser(admin, target); !allowed {
		a.denyWithAudit(w, r, admin, "admin.users.delete", target.Account, reason)
		return
	}
	if target.Role == "Owner" && a.countOwners(ctx) <= 1 {
		fail(w, http.StatusBadRequest, "至少需要保留一个 Owner。")
		return
	}
	if err := a.deleteUser(ctx, id); err != nil {
		fail(w, http.StatusInternalServerError, "删除失败。")
		return
	}
	a.audit(ctx, admin.Account, "user_delete", target.Account)
	writeMessage(w, http.StatusOK, "已删除 "+target.Account+"。")
}

func (a *App) handleAudit(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.requireLogin(w, r, "admin.audit")
	if !ok {
		return
	}
	if !canReadAudit(actor) {
		a.denyWithAudit(w, r, actor, "admin.audit", "", reasonNotAdmin)
		return
	}
	limit := 100
	if parsed, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && parsed > 0 && parsed <= 500 {
		limit = parsed
	}
	rows, err := a.db.Query(r.Context(),
		"SELECT coalesce(actor, ''), action, detail, created_at FROM audit ORDER BY id DESC LIMIT $1", limit)
	if err != nil {
		fail(w, http.StatusInternalServerError, "读取日志失败。")
		return
	}
	defer rows.Close()

	type entry struct {
		Actor     string `json:"actor"`
		Action    string `json:"action"`
		Detail    string `json:"detail"`
		CreatedAt string `json:"createdAt"`
	}
	items := []entry{}
	for rows.Next() {
		var item entry
		if err := rows.Scan(&item.Actor, &item.Action, &item.Detail, &item.CreatedAt); err != nil {
			fail(w, http.StatusInternalServerError, "读取日志失败。")
			return
		}
		items = append(items, item)
	}
	writeJSON(w, http.StatusOK, items)
}
