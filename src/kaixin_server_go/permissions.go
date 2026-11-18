package main

import (
	"context"
	"net/http"
	"strings"
)

// 权限矩阵（SYS-02 定稿版）
//
// 服务端只强制它真正能看到的资源：成员与权限（R4）、软件目录（R5）、备份与恢复（R6）。
// 本机数据（会话/目标/资料）靠 user_id 隔离，不在这里判断。
//
// 三条硬规则：
//   1. Admin 只能管 Member，不能管 Admin 与 Owner —— 防两个管理员互踢失联
//   2. 恢复与下载备份只给 Owner —— 能把整库回滚或带离服务器
//   3. 任何角色都不能删自己 / 改自己 —— 避免把唯一 Owner 弄没

// 拒绝原因，写进审计，便于事后追。
const (
	reasonNotLoggedIn   = "not_logged_in"
	reasonNotAdmin      = "not_admin"
	reasonAdminOnAdmin  = "admin_cannot_manage_admin"
	reasonTargetIsOwner = "cannot_manage_owner"
	reasonSelfTarget    = "cannot_target_self"
	reasonOwnerOnly     = "owner_only"
)

// canManageUser 判断 actor 能否对 target 执行管理动作（改角色/启停/删除）。
func canManageUser(actor, target *User) (bool, string) {
	if actor == nil {
		return false, reasonNotLoggedIn
	}
	if target == nil {
		return false, "target_not_found"
	}
	if actor.ID == target.ID {
		return false, reasonSelfTarget
	}
	switch actor.Role {
	case "Owner":
		// Owner 可以管任何人，包括其他 Owner（但数量保护由调用方判断）
		return true, ""
	case "Admin":
		if target.Role == "Owner" {
			return false, reasonTargetIsOwner
		}
		if target.Role == "Admin" {
			return false, reasonAdminOnAdmin
		}
		return true, ""
	default:
		return false, reasonNotAdmin
	}
}

// canAssignRole 判断 actor 能否把某人设成目标角色。
// 只有 Owner 能指派 Owner 与 Admin。
func canAssignRole(actor *User, role string) (bool, string) {
	if actor == nil {
		return false, reasonNotLoggedIn
	}
	if role == "Owner" || role == "Admin" {
		if actor.Role != "Owner" {
			return false, "only_owner_can_assign_admin"
		}
	}
	return true, ""
}

// canReadAudit 审计日志只给 Owner 与 Admin。
func canReadAudit(actor *User) bool {
	return actor != nil && (actor.Role == "Owner" || actor.Role == "Admin")
}

// canTriggerBackup 备份：Owner 与 Admin 都可以触发与查看。
func canTriggerBackup(actor *User) bool {
	return actor != nil && (actor.Role == "Owner" || actor.Role == "Admin")
}

// canRestoreBackup 恢复与下载只给 Owner。
func canRestoreBackup(actor *User) bool {
	return actor != nil && actor.Role == "Owner"
}

// canEditCatalog 软件目录：Owner 与 Admin 可写，Member 只读。
func canEditCatalog(actor *User) bool {
	return actor != nil && (actor.Role == "Owner" || actor.Role == "Admin")
}

// denyWithAudit 统一拒绝：返回 403、带错误码，并写一条拒绝审计。
// 前端按 code 提示，不靠文案匹配。
func (a *App) denyWithAudit(w http.ResponseWriter, r *http.Request, actor *User, action, target, reason string) {
	account := "anonymous"
	if actor != nil {
		account = actor.Account
	}
	detail := action + " | target=" + target + " | reason=" + reason
	a.audit(r.Context(), account, "deny", detail)

	writeJSON(w, http.StatusForbidden, map[string]any{
		"code":    10004,
		"message": "没有权限执行该操作。",
		"reason":  reason,
	})
}

// requireLogin 拿到当前用户；未登录时按统一格式拒绝。
func (a *App) requireLogin(w http.ResponseWriter, r *http.Request, action string) (*User, bool) {
	user, err := a.currentUser(r)
	if err != nil {
		fail(w, http.StatusInternalServerError, "读取登录状态失败。")
		return nil, false
	}
	if user == nil {
		a.denyWithAudit(w, r, nil, action, "", reasonNotLoggedIn)
		return nil, false
	}
	return user, true
}

// requireCatalogEditor 软件目录的写权限：Owner 与 Admin；拒绝时记审计。
func (a *App) requireCatalogEditor(w http.ResponseWriter, r *http.Request, action string) (*User, bool) {
	actor, ok := a.requireLogin(w, r, action)
	if !ok {
		return nil, false
	}
	if !canEditCatalog(actor) {
		a.denyWithAudit(w, r, actor, action, "software", reasonNotAdmin)
		return nil, false
	}
	return actor, true
}

// memberPage 成员列表的分页参数。
type memberPage struct {
	Page int
	Size int
}

func parseMemberPage(r *http.Request) memberPage {
	page, size := 1, 20
	if parsed, ok := parseInt64(r.URL.Query().Get("page")); ok && parsed > 0 {
		page = int(parsed)
	}
	if parsed, ok := parseInt64(r.URL.Query().Get("size")); ok && parsed > 0 && parsed <= 100 {
		size = int(parsed)
	}
	return memberPage{Page: page, Size: size}
}

// visibleUsers 按角色过滤可见成员：Owner/Admin 看全部，Member 只看自己。
func visibleUsers(actor *User, users []User) []User {
	if actor == nil {
		return nil
	}
	if actor.Role == "Owner" || actor.Role == "Admin" {
		return users
	}
	out := make([]User, 0, 1)
	for _, user := range users {
		if user.ID == actor.ID {
			out = append(out, user)
		}
	}
	return out
}

// auditActionName 把 HTTP 动作转成审计里的短名。
func auditActionName(method, path string) string {
	path = strings.TrimPrefix(path, "/api/")
	path = strings.ReplaceAll(path, "/", ".")
	switch method {
	case http.MethodPost:
		return "post." + path
	case http.MethodPut:
		return "put." + path
	case http.MethodDelete:
		return "delete." + path
	default:
		return "get." + path
	}
}

var _ = context.Background
