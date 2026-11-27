package admin

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"gocircle/server/internal/httpapi"
)

// Handler 提供后台登录入口。
type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) Register(r *gin.Engine) {
	v1 := r.Group("/api/v1/admin")
	v1.POST("/auth/login", h.Login)
}

// Login POST /api/v1/admin/auth/login
func (h *Handler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpapi.Fail(c, httpapi.NewDomainError(httpapi.CodeInvalidArg, err.Error(), http.StatusBadRequest))
		return
	}
	ip := c.ClientIP()
	resp, err := h.svc.Login(c.Request.Context(), &req, ip, httpapi.TraceID(c.Request.Context()))
	if err != nil {
		httpapi.Fail(c, err)
		return
	}
	httpapi.OK(c, resp)
}
