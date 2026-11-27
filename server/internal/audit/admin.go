package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// LogItem 审计日志列表项（detail 不含敏感明文）。
type LogItem struct {
	ID         int64           `json:"id"`
	ActorType  string          `json:"actorType"`
	ActorID    string          `json:"actorId"`
	Action     string          `json:"action"`
	TargetType string          `json:"targetType"`
	TargetID   string          `json:"targetId"`
	TraceID    string          `json:"traceId"`
	Detail     json.RawMessage `json:"detail"`
	CreatedAt  time.Time       `json:"createdAt"`
}

// LogFilter 审计查询条件。
type LogFilter struct {
	ActorType string
	Action    string
	Keyword   string
	Page      int
	Size      int
}

// VerifyResult 哈希链校验结果。
type VerifyResult struct {
	OK       bool   `json:"ok"`
	Checked  int    `json:"checked"`
	BrokenAt *int64 `json:"brokenAt"`
}

// ListLogs 后台只读检索审计日志。
func (s *Service) ListLogs(ctx context.Context, f LogFilter) ([]LogItem, int, error) {
	where := ` WHERE 1=1`
	args := []any{}
	if f.ActorType != "" {
		args = append(args, f.ActorType)
		where += fmt.Sprintf(" AND actor_type=$%d", len(args))
	}
	if f.Action != "" {
		args = append(args, "%"+f.Action+"%")
		where += fmt.Sprintf(" AND action ILIKE $%d", len(args))
	}
	if f.Keyword != "" {
		args = append(args, "%"+f.Keyword+"%")
		n := len(args)
		where += fmt.Sprintf(" AND (actor_id ILIKE $%d OR target_id ILIKE $%d OR COALESCE(trace_id,'') ILIKE $%d)", n, n, n)
	}

	var total int
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM circle.audit_logs`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	page, size := f.Page, f.Size
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	args = append(args, size, (page-1)*size)

	rows, err := s.pool.Query(ctx,
		`SELECT id, actor_type, actor_id, action, target_type, target_id,
		        COALESCE(trace_id,''), COALESCE(detail_json,'{}'::jsonb), created_at
		 FROM circle.audit_logs`+where+
			fmt.Sprintf(" ORDER BY id DESC LIMIT $%d OFFSET $%d", len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]LogItem, 0, size)
	for rows.Next() {
		var it LogItem
		var detail []byte
		if err := rows.Scan(&it.ID, &it.ActorType, &it.ActorID, &it.Action, &it.TargetType,
			&it.TargetID, &it.TraceID, &detail, &it.CreatedAt); err != nil {
			return nil, 0, err
		}
		it.Detail = json.RawMessage(detail)
		out = append(out, it)
	}
	return out, total, rows.Err()
}

// VerifyChain 按 id 顺序重算哈希链，定位第一条被篡改的记录。
func (s *Service) VerifyChain(ctx context.Context) (*VerifyResult, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, actor_type, actor_id, action, target_type, target_id,
		        COALESCE(trace_id,''), COALESCE(detail_json,'{}'::jsonb), prev_hash, hash
		 FROM circle.audit_logs ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	res := &VerifyResult{OK: true}
	var prevHash string
	for rows.Next() {
		var (
			id                                                        int64
			actorType, actorID, action, targetType, targetID, traceID string
			detail                                                    []byte
			rowPrev, rowHash                                          string
		)
		if err := rows.Scan(&id, &actorType, &actorID, &action, &targetType, &targetID,
			&traceID, &detail, &rowPrev, &rowHash); err != nil {
			return nil, err
		}
		res.Checked++

		// JSONB 读回的键顺序/空白与写入时不同，先规范化再算哈希。
		expected := computeHash(rowPrev, actorType, actorID, action, targetType, targetID, traceID, canonicalJSON(detail))
		if rowPrev != prevHash || expected != rowHash {
			res.OK = false
			broken := id
			res.BrokenAt = &broken
			return res, nil
		}
		prevHash = rowHash
	}
	return res, rows.Err()
}
