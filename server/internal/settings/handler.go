package settings

import (
	"net/http"

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
		adm.GET("/settings", h.List)
		adm.PATCH("/settings/:key", h.Update)
	}
}

// List GET /api/v1/admin/settings
func (h *Handler) List(c *gin.Context) {
	items, err := h.svc.List(c.Request.Context())
	if err != nil {
		httpapi.Fail(c, err)
		return
	}
	httpapi.OK(c, gin.H{"items": items})
}

// Update PATCH /api/v1/admin/settings/:key
func (h *Handler) Update(c *gin.Context) {
	var req UpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpapi.Fail(c, httpapi.NewDomainError(httpapi.CodeInvalidArg, err.Error(), http.StatusBadRequest))
		return
	}
	updated, err := h.svc.Update(c.Request.Context(), c.Param("key"), req.Value,
		admin.ActorID(c.Request.Context()), httpapi.TraceID(c.Request.Context()))
	if err != nil {
		httpapi.Fail(c, err)
		return
	}
	httpapi.OK(c, updated)
}
