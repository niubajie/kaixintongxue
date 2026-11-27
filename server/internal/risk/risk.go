// Package risk writes risk events for triage (IP limit hits, bot patterns,
// mellow visits, etc.). Does NOT perform enforcement — that happens inside
// service layers. risk_events is purely the append log + later analysis.
package risk

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Severity string

const (
	SeverityLow      Severity = "low"
	SeverityMedium   Severity = "medium"
	SeverityHigh     Severity = "high"
	SeverityCritical Severity = "critical"
)

type Service struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

// Record appends a risk event. userID/deviceHash are optional.
func (s *Service) Record(ctx context.Context, eventType string, severity Severity,
	userID *int64, ip string, deviceHash string, detail any) error {

	var detailJSON []byte
	if detail != nil {
		b, err := json.Marshal(detail)
		if err != nil {
			return err
		}
		detailJSON = b
	}

	_, err := s.pool.Exec(ctx,
		`INSERT INTO circle.risk_events
		   (event_type, severity, user_id, ip, device_hash, detail_json, created_at)
		 VALUES ($1,$2,$3,$4::inet,$5,$6::jsonb,$7::timestamptz)`,
		eventType, string(severity), userID, ip, deviceHash, detailJSON, time.Now().UTC())
	return err
}
