package category

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

// Register 注册分类路由。adminMW 负责后台鉴权。
func (h *Handler) Register(r *gin.Engine, authMW *auth.Service, adminMW *admin.Middleware) {
	v1 := r.Group("/api/v1")
	{
		v1.GET("/categories", h.ListPublic)

		adm := v1.Group("/admin", adminMW.Require(auth.RoleAdminOperator))
		{
			adm.GET("/categories", h.ListAll)
			adm.PATCH("/categories/:code", h.Update)
		}
	}
}

// ListPublic GET /api/v1/categories
func (h *Handler) ListPublic(c *gin.Context) {
	list, err := h.svc.ListPublic(c.Request.Context())
	if err != nil {
		httpapi.Fail(c, err)
		return
	}
	httpapi.OK(c, gin.H{"items": list})
}

// ListAll GET /api/v1/admin/categories
func (h *Handler) ListAll(c *gin.Context) {
	list, err := h.svc.ListAll(c.Request.Context())
	if err != nil {
		httpapi.Fail(c, err)
		return
	}
	httpapi.OK(c, gin.H{"items": list})
}

// Update PATCH /api/v1/admin/categories/:code
func (h *Handler) Update(c *gin.Context) {
	var req UpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpapi.Fail(c, httpapi.NewDomainError(httpapi.CodeInvalidArg, err.Error(), http.StatusBadRequest))
		return
	}
	adminID := admin.ActorID(c.Request.Context())
	updated, err := h.svc.Update(c.Request.Context(), c.Param("code"), adminID, httpapi.TraceID(c.Request.Context()), &req)
	if err != nil {
		httpapi.Fail(c, err)
		return
	}
	httpapi.OK(c, updated)
}
