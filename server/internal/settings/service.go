package settings

import (
	"context"
	"errors"
	"fmt"
	"net/http"

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

func (s *Service) List(ctx context.Context) ([]Setting, error) {
	return s.repo.List(ctx)
}

// Update 校验白名单与取值范围后写入，并记录审计。
func (s *Service) Update(ctx context.Context, key string, value *int64, adminID, traceID string) (*Setting, error) {
	rule, ok := Rules[key]
	if !ok {
		return nil, httpapi.NewDomainError(httpapi.CodeInvalidArg, "不支持修改该配置项", http.StatusBadRequest)
	}
	if value == nil {
		return nil, httpapi.NewDomainError(httpapi.CodeInvalidArg, "value 不能为空", http.StatusBadRequest)
	}
	if *value < rule.Min || *value > rule.Max {
		return nil, httpapi.NewDomainError(httpapi.CodeInvalidArg,
			fmt.Sprintf("%s 的取值范围为 %d–%d", rule.Label, rule.Min, rule.Max), http.StatusBadRequest)
	}

	updated, err := s.repo.Update(ctx, key, *value, adminID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, httpapi.NewDomainError(httpapi.CodeInvalidArg, "配置项不存在", http.StatusNotFound)
		}
		return nil, err
	}

	_ = s.audit.Write(ctx, "admin", adminID, "SETTING_UPDATE", "setting", key, traceID,
		map[string]any{"value": *value})
	return updated, nil
}
