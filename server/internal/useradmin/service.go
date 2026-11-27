package useradmin

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"gocircle/server/internal/audit"
	"gocircle/server/internal/httpapi"
)

type Service struct {
	repo  *Repository
	audit *audit.Service
}

func NewService(repo *Repository, auditSvc *audit.Service) *Service {
	return &Service{repo: repo, audit: auditSvc}
}

func (s *Service) List(ctx context.Context, f ListFilter) ([]User, int, error) {
	switch f.Status {
	case "", "active", "blocked", "pending_delete":
	default:
		return nil, 0, httpapi.NewDomainError(httpapi.CodeInvalidArg, "状态取值非法", http.StatusBadRequest)
	}
	switch f.RiskLevel {
	case "", "normal", "low", "medium", "high":
	default:
		return nil, 0, httpapi.NewDomainError(httpapi.CodeInvalidArg, "风险等级取值非法", http.StatusBadRequest)
	}
	return s.repo.List(ctx, f)
}

// UpdateStatus 封禁或解封用户，并写审计。
func (s *Service) UpdateStatus(ctx context.Context, uuid, adminID, traceID, status string) (*User, error) {
	status = strings.TrimSpace(status)
	if status != "active" && status != "blocked" {
		return nil, httpapi.NewDomainError(httpapi.CodeInvalidArg, "仅支持 active 或 blocked", http.StatusBadRequest)
	}

	u, err := s.repo.UpdateStatus(ctx, uuid, status)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, httpapi.NewDomainError(httpapi.CodeUserNotFound, "用户不存在", http.StatusNotFound)
		}
		return nil, err
	}

	action := "USER_BLOCK"
	if status == "active" {
		action = "USER_UNBLOCK"
	}
	_ = s.audit.Write(ctx, "admin", adminID, action, "user", uuid, traceID,
		map[string]any{"status": status})
	return u, nil
}
