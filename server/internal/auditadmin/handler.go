// Package auditadmin 暴露审计日志的后台 HTTP 接口。
// 单独成包是为了避免 audit ← admin 的循环依赖（admin 写审计需要 audit）。
package auditadmin

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"gocircle/server/internal/admin"
	"gocircle/server/internal/audit"
	"gocircle/server/internal/auth"
	"gocircle/server/internal/httpapi"
)

type Handler struct {
	svc *audit.Service
}

func NewHandler(svc *audit.Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) Register(r *gin.Engine, adminMW *admin.Middleware) {
	adm := r.Group("/api/v1/admin", adminMW.Require(auth.RoleAuditor))
	{
		adm.GET("/audit-logs", h.List)
		adm.GET("/audit-logs/verify", h.Verify)
	}
}

// List GET /api/v1/admin/audit-logs
func (h *Handler) List(c *gin.Context) {
	actorType := c.Query("actorType")
	switch actorType {
	case "", "user", "admin", "system":
	default:
		httpapi.Fail(c, httpapi.NewDomainError(httpapi.CodeInvalidArg, "actorType 取值非法", http.StatusBadRequest))
		return
	}
	items, total, err := h.svc.ListLogs(c.Request.Context(), audit.LogFilter{
		ActorType: actorType,
		Action:    c.Query("action"),
		Keyword:   c.Query("keyword"),
		Page:      queryInt(c, "page", 1),
		Size:      queryInt(c, "size", 20),
	})
	if err != nil {
		httpapi.Fail(c, err)
		return
	}
	httpapi.OK(c, gin.H{"items": items, "total": total})
}

// Verify GET /api/v1/admin/audit-logs/verify
func (h *Handler) Verify(c *gin.Context) {
	res, err := h.svc.VerifyChain(c.Request.Context())
	if err != nil {
		httpapi.Fail(c, err)
		return
	}
	httpapi.OK(c, res)
}

func queryInt(c *gin.Context, key string, def int) int {
	v := c.Query(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}
