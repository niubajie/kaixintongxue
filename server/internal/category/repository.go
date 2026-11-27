package category

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("category: not found")

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) List(ctx context.Context, onlyEnabled bool) ([]Category, error) {
	q := `SELECT code, name, enabled, sort_order, contact_price_cents, access_price_cents,
	             sensitive_level, COALESCE(disclaimer_text,''), created_at
	      FROM circle.resource_categories`
	if onlyEnabled {
		q += ` WHERE enabled = TRUE`
	}
	q += ` ORDER BY sort_order ASC, code ASC`

	rows, err := r.pool.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Category
	for rows.Next() {
		var c Category
		if err := rows.Scan(&c.Code, &c.Name, &c.Enabled, &c.SortOrder,
			&c.ContactPriceCents, &c.AccessPriceCents, &c.SensitiveLevel,
			&c.DisclaimerText, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *Repository) Get(ctx context.Context, code string) (*Category, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT code, name, enabled, sort_order, contact_price_cents, access_price_cents,
		        sensitive_level, COALESCE(disclaimer_text,''), created_at
		 FROM circle.resource_categories WHERE code=$1`, code)
	var c Category
	if err := row.Scan(&c.Code, &c.Name, &c.Enabled, &c.SortOrder,
		&c.ContactPriceCents, &c.AccessPriceCents, &c.SensitiveLevel,
		&c.DisclaimerText, &c.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &c, nil
}

// Update 只更新非 nil 字段；每次调用由 service 写审计。
func (r *Repository) Update(ctx context.Context, code string, req *UpdateRequest) (*Category, error) {
	row := r.pool.QueryRow(ctx,
		`UPDATE circle.resource_categories SET
		   enabled = COALESCE($2, enabled),
		   sort_order = COALESCE($3, sort_order),
		   contact_price_cents = COALESCE($4, contact_price_cents),
		   access_price_cents = COALESCE($5, access_price_cents),
		   sensitive_level = COALESCE($6, sensitive_level),
		   disclaimer_text = COALESCE($7, disclaimer_text)
		 WHERE code=$1
		 RETURNING code, name, enabled, sort_order, contact_price_cents, access_price_cents,
		           sensitive_level, COALESCE(disclaimer_text,''), created_at`,
		code, req.Enabled, req.SortOrder, req.ContactPriceCents,
		req.AccessPriceCents, req.SensitiveLevel, req.DisclaimerText)

	var c Category
	if err := row.Scan(&c.Code, &c.Name, &c.Enabled, &c.SortOrder,
		&c.ContactPriceCents, &c.AccessPriceCents, &c.SensitiveLevel,
		&c.DisclaimerText, &c.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &c, nil
}
