package resource

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("resource: not found")

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

const resourceCols = `id, uuid, owner_user_id, category_code, title, summary, detail_md,
	region_code, provider_type, contact_mode, status, quality_score,
	COALESCE(reject_reason,''), expires_at, published_at, created_at`

func scanResource(row pgx.Row) (*Resource, error) {
	var r Resource
	err := row.Scan(&r.ID, &r.UUID, &r.OwnerUserID, &r.CategoryCode, &r.Title, &r.Summary,
		&r.DetailMD, &r.RegionCode, &r.ProviderType, &r.ContactMode, &r.Status,
		&r.QualityScore, &r.RejectReason, &r.ExpiresAt, &r.PublishedAt, &r.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &r, nil
}

// InsertResource 在主表插入（事务由 service 开启）。
func (r *Repository) InsertResource(ctx context.Context, tx pgx.Tx, p *CreateParams) (int64, error) {
	var id int64
	err := tx.QueryRow(ctx,
		`INSERT INTO circle.resources
		   (owner_user_id, category_code, title, summary, detail_md, region_code,
		    provider_type, contact_mode, status, quality_score, dedup_hmac, content_simhash, expires_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,'apply',$8,$9,$10,$11,$12)
		 RETURNING id`,
		p.OwnerUserID, p.CategoryCode, p.Title, p.Summary, p.DetailMD, p.RegionCode,
		p.ProviderType, p.Status, p.QualityScore, p.DedupHMAC, p.ContentSimhash, p.ExpiresAt,
	).Scan(&id)
	return id, err
}

// InsertContact 写联系方式密文（独立表）。
func (r *Repository) InsertContact(ctx context.Context, tx pgx.Tx, resourceID int64, c *ContactCipher) error {
	_, err := tx.Exec(ctx,
		`INSERT INTO circle.resource_contacts
		   (resource_id, contact_name, phone_cipher, phone_hmac, phone_nonce,
		    wechat_cipher, wechat_hmac, wechat_nonce, dek_wrapped, key_id)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		resourceID, c.ContactName, c.PhoneCipher, c.PhoneHMAC, c.PhoneNonce,
		c.WechatCipher, c.WechatHMAC, c.WechatNonce, c.DEKWrapped, c.KeyID)
	return err
}

func (r *Repository) GetByUUID(ctx context.Context, uuid string) (*Resource, error) {
	return scanResource(r.pool.QueryRow(ctx,
		`SELECT `+resourceCols+` FROM circle.resources WHERE uuid=$1 AND deleted_at IS NULL`, uuid))
}

func (r *Repository) GetByID(ctx context.Context, id int64) (*Resource, error) {
	return scanResource(r.pool.QueryRow(ctx,
		`SELECT `+resourceCols+` FROM circle.resources WHERE id=$1 AND deleted_at IS NULL`, id))
}

// GetCategory 返回分类名称、启用状态、免责声明。
func (r *Repository) GetCategory(ctx context.Context, code string) (name string, enabled bool, disclaimer string, err error) {
	err = r.pool.QueryRow(ctx,
		`SELECT name, enabled, COALESCE(disclaimer_text,'') FROM circle.resource_categories WHERE code=$1`, code,
	).Scan(&name, &enabled, &disclaimer)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, "", ErrNotFound
	}
	return name, enabled, disclaimer, err
}

// FindDuplicate 强去重：返回已存在的 uuid（无则空）。
func (r *Repository) FindDuplicate(ctx context.Context, hmac string) (string, error) {
	var uuid string
	err := r.pool.QueryRow(ctx,
		`SELECT uuid FROM circle.resources
		 WHERE dedup_hmac=$1 AND deleted_at IS NULL AND status IN ('pending','online')
		 LIMIT 1`, hmac).Scan(&uuid)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return uuid, err
}

type SimCandidate struct {
	UUID    string
	Simhash int64
}

// SimhashCandidates 取同分类近期候选，供弱去重比对。
func (r *Repository) SimhashCandidates(ctx context.Context, category string, limit int) ([]SimCandidate, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT uuid, content_simhash FROM circle.resources
		 WHERE category_code=$1 AND deleted_at IS NULL AND status IN ('pending','online')
		 ORDER BY id DESC LIMIT $2`, category, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SimCandidate
	for rows.Next() {
		var c SimCandidate
		if err := rows.Scan(&c.UUID, &c.Simhash); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ListPublic 只返回 online、未过期、分类启用的脱敏列表。
func (r *Repository) ListPublic(ctx context.Context, f ListFilter) ([]PublicItem, error) {
	q := `SELECT r.uuid, r.category_code, r.title, r.summary, r.region_code,
	             r.provider_type, r.quality_score, r.published_at
	      FROM circle.resources r
	      JOIN circle.resource_categories c ON c.code = r.category_code
	      WHERE r.deleted_at IS NULL AND r.status='online' AND c.enabled = TRUE
	        AND (r.expires_at IS NULL OR r.expires_at > NOW())`
	args := []any{}
	q = appendFilter(q, &args, "r.category_code", f.CategoryCode)
	q = appendFilter(q, &args, "r.region_code", f.RegionCode)
	if f.Keyword != "" {
		args = append(args, "%"+f.Keyword+"%")
		n := len(args)
		q += fmt.Sprintf(" AND (r.title ILIKE $%d OR r.summary ILIKE $%d)", n, n)
	}
	limit := f.Size
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	args = append(args, limit)
	q += fmt.Sprintf(" ORDER BY r.quality_score DESC, r.published_at DESC NULLS LAST LIMIT $%d", len(args))

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PublicItem
	for rows.Next() {
		var it PublicItem
		if err := rows.Scan(&it.UUID, &it.CategoryCode, &it.Title, &it.Summary,
			&it.RegionCode, &it.ProviderType, &it.QualityScore, &it.PublishedAt); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// GetPublicDetail 详情（脱敏，不含联系方式）。
func (r *Repository) GetPublicDetail(ctx context.Context, uuid string) (*PublicDetail, error) {
	var d PublicDetail
	err := r.pool.QueryRow(ctx,
		`SELECT r.uuid, r.category_code, r.title, r.summary, r.region_code, r.provider_type,
		        r.quality_score, r.published_at, c.name, r.detail_md, COALESCE(c.disclaimer_text,''), r.contact_mode
		 FROM circle.resources r
		 JOIN circle.resource_categories c ON c.code = r.category_code
		 WHERE r.uuid=$1 AND r.deleted_at IS NULL AND r.status='online' AND c.enabled = TRUE
		   AND (r.expires_at IS NULL OR r.expires_at > NOW())`, uuid,
	).Scan(&d.UUID, &d.CategoryCode, &d.Title, &d.Summary, &d.RegionCode, &d.ProviderType,
		&d.QualityScore, &d.PublishedAt, &d.CategoryName, &d.DetailMD, &d.DisclaimerText, &d.ContactMode)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// ListAdmin 后台审核列表（含全部状态）。
func (r *Repository) ListAdmin(ctx context.Context, f ListFilter) ([]Resource, int, error) {
	where := ` WHERE deleted_at IS NULL`
	args := []any{}
	if f.Status != "" {
		args = append(args, f.Status)
		where += fmt.Sprintf(" AND status=$%d", len(args))
	}
	if f.CategoryCode != "" {
		args = append(args, f.CategoryCode)
		where += fmt.Sprintf(" AND category_code=$%d", len(args))
	}
	if f.Keyword != "" {
		args = append(args, "%"+f.Keyword+"%")
		where += fmt.Sprintf(" AND (title ILIKE $%d OR summary ILIKE $%d)", len(args), len(args))
	}

	var total int
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM circle.resources`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	page, size := normalizePage(f.Page, f.Size)
	args = append(args, size, (page-1)*size)
	q := `SELECT ` + resourceCols + ` FROM circle.resources` + where +
		fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d OFFSET $%d", len(args)-1, len(args))

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []Resource
	for rows.Next() {
		res, err := scanResource(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *res)
	}
	return out, total, rows.Err()
}

// ListByOwner 用户自己的资源。
func (r *Repository) ListByOwner(ctx context.Context, userID int64, page, size int) ([]Resource, int, error) {
	var total int
	if err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM circle.resources WHERE owner_user_id=$1 AND deleted_at IS NULL`, userID).Scan(&total); err != nil {
		return nil, 0, err
	}
	page, size = normalizePage(page, size)
	rows, err := r.pool.Query(ctx,
		`SELECT `+resourceCols+` FROM circle.resources
		 WHERE owner_user_id=$1 AND deleted_at IS NULL
		 ORDER BY created_at DESC LIMIT $2 OFFSET $3`, userID, size, (page-1)*size)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []Resource
	for rows.Next() {
		res, err := scanResource(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *res)
	}
	return out, total, rows.Err()
}

// Transition 状态流转；from 为允许的当前状态集合。返回是否命中。
func (r *Repository) Transition(ctx context.Context, id int64, from []string, to, reason, reviewedBy string) (bool, error) {
	tag, err := r.pool.Exec(ctx,
		`UPDATE circle.resources SET
		   status=$1,
		   reject_reason=NULLIF($2,''),
		   reviewed_by=NULLIF($3,''),
		   reviewed_at=NOW(),
		   published_at=CASE WHEN $1='online' THEN COALESCE(published_at, NOW()) ELSE published_at END,
		   updated_at=NOW()
		 WHERE id=$4 AND deleted_at IS NULL AND status = ANY($5)`,
		to, reason, reviewedBy, id, from)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// SoftDelete 仅本人可删，且仅限未上线的状态。
func (r *Repository) SoftDelete(ctx context.Context, uuid string, ownerID int64) (bool, error) {
	tag, err := r.pool.Exec(ctx,
		`UPDATE circle.resources SET deleted_at=NOW(), updated_at=NOW()
		 WHERE uuid=$1 AND owner_user_id=$2 AND deleted_at IS NULL
		   AND status IN ('draft','pending','rejected','offline','expired')`, uuid, ownerID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// ExpireDue 把已过期但仍 online 的资源置为 expired。返回处理条数。
func (r *Repository) ExpireDue(ctx context.Context) (int64, error) {
	tag, err := r.pool.Exec(ctx,
		`UPDATE circle.resources SET status='expired', updated_at=NOW()
		 WHERE status='online' AND expires_at IS NOT NULL AND expires_at < NOW() AND deleted_at IS NULL`)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func appendFilter(q string, args *[]any, col, val string) string {
	if val == "" {
		return q
	}
	*args = append(*args, val)
	return q + fmt.Sprintf(" AND %s=$%d", col, len(*args))
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
