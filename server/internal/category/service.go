package category

import (
	"context"
	"encoding/json"
	"fmt"

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

func (s *Service) ListPublic(ctx context.Context) ([]Category, error) {
	return s.repo.List(ctx, true)
}

func (s *Service) ListAll(ctx context.Context) ([]Category, error) {
	return s.repo.List(ctx, false)
}

// Update 后台改分类；写审计（改价必须留痕）。
func (s *Service) Update(ctx context.Context, code, adminID, traceID string, req *UpdateRequest) (*Category, error) {
	if req.ContactPriceCents != nil && *req.ContactPriceCents < 0 {
		return nil, httpapi.NewDomainError(httpapi.CodeInvalidArg, "联系方式价格不能为负", 400)
	}
	if req.AccessPriceCents != nil && *req.AccessPriceCents < 0 {
		return nil, httpapi.NewDomainError(httpapi.CodeInvalidArg, "分类包价格不能为负", 400)
	}
	if req.SensitiveLevel != nil {
		switch *req.SensitiveLevel {
		case "low", "medium", "high", "extreme":
		default:
			return nil, httpapi.NewDomainError(httpapi.CodeInvalidArg, "敏感级别非法", 400)
		}
	}

	updated, err := s.repo.Update(ctx, code, req)
	if err != nil {
		if err == ErrNotFound {
			return nil, httpapi.NewDomainError(httpapi.CodeResourceNotFound, "分类不存在", 404)
		}
		return nil, fmt.Errorf("update category: %w", err)
	}

	// 审计：分类改价/启停必须可追溯（不回滚，append-only）。
	detail, _ := json.Marshal(req)
	_ = s.audit.Write(ctx, "admin", adminID, "CATEGORY_UPDATE", "category", code, traceID,
		map[string]string{"payload": string(detail)})

	return updated, nil
}
