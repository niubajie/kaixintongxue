// Package audit writes immutable append-only audit logs.
// Hash chain (prev_hash → hash) enables tamper detection.
//
// Critical: NEVER store plaintext phone / token / audio body in detail_json.
package audit

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

// Write appends one audit row. hash = sha256(prev_hash + action + actor + target + trace_id + detail_json).
func (s *Service) Write(ctx context.Context, actorType, actorID, action, targetType, targetID, traceID string, detail any) error {
	detailJSON, err := json.Marshal(detail)
	if err != nil {
		return err
	}
	detailJSON = canonicalJSON(detailJSON)

	// Read latest hash for chain continuity.
	var prevHash string
	if err := s.pool.QueryRow(ctx,
		`SELECT COALESCE(hash,'') FROM circle.audit_logs ORDER BY id DESC LIMIT 1`).Scan(&prevHash); err != nil {
		prevHash = ""
	}

	hash := computeHash(prevHash, actorType, actorID, action, targetType, targetID, traceID, detailJSON)

	_, err = s.pool.Exec(ctx,
		`INSERT INTO circle.audit_logs
		   (actor_type, actor_id, action, target_type, target_id, trace_id, detail_json, prev_hash, hash, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7::jsonb,$8,$9,$10::timestamptz)`,
		actorType, actorID, action, targetType, targetID, traceID, detailJSON, prevHash, hash, time.Now().UTC())
	return err
}

// canonicalJSON 把 JSON 规范化成「键按字典序、无多余空白」的形式。
// 原因：JSONB 读回时键顺序由 PostgreSQL 决定，直接哈希会对不上；两端都先规范化再算。
func canonicalJSON(raw []byte) []byte {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return raw
	}
	out, err := json.Marshal(v)
	if err != nil {
		return raw
	}
	return out
}

func computeHash(prev, actorType, actorID, action, targetType, targetID, traceID string, detailJSON []byte) string {
	h := sha256.New()
	h.Write([]byte(prev))
	h.Write([]byte("|"))
	h.Write([]byte(actorType))
	h.Write([]byte("|"))
	h.Write([]byte(actorID))
	h.Write([]byte("|"))
	h.Write([]byte(action))
	h.Write([]byte("|"))
	h.Write([]byte(targetType))
	h.Write([]byte("|"))
	h.Write([]byte(targetID))
	h.Write([]byte("|"))
	h.Write([]byte(traceID))
	h.Write([]byte("|"))
	h.Write(detailJSON)
	return hex.EncodeToString(h.Sum(nil))
}
