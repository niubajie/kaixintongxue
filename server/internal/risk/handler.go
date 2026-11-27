package risk

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
	adm := r.Group("/api/v1/admin", adminMW.Require(auth.RoleAdminOperator, auth.RoleAuditor))
	adm.GET("/risk/events", h.ListEvents)
}

// ListEvents GET /api/v1/admin/risk/events
func (h *Handler) ListEvents(c *gin.Context) {
	severity := c.Query("severity")
	switch severity {
	case "", "low", "medium", "high", "critical":
	default:
		httpapi.Fail(c, httpapi.NewDomainError(httpapi.CodeInvalidArg, "风险级别取值非法", http.StatusBadRequest))
		return
	}
	items, total, err := h.svc.ListEvents(c.Request.Context(), EventFilter{
		Severity:  severity,
		EventType: c.Query("eventType"),
		Page:      queryInt(c, "page", 1),
		Size:      queryInt(c, "size", 20),
	})
	if err != nil {
		httpapi.Fail(c, err)
		return
	}
	httpapi.OK(c, gin.H{"items": items, "total": total})
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
