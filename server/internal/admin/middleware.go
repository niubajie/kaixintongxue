package admin

import (
	"context"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"gocircle/server/internal/auth"
)

type ctxAdminKey struct{}

// Middleware 校验后台令牌并做角色判定。
type Middleware struct {
	auth *auth.Service
}

func NewMiddleware(a *auth.Service) *Middleware {
	return &Middleware{auth: a}
}

// Require 要求令牌有效，且角色属于 allowed（super_admin 恒定放行）。
func (m *Middleware) Require(allowed ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := bearer(c.GetHeader("Authorization"))
		if token == "" {
			c.AbortWithStatusJSON(401, gin.H{"code": 10002, "message": "missing token"})
			return
		}
		payload, err := m.auth.VerifyAccess(token)
		if err != nil {
			c.AbortWithStatusJSON(401, gin.H{"code": 10002, "message": "token invalid"})
			return
		}
		if payload.Role == "" {
			c.AbortWithStatusJSON(403, gin.H{"code": 10003, "message": "not an admin"})
			return
		}
		if payload.Role != auth.RoleSuperAdmin && !contains(allowed, payload.Role) {
			c.AbortWithStatusJSON(403, gin.H{"code": 10003, "message": "permission denied"})
			return
		}
		ctx := context.WithValue(c.Request.Context(), ctxAdminKey{}, payload)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

// ActorID 返回当前后台操作人 ID 字符串，用于审计。
func ActorID(ctx context.Context) string {
	if p, ok := ctx.Value(ctxAdminKey{}).(*auth.Payload); ok {
		return strconv.FormatInt(p.UserID, 10)
	}
	return "unknown"
}

func bearer(h string) string {
	if len(h) > 7 && strings.EqualFold(h[:7], "Bearer ") {
		return h[7:]
	}
	return ""
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
