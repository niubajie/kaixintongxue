package risk

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// Event 风控事件列表项。
type Event struct {
	ID         int64           `json:"id"`
	EventType  string          `json:"eventType"`
	Severity   string          `json:"severity"`
	UserID     *int64          `json:"userId"`
	IP         string          `json:"ip"`
	DeviceHash string          `json:"deviceHash"`
	Detail     json.RawMessage `json:"detail"`
	CreatedAt  time.Time       `json:"createdAt"`
}

// EventFilter 风控事件查询条件。
type EventFilter struct {
	Severity  string
	EventType string
	Page      int
	Size      int
}

// ListEvents 后台只读查询风控事件。
func (s *Service) ListEvents(ctx context.Context, f EventFilter) ([]Event, int, error) {
	where := ` WHERE 1=1`
	args := []any{}
	if f.Severity != "" {
		args = append(args, f.Severity)
		where += fmt.Sprintf(" AND severity=$%d", len(args))
	}
	if f.EventType != "" {
		args = append(args, "%"+f.EventType+"%")
		where += fmt.Sprintf(" AND event_type ILIKE $%d", len(args))
	}

	var total int
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM circle.risk_events`+where, args...).Scan(&total); err != nil {
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
		`SELECT id, event_type, severity, user_id, COALESCE(host(ip),''),
		        COALESCE(device_hash,''), COALESCE(detail_json,'{}'::jsonb), created_at
		 FROM circle.risk_events`+where+
			fmt.Sprintf(" ORDER BY id DESC LIMIT $%d OFFSET $%d", len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]Event, 0, size)
	for rows.Next() {
		var e Event
		var detail []byte
		if err := rows.Scan(&e.ID, &e.EventType, &e.Severity, &e.UserID, &e.IP,
			&e.DeviceHash, &detail, &e.CreatedAt); err != nil {
			return nil, 0, err
		}
		e.Detail = json.RawMessage(detail)
		out = append(out, e)
	}
	return out, total, rows.Err()
}
