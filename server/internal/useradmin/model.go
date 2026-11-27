// Package useradmin 提供后台用户管理：列表查询与封禁/解封。
//
// 分层：handler → service → repository。
// 封禁只改 users.status（active/blocked），不改微信账号与资源归属。
package useradmin

import "time"

// User 后台用户列表项（不含手机号明文）。
type User struct {
	UUID          string     `json:"uuid"`
	Nickname      string     `json:"nickname"`
	Status        string     `json:"status"`
	RiskLevel     string     `json:"riskLevel"`
	PhoneBound    bool       `json:"phoneBound"`
	ResourceCount int        `json:"resourceCount"`
	LastLoginAt   *time.Time `json:"lastLoginAt"`
	CreatedAt     time.Time  `json:"createdAt"`
}

// ListFilter 用户查询条件。
type ListFilter struct {
	Keyword   string
	Status    string
	RiskLevel string
	Page      int
	Size      int
}

// UpdateStatusRequest 封禁/解封请求。
type UpdateStatusRequest struct {
	Status string `json:"status" binding:"required"`
}
