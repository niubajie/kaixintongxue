package resource

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

func (h *Handler) Register(r *gin.Engine, authMW *auth.Service, adminMW *admin.Middleware) {
	v1 := r.Group("/api/v1")
	{
		// 公开
		v1.GET("/resources", h.ListPublic)
		v1.GET("/resources/:uuid", h.Detail)

		// 登录用户
		user := v1.Group("", authMW.RequireAuth())
		{
			user.POST("/resources", h.Create)
			user.GET("/my/resources", h.ListMine)
			user.POST("/resources/:uuid/offline", h.Offline)
			user.DELETE("/resources/:uuid", h.Delete)
		}

		// 后台
		adm := v1.Group("/admin", adminMW.Require(auth.RoleReviewer, auth.RoleAdminOperator))
		{
			adm.GET("/resources", h.ListAdmin)
			adm.POST("/resources/:uuid/review", h.Review)
		}
	}
}

// ListPublic GET /api/v1/resources
func (h *Handler) ListPublic(c *gin.Context) {
	f := ListFilter{
		CategoryCode: c.Query("category"),
		RegionCode:   c.Query("region"),
		Keyword:      c.Query("keyword"),
		Size:         queryInt(c, "size", 20),
	}
	items, err := h.svc.ListPublic(c.Request.Context(), f)
	if err != nil {
		httpapi.Fail(c, err)
		return
	}
	httpapi.OK(c, gin.H{"items": items})
}

// Detail GET /api/v1/resources/:uuid
func (h *Handler) Detail(c *gin.Context) {
	d, err := h.svc.Detail(c.Request.Context(), c.Param("uuid"))
	if err != nil {
		httpapi.Fail(c, err)
		return
	}
	httpapi.OK(c, d)
}

// Create POST /api/v1/resources
func (h *Handler) Create(c *gin.Context) {
	u := auth.User(c.Request.Context())
	if u == nil {
		httpapi.Fail(c, httpapi.NewDomainError(httpapi.CodeUnauthorized, "unauthorized", http.StatusUnauthorized))
		return
	}
	var req CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpapi.Fail(c, httpapi.NewDomainError(httpapi.CodeInvalidArg, err.Error(), http.StatusBadRequest))
		return
	}
	res, err := h.svc.Create(c.Request.Context(), u.UserID, httpapi.TraceID(c.Request.Context()), &req)
	if err != nil {
		httpapi.Fail(c, err)
		return
	}
	httpapi.OK(c, res)
}

// ListMine GET /api/v1/my/resources
func (h *Handler) ListMine(c *gin.Context) {
	u := auth.User(c.Request.Context())
	if u == nil {
		httpapi.Fail(c, httpapi.NewDomainError(httpapi.CodeUnauthorized, "unauthorized", http.StatusUnauthorized))
		return
	}
	items, total, err := h.svc.ListMine(c.Request.Context(), u.UserID, queryInt(c, "page", 1), queryInt(c, "size", 20))
	if err != nil {
		httpapi.Fail(c, err)
		return
	}
	httpapi.OK(c, gin.H{"items": items, "total": total})
}

// Offline POST /api/v1/resources/:uuid/offline
func (h *Handler) Offline(c *gin.Context) {
	u := auth.User(c.Request.Context())
	if u == nil {
		httpapi.Fail(c, httpapi.NewDomainError(httpapi.CodeUnauthorized, "unauthorized", http.StatusUnauthorized))
		return
	}
	err := h.svc.OfflineOwn(c.Request.Context(), c.Param("uuid"), u.UserID, httpapi.TraceID(c.Request.Context()))
	if err != nil {
		httpapi.Fail(c, err)
		return
	}
	httpapi.OK(c, gin.H{"ok": true})
}

// Delete DELETE /api/v1/resources/:uuid
func (h *Handler) Delete(c *gin.Context) {
	u := auth.User(c.Request.Context())
	if u == nil {
		httpapi.Fail(c, httpapi.NewDomainError(httpapi.CodeUnauthorized, "unauthorized", http.StatusUnauthorized))
		return
	}
	err := h.svc.DeleteOwn(c.Request.Context(), c.Param("uuid"), u.UserID, httpapi.TraceID(c.Request.Context()))
	if err != nil {
		httpapi.Fail(c, err)
		return
	}
	httpapi.OK(c, gin.H{"ok": true})
}

// ListAdmin GET /api/v1/admin/resources
func (h *Handler) ListAdmin(c *gin.Context) {
	f := ListFilter{
		Status:       c.Query("status"),
		CategoryCode: c.Query("category"),
		Keyword:      c.Query("keyword"),
		Page:         queryInt(c, "page", 1),
		Size:         queryInt(c, "size", 20),
	}
	items, total, err := h.svc.ListAdmin(c.Request.Context(), f)
	if err != nil {
		httpapi.Fail(c, err)
		return
	}
	httpapi.OK(c, gin.H{"items": items, "total": total})
}

// Review POST /api/v1/admin/resources/:uuid/review
func (h *Handler) Review(c *gin.Context) {
	var req ReviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpapi.Fail(c, httpapi.NewDomainError(httpapi.CodeInvalidArg, err.Error(), http.StatusBadRequest))
		return
	}
	res, err := h.svc.Review(c.Request.Context(), c.Param("uuid"),
		admin.ActorID(c.Request.Context()), httpapi.TraceID(c.Request.Context()), &req)
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
