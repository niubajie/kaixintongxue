package useradmin

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("useradmin: not found")

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

const userCols = `u.uuid::text, COALESCE(u.nickname,''), u.status, u.risk_level,
	(p.user_id IS NOT NULL),
	(SELECT COUNT(*) FROM circle.resources res WHERE res.owner_user_id = u.id AND res.deleted_at IS NULL),
	u.last_login_at, u.created_at`

func (r *Repository) List(ctx context.Context, f ListFilter) ([]User, int, error) {
	where := ` WHERE u.deleted_at IS NULL`
	args := []any{}
	if f.Keyword != "" {
		args = append(args, "%"+f.Keyword+"%")
		where += fmt.Sprintf(" AND (u.nickname ILIKE $%d OR u.uuid::text ILIKE $%d)", len(args), len(args))
	}
	if f.Status != "" {
		args = append(args, f.Status)
		where += fmt.Sprintf(" AND u.status=$%d", len(args))
	}
	if f.RiskLevel != "" {
		args = append(args, f.RiskLevel)
		where += fmt.Sprintf(" AND u.risk_level=$%d", len(args))
	}

	var total int
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM circle.users u`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	page, size := normalizePage(f.Page, f.Size)
	args = append(args, size, (page-1)*size)
	q := `SELECT ` + userCols + ` FROM circle.users u
	      LEFT JOIN circle.user_private p ON p.user_id = u.id` + where +
		fmt.Sprintf(" ORDER BY u.created_at DESC LIMIT $%d OFFSET $%d", len(args)-1, len(args))

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]User, 0, size)
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.UUID, &u.Nickname, &u.Status, &u.RiskLevel, &u.PhoneBound,
			&u.ResourceCount, &u.LastLoginAt, &u.CreatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, u)
	}
	return out, total, rows.Err()
}

// UpdateStatus 只允许 active / blocked 之间切换，返回更新后的用户。
func (r *Repository) UpdateStatus(ctx context.Context, uuid, status string) (*User, error) {
	tag, err := r.pool.Exec(ctx,
		`UPDATE circle.users SET status=$1, updated_at=NOW()
		 WHERE uuid=$2 AND deleted_at IS NULL`, status, uuid)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrNotFound
	}
	return r.GetByUUID(ctx, uuid)
}

func (r *Repository) GetByUUID(ctx context.Context, uuid string) (*User, error) {
	var u User
	err := r.pool.QueryRow(ctx,
		`SELECT `+userCols+` FROM circle.users u
		 LEFT JOIN circle.user_private p ON p.user_id = u.id
		 WHERE u.uuid=$1 AND u.deleted_at IS NULL`, uuid,
	).Scan(&u.UUID, &u.Nickname, &u.Status, &u.RiskLevel, &u.PhoneBound,
		&u.ResourceCount, &u.LastLoginAt, &u.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func normalizePage(page, size int) (int, int) {
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	return page, size
}
