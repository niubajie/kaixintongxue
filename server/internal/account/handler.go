// Package account handler — HTTP layer.
// Handlers parse/validate requests, call service, write response wrappers.
// NO business logic, NO SQL here.
package account

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"gocircle/server/internal/auth"
	"gocircle/server/internal/httpapi"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// Register wires account routes onto the router.
func (h *Handler) Register(r *gin.Engine, authMW *auth.Service) {
	v1 := r.Group("/api/v1")
	{
		// Public
		v1.POST("/auth/wx-login", h.WxLogin)

		// Need auth
		acc := v1.Group("", authMW.RequireAuth())
		{
			acc.POST("/account/bind-phone", h.BindPhone)
			acc.POST("/auth/refresh", h.Refresh)
			acc.POST("/auth/logout", h.Logout)
		}
	}
}

// WxLogin POST /api/v1/auth/wx-login
func (h *Handler) WxLogin(c *gin.Context) {
	var req WxLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpapi.Fail(c, httpapi.NewDomainError(httpapi.CodeInvalidArg, err.Error(), http.StatusBadRequest))
		return
	}
	ip := IPFromRequest(c.Request)
	resp, err := h.svc.Login(c.Request.Context(), &req, ip)
	if err != nil {
		httpapi.Fail(c, err)
		return
	}
	httpapi.OK(c, resp)
}

// BindPhone POST /api/v1/account/bind-phone
func (h *Handler) BindPhone(c *gin.Context) {
	var req BindPhoneRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpapi.Fail(c, httpapi.NewDomainError(httpapi.CodeInvalidArg, err.Error(), http.StatusBadRequest))
		return
	}
	u := auth.User(c.Request.Context())
	if u == nil {
		httpapi.Fail(c, httpapi.NewDomainError(httpapi.CodeUnauthorized, "unauthorized", http.StatusUnauthorized))
		return
	}
	resp, err := h.svc.BindPhone(c.Request.Context(), u.UserID, &req)
	if err != nil {
		httpapi.Fail(c, err)
		return
	}
	httpapi.OK(c, resp)
}

// Refresh POST /api/v1/auth/refresh
func (h *Handler) Refresh(c *gin.Context) {
	var req RefreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpapi.Fail(c, httpapi.NewDomainError(httpapi.CodeInvalidArg, err.Error(), http.StatusBadRequest))
		return
	}
	u := auth.User(c.Request.Context())
	resp, err := h.svc.Refresh(c.Request.Context(), u, &req)
	if err != nil {
		httpapi.Fail(c, err)
		return
	}
	httpapi.OK(c, resp)
}

// Logout POST /api/v1/auth/logout — revoke session for current access token.
func (h *Handler) Logout(c *gin.Context) {
	u := auth.User(c.Request.Context())
	if u == nil {
		httpapi.Fail(c, httpapi.NewDomainError(httpapi.CodeUnauthorized, "unauthorized", http.StatusUnauthorized))
		return
	}
	// TODO: call service.Logout to revoke session by access-token hash.
	httpapi.OK(c, gin.H{"revoked": true})
}
