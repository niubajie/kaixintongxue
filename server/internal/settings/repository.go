package settings

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("settings: not found")

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) List(ctx context.Context) ([]Setting, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT key, (value_json #>> '{}')::bigint, COALESCE(updated_by,''), updated_at
		 FROM circle.settings ORDER BY key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Setting, 0, len(Rules))
	for rows.Next() {
		var s Setting
		if err := rows.Scan(&s.Key, &s.Value, &s.UpdatedBy, &s.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *Repository) Update(ctx context.Context, key string, value int64, adminID string) (*Setting, error) {
	tag, err := r.pool.Exec(ctx,
		`UPDATE circle.settings
		 SET value_json=to_jsonb($1::bigint), updated_by=$2, updated_at=NOW()
		 WHERE key=$3`, value, adminID, key)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrNotFound
	}
	return r.Get(ctx, key)
}

func (r *Repository) Get(ctx context.Context, key string) (*Setting, error) {
	var s Setting
	err := r.pool.QueryRow(ctx,
		`SELECT key, (value_json #>> '{}')::bigint, COALESCE(updated_by,''), updated_at
		 FROM circle.settings WHERE key=$1`, key,
	).Scan(&s.Key, &s.Value, &s.UpdatedBy, &s.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}
