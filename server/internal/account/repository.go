// Package account repository — SQL only, no business logic.
// All queries use parameter placeholders ($1, $2...). No string concatenation.
package account

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository owns all DB access for the account module.
type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// ---- Users ----

func (r *Repository) CreateUser(ctx context.Context, tx pgx.Tx, nickname string) (int64, error) {
	row := tx.QueryRow(ctx,
		`INSERT INTO circle.users (nickname) VALUES ($1) RETURNING id`,
		nickname,
	)
	var id int64
	if err := row.Scan(&id); err != nil {
		return 0, err
	}
	return id, nil
}

func (r *Repository) GetUserByID(ctx context.Context, id int64) (*User, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT id, uuid, COALESCE(nickname,''), COALESCE(avatar_url,''), status, risk_level, created_at
		 FROM circle.users WHERE id=$1 AND deleted_at IS NULL`, id)
	u := &User{}
	if err := row.Scan(&u.ID, &u.UUID, &u.Nickname, &u.AvatarURL, &u.Status, &u.RiskLevel, &u.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return u, nil
}

func (r *Repository) TouchUserLastLogin(ctx context.Context, userID int64) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE circle.users SET last_login_at=NOW(), updated_at=NOW() WHERE id=$1`, userID)
	return err
}

// ---- WeChat accounts ----

func (r *Repository) GetWxAccount(ctx context.Context, openID, appType string) (*WeChatAccount, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT id, user_id, open_id, COALESCE(union_id,''), app_type, COALESCE(last_session_key,''), created_at
		 FROM circle.wechat_accounts WHERE open_id=$1 AND app_type=$2`, openID, appType)
	w := &WeChatAccount{}
	if err := row.Scan(&w.ID, &w.UserID, &w.OpenID, &w.UnionID, &w.AppType, &w.LastSessionKey, &w.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return w, nil
}

func (r *Repository) CreateWxAccount(ctx context.Context, tx pgx.Tx, userID int64, openID, unionID, appType string) error {
	_, err := tx.Exec(ctx,
		`INSERT INTO circle.wechat_accounts (user_id, open_id, union_id, app_type) VALUES ($1,$2,$3,$4)`,
		userID, openID, unionID, appType)
	return err
}

// GetPrimaryWxForUser returns the first wechat_account row for a user.
func (r *Repository) GetPrimaryWxForUser(ctx context.Context, userID int64) (*WeChatAccount, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT id, user_id, open_id, COALESCE(union_id,''), app_type, COALESCE(last_session_key,''), created_at
		 FROM circle.wechat_accounts WHERE user_id=$1 ORDER BY id ASC LIMIT 1`, userID)
	w := &WeChatAccount{}
	if err := row.Scan(&w.ID, &w.UserID, &w.OpenID, &w.UnionID, &w.AppType, &w.LastSessionKey, &w.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return w, nil
}

func (r *Repository) UpdateWxSessionKey(ctx context.Context, wxAccountID int64, sessionKey string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE circle.wechat_accounts SET last_session_key=$1 WHERE id=$2`, sessionKey, wxAccountID)
	return err
}

// ---- Private phone ----

func (r *Repository) HasPhone(ctx context.Context, userID int64) (bool, error) {
	var count int
	if err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM circle.user_private WHERE user_id=$1`, userID).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *Repository) UpsertPhone(ctx context.Context, userID int64, cipher []byte, hmac, keyID string,
	dekWrapped []byte, nonce []byte) error {
	// Use a single INSERT ... ON CONFLICT to avoid race with duplicate upsert.
	_, err := r.pool.Exec(ctx,
		`INSERT INTO circle.user_private (user_id, phone_cipher, phone_hmac, dek_wrapped, key_id, aes_gcm_nonce)
		 VALUES ($1,$2,$3,$4,$5,$6)
		 ON CONFLICT (user_id) DO UPDATE SET
		   phone_cipher=EXCLUDED.phone_cipher,
		   phone_hmac=EXCLUDED.phone_hmac,
		   dek_wrapped=EXCLUDED.dek_wrapped,
		   key_id=EXCLUDED.key_id,
		   aes_gcm_nonce=EXCLUDED.aes_gcm_nonce,
		   updated_at=NOW()`,
		userID, cipher, hmac, dekWrapped, keyID, nonce)
	return err
}

// ---- IP registration counter ----

// CountIPRegistrations returns how many distinct open_ids registered from
// this IP in the last 24h.
func (r *Repository) CountIPRegistrations(ctx context.Context, ip string) (int, error) {
	var count int
	if err := r.pool.QueryRow(ctx,
		`SELECT COUNT(DISTINCT open_id) FROM circle.ip_registrations
		 WHERE ip=$1::inet AND created_at > NOW() - INTERVAL '24h'`, ip).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (r *Repository) RecordIPRegistration(ctx context.Context, ip, openID string, userID *int64) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO circle.ip_registrations (ip, open_id, user_id) VALUES ($1::inet, $2, $3)`,
		ip, openID, userID)
	return err
}

// ---- Sessions ----

func (r *Repository) CreateSession(ctx context.Context, userID int64, tokenHash, refreshHash, deviceHash, createdIP string, expiresAt string) (int64, error) {
	row := r.pool.QueryRow(ctx,
		`INSERT INTO circle.sessions (user_id, token_hash, refresh_hash, device_hash, created_ip, expires_at)
		 VALUES ($1,$2,$3,$4,$5::inet, $6::timestamptz) RETURNING id`,
		userID, tokenHash, refreshHash, deviceHash, createdIP, expiresAt)
	var id int64
	if err := row.Scan(&id); err != nil {
		return 0, err
	}
	return id, nil
}

func (r *Repository) RevokeSessionByTokenHash(ctx context.Context, tokenHash string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE circle.sessions SET revoked_at=NOW() WHERE token_hash=$1 AND revoked_at IS NULL`, tokenHash)
	return err
}
