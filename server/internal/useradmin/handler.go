package useradmin

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"gocircle/server/internal/admin"
	"gocircle/server/internal/auth"
	"gocircle/server/internal/httpapi"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) Register(r *gin.Engine, adminMW *admin.Middleware) {
	adm := r.Group("/api/v1/admin", adminMW.Require(auth.RoleAdminOperator))
	{
		adm.GET("/users", h.List)
		adm.PATCH("/users/:uuid/status", h.UpdateStatus)
	}
}

// List GET /api/v1/admin/users
func (h *Handler) List(c *gin.Context) {
	items, total, err := h.svc.List(c.Request.Context(), ListFilter{
		Keyword:   c.Query("keyword"),
		Status:    c.Query("status"),
		RiskLevel: c.Query("riskLevel"),
		Page:      queryInt(c, "page", 1),
		Size:      queryInt(c, "size", 20),
	})
	if err != nil {
		httpapi.Fail(c, err)
		return
	}
	httpapi.OK(c, gin.H{"items": items, "total": total})
}

// UpdateStatus PATCH /api/v1/admin/users/:uuid/status
func (h *Handler) UpdateStatus(c *gin.Context) {
	var req UpdateStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpapi.Fail(c, httpapi.NewDomainError(httpapi.CodeInvalidArg, err.Error(), http.StatusBadRequest))
		return
	}
	u, err := h.svc.UpdateStatus(c.Request.Context(), c.Param("uuid"),
		admin.ActorID(c.Request.Context()), httpapi.TraceID(c.Request.Context()), req.Status)
	if err != nil {
		httpapi.Fail(c, err)
		return
	}
	httpapi.OK(c, u)
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
