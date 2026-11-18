package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// 与 Python 版完全兼容的表结构，新增字段用 ADD COLUMN IF NOT EXISTS 平滑升级。
var schemaStatements = []string{
	`CREATE TABLE IF NOT EXISTS users (
		id BIGSERIAL PRIMARY KEY,
		account TEXT NOT NULL UNIQUE,
		password_hash TEXT,
		password_salt TEXT,
		display_name TEXT NOT NULL,
		phone TEXT,
		wechat_open_id TEXT,
		role TEXT NOT NULL DEFAULT 'Member',
		created_at TEXT NOT NULL,
		last_login_at TEXT
	)`,
	`CREATE TABLE IF NOT EXISTS sessions (
		token TEXT PRIMARY KEY,
		user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		created_at TEXT NOT NULL,
		expires_at TEXT NOT NULL,
		client TEXT
	)`,
	`CREATE TABLE IF NOT EXISTS software (
		id BIGSERIAL PRIMARY KEY,
		name TEXT NOT NULL UNIQUE,
		winget_id TEXT NOT NULL DEFAULT '',
		purpose TEXT NOT NULL DEFAULT '',
		required BOOLEAN NOT NULL DEFAULT FALSE,
		notes TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS software_versions (
		id BIGSERIAL PRIMARY KEY,
		software_id BIGINT NOT NULL REFERENCES software(id) ON DELETE CASCADE,
		version TEXT NOT NULL,
		file_name TEXT NOT NULL,
		file_path TEXT NOT NULL,
		file_size BIGINT NOT NULL,
		sha256 TEXT NOT NULL,
		scan_status TEXT NOT NULL,
		scan_detail TEXT NOT NULL DEFAULT '',
		scanned_at TEXT,
		published_at TEXT NOT NULL,
		is_current BOOLEAN NOT NULL DEFAULT FALSE,
		release_notes TEXT NOT NULL DEFAULT '',
		UNIQUE (software_id, version)
	)`,
	`CREATE TABLE IF NOT EXISTS version_notes (
		id BIGSERIAL PRIMARY KEY,
		version TEXT NOT NULL,
		title TEXT NOT NULL,
		detail TEXT NOT NULL,
		published_at TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS downloads (
		id BIGSERIAL PRIMARY KEY,
		user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
		software_id BIGINT NOT NULL,
		version_id BIGINT,
		created_at TEXT NOT NULL,
		ip TEXT
	)`,
	`CREATE TABLE IF NOT EXISTS audit (
		id BIGSERIAL PRIMARY KEY,
		actor TEXT,
		action TEXT NOT NULL,
		detail TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS idx_sessions_expires ON sessions(expires_at)`,
	`CREATE INDEX IF NOT EXISTS idx_versions_software ON software_versions(software_id, published_at DESC)`,
	`ALTER TABLE users ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'active'`,
	`ALTER TABLE software ADD COLUMN IF NOT EXISTS summary TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE software ADD COLUMN IF NOT EXISTS ai_software TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE software_versions ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'published'`,
}

func initSchema(ctx context.Context, pool *pgxpool.Pool) error {
	for _, statement := range schemaStatements {
		if _, err := pool.Exec(ctx, statement); err != nil {
			return fmt.Errorf("建表失败: %w", err)
		}
	}
	return nil
}

const softwareColumns = "id, name, winget_id, purpose, summary, ai_software, required, notes"

const versionColumns = `id, software_id, version, file_name, file_path, file_size, sha256,
	scan_status, scan_detail, scanned_at, published_at, is_current, release_notes, status`

const userColumns = "id, account, display_name, coalesce(phone, ''), coalesce(wechat_open_id, ''), role, status, created_at, last_login_at"

func scanUser(row pgx.Row) (User, error) {
	var user User
	err := row.Scan(&user.ID, &user.Account, &user.DisplayName, &user.Phone, &user.WechatOpenID,
		&user.Role, &user.Status, &user.CreatedAt, &user.LastLoginAt)
	return user, err
}

func scanVersion(row pgx.Row) (Version, error) {
	var version Version
	err := row.Scan(&version.ID, &version.SoftwareID, &version.Version, &version.FileName, &version.FilePath,
		&version.FileSize, &version.SHA256, &version.ScanStatus, &version.ScanDetail, &version.ScannedAt,
		&version.PublishedAt, &version.IsCurrent, &version.ReleaseNotes, &version.Status)
	return version, err
}

func scanSoftware(row pgx.Row) (Software, error) {
	var software Software
	err := row.Scan(&software.ID, &software.Name, &software.WingetID, &software.Purpose, &software.Summary,
		&software.AISoftware, &software.Required, &software.Notes)
	return software, err
}

// ---------- 用户与会话 ----------

func (a *App) findUserByAccount(ctx context.Context, account string) (*User, error) {
	row := a.db.QueryRow(ctx, "SELECT "+userColumns+" FROM users WHERE account = $1", account)
	user, err := scanUser(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (a *App) findUserByID(ctx context.Context, id int64) (*User, error) {
	row := a.db.QueryRow(ctx, "SELECT "+userColumns+" FROM users WHERE id = $1", id)
	user, err := scanUser(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (a *App) getPassword(ctx context.Context, account string) (hash, salt string) {
	row := a.db.QueryRow(ctx, "SELECT coalesce(password_hash, ''), coalesce(password_salt, '') FROM users WHERE account = $1", account)
	_ = row.Scan(&hash, &salt)
	return hash, salt
}

func (a *App) userByToken(ctx context.Context, token string) (*User, error) {
	if token == "" {
		return nil, nil
	}
	row := a.db.QueryRow(ctx, `SELECT u.id, u.account, u.display_name, coalesce(u.phone, ''),
			coalesce(u.wechat_open_id, ''), u.role, u.status, u.created_at, u.last_login_at
		FROM sessions s JOIN users u ON u.id = s.user_id
		WHERE s.token = $1 AND s.expires_at > $2 AND u.status = 'active'`, token, nowText())
	user, err := scanUser(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (a *App) createUser(ctx context.Context, account, password, displayName, phone string) (*User, error) {
	hash, salt := hashPassword(password)
	var id int64
	err := a.db.QueryRow(ctx, `INSERT INTO users
			(account, password_hash, password_salt, display_name, phone, role, created_at)
		VALUES ($1, $2, $3, $4, nullif($5, ''), 'Member', $6) RETURNING id`,
		account, hash, salt, displayName, phone, nowText()).Scan(&id)
	if err != nil {
		return nil, err
	}
	return a.findUserByID(ctx, id)
}

func (a *App) createSession(ctx context.Context, userID int64, client string) (string, error) {
	token := newToken()
	now := time.Now()
	_, err := a.db.Exec(ctx, `INSERT INTO sessions (token, user_id, created_at, expires_at, client)
		VALUES ($1, $2, $3, $4, $5)`,
		token, userID, now.Format(timeLayout), now.Add(30*24*time.Hour).Format(timeLayout), client)
	return token, err
}

func (a *App) deleteSession(ctx context.Context, token string) {
	if token == "" {
		return
	}
	_, _ = a.db.Exec(ctx, "DELETE FROM sessions WHERE token = $1", token)
}

func (a *App) touchLogin(ctx context.Context, userID int64) {
	_, _ = a.db.Exec(ctx, "UPDATE users SET last_login_at = $1 WHERE id = $2", nowText(), userID)
}

func (a *App) listUsers(ctx context.Context, keyword, role string, limit, offset int) ([]User, int, error) {
	clause := ""
	args := []any{}
	if keyword != "" {
		args = append(args, "%"+keyword+"%")
		clause += fmt.Sprintf(" AND (account ILIKE $%d OR display_name ILIKE $%d OR phone ILIKE $%d)", len(args), len(args), len(args))
	}
	if role != "" {
		args = append(args, role)
		clause += fmt.Sprintf(" AND role = $%d", len(args))
	}

	var total int
	if err := a.db.QueryRow(ctx, "SELECT count(1) FROM users WHERE true"+clause, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	args = append(args, limit, offset)
	rows, err := a.db.Query(ctx, "SELECT "+userColumns+" FROM users WHERE true"+clause+
		fmt.Sprintf(" ORDER BY created_at DESC, id DESC LIMIT $%d OFFSET $%d", len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	users := []User{}
	for rows.Next() {
		user, err := scanUser(rows)
		if err != nil {
			return nil, 0, err
		}
		users = append(users, user)
	}
	return users, total, rows.Err()
}

func (a *App) setUserRole(ctx context.Context, id int64, role string) error {
	_, err := a.db.Exec(ctx, "UPDATE users SET role = $1 WHERE id = $2", role, id)
	return err
}

func (a *App) setUserStatus(ctx context.Context, id int64, status string) error {
	if _, err := a.db.Exec(ctx, "UPDATE users SET status = $1 WHERE id = $2", status, id); err != nil {
		return err
	}
	if status != "active" {
		_, _ = a.db.Exec(ctx, "DELETE FROM sessions WHERE user_id = $1", id)
	}
	return nil
}

func (a *App) deleteUser(ctx context.Context, id int64) error {
	_, err := a.db.Exec(ctx, "DELETE FROM users WHERE id = $1", id)
	return err
}

// ensureOwner 保证系统里至少有一个 Owner。
// 全新部署时第一个注册的人应该是拥有者，否则谁都无法管理成员与目录。
func (a *App) ensureOwner(ctx context.Context) error {
	if a.countOwners(ctx) > 0 {
		return nil
	}
	var id int64
	var account string
	err := a.db.QueryRow(ctx,
		`SELECT id, account FROM users ORDER BY id ASC LIMIT 1`).Scan(&id, &account)
	if err != nil {
		return nil // 还没有任何用户，等第一个注册的人
	}
	if err := a.setUserRole(ctx, id, "Owner"); err != nil {
		return err
	}
	a.audit(ctx, "system", "owner_bootstrap", account+" 被设为 Owner（首个账号）")
	log.Printf("首个账号 %s 已提升为 Owner", account)
	return nil
}

func (a *App) countOwners(ctx context.Context) int {
	var total int
	_ = a.db.QueryRow(ctx, "SELECT count(1) FROM users WHERE role = 'Owner'").Scan(&total)
	return total
}

func (a *App) audit(ctx context.Context, actor, action, detail string) {
	_, _ = a.db.Exec(ctx, "INSERT INTO audit (actor, action, detail, created_at) VALUES ($1, $2, $3, $4)",
		actor, action, detail, nowText())
}

// ---------- 技能与版本 ----------

func (a *App) listSoftware(ctx context.Context) ([]Software, error) {
	rows, err := a.db.Query(ctx, "SELECT "+softwareColumns+" FROM software ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []Software{}
	for rows.Next() {
		software, err := scanSoftware(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, software)
	}
	return items, rows.Err()
}

func (a *App) findSoftware(ctx context.Context, id int64) (*Software, error) {
	row := a.db.QueryRow(ctx, "SELECT "+softwareColumns+" FROM software WHERE id = $1", id)
	software, err := scanSoftware(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &software, nil
}

func (a *App) findSoftwareByName(ctx context.Context, name string) (*Software, error) {
	row := a.db.QueryRow(ctx, "SELECT "+softwareColumns+" FROM software WHERE name = $1", name)
	software, err := scanSoftware(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &software, nil
}

func (a *App) createSoftware(ctx context.Context, input Software) (int64, error) {
	var id int64
	err := a.db.QueryRow(ctx, `INSERT INTO software
			(name, winget_id, purpose, summary, ai_software, required, notes, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING id`,
		input.Name, input.WingetID, input.Purpose, input.Summary, input.AISoftware, input.Required, input.Notes,
		nowText()).Scan(&id)
	return id, err
}

func (a *App) updateSoftware(ctx context.Context, id int64, input Software) error {
	_, err := a.db.Exec(ctx, `UPDATE software SET name = $1, winget_id = $2, purpose = $3,
		summary = $4, ai_software = $5, required = $6, notes = $7 WHERE id = $8`,
		input.Name, input.WingetID, input.Purpose, input.Summary, input.AISoftware, input.Required, input.Notes, id)
	return err
}

func (a *App) deleteSoftware(ctx context.Context, id int64) error {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "DELETE FROM software_versions WHERE software_id = $1", id); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, "DELETE FROM software WHERE id = $1", id); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (a *App) listVersions(ctx context.Context, softwareID int64) ([]Version, error) {
	rows, err := a.db.Query(ctx, "SELECT "+versionColumns+
		" FROM software_versions WHERE software_id = $1 ORDER BY published_at DESC, id DESC", softwareID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []Version{}
	for rows.Next() {
		version, err := scanVersion(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, version)
	}
	return items, rows.Err()
}

func (a *App) currentVersion(ctx context.Context, softwareID int64) (*Version, error) {
	row := a.db.QueryRow(ctx, "SELECT "+versionColumns+
		" FROM software_versions WHERE software_id = $1 AND is_current ORDER BY published_at DESC LIMIT 1", softwareID)
	version, err := scanVersion(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &version, nil
}

func (a *App) getVersion(ctx context.Context, id int64) (*Version, error) {
	row := a.db.QueryRow(ctx, "SELECT "+versionColumns+" FROM software_versions WHERE id = $1", id)
	version, err := scanVersion(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &version, nil
}

func (a *App) upsertVersion(ctx context.Context, version Version) (int64, error) {
	var id int64
	err := a.db.QueryRow(ctx, `INSERT INTO software_versions
			(software_id, version, file_name, file_path, file_size, sha256, scan_status, scan_detail,
			 scanned_at, published_at, is_current, release_notes, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, false, $11, $12)
		ON CONFLICT (software_id, version) DO UPDATE SET
			file_name = EXCLUDED.file_name,
			file_path = EXCLUDED.file_path,
			file_size = EXCLUDED.file_size,
			sha256 = EXCLUDED.sha256,
			scan_status = EXCLUDED.scan_status,
			scan_detail = EXCLUDED.scan_detail,
			scanned_at = EXCLUDED.scanned_at,
			published_at = EXCLUDED.published_at,
			release_notes = EXCLUDED.release_notes,
			status = EXCLUDED.status
		RETURNING id`,
		version.SoftwareID, version.Version, version.FileName, version.FilePath, version.FileSize,
		version.SHA256, version.ScanStatus, version.ScanDetail, nowText(), nowText(),
		version.ReleaseNotes, version.Status).Scan(&id)
	return id, err
}

func (a *App) setCurrentVersion(ctx context.Context, softwareID, versionID int64) error {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "UPDATE software_versions SET is_current = false WHERE software_id = $1", softwareID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, "UPDATE software_versions SET is_current = true WHERE id = $1", versionID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (a *App) setVersionStatus(ctx context.Context, id int64, status string) error {
	_, err := a.db.Exec(ctx, "UPDATE software_versions SET status = $1 WHERE id = $2", status, id)
	return err
}

func (a *App) updateScan(ctx context.Context, id int64, status, detail string) error {
	_, err := a.db.Exec(ctx, "UPDATE software_versions SET scan_status = $1, scan_detail = $2, scanned_at = $3 WHERE id = $4",
		status, detail, nowText(), id)
	return err
}

func (a *App) listNotes(ctx context.Context) ([]Note, error) {
	rows, err := a.db.Query(ctx, "SELECT version, title, detail, published_at FROM version_notes ORDER BY id DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []Note{}
	for rows.Next() {
		var note Note
		if err := rows.Scan(&note.Version, &note.Title, &note.Detail, &note.PublishedAt); err != nil {
			return nil, err
		}
		items = append(items, note)
	}
	return items, rows.Err()
}

func (a *App) addNote(ctx context.Context, version, title, detail string) error {
	_, err := a.db.Exec(ctx, "INSERT INTO version_notes (version, title, detail, published_at) VALUES ($1, $2, $3, $4)",
		version, title, detail, nowText())
	return err
}

func (a *App) seed(ctx context.Context) error {
	var users int
	if err := a.db.QueryRow(ctx, "SELECT count(1) FROM users").Scan(&users); err != nil {
		return err
	}
	if users == 0 {
		hash, salt := hashPassword("kaixin123")
		_, err := a.db.Exec(ctx, `INSERT INTO users
				(account, password_hash, password_salt, display_name, role, created_at)
			VALUES ('admin', $1, $2, '开心同学管理员', 'Owner', $3)`, hash, salt, nowText())
		if err != nil {
			return err
		}
	}

	items := []Software{
		{Name: "开心同学 AI 助手", Purpose: "Windows 桌面客户端安装包", Required: true, Notes: "客户端本体，下载需登录。"},
		{Name: "Microsoft Visual C++ 运行库", WingetID: "Microsoft.VCRedist.2015+.x64", Purpose: "多数本地 AI 组件依赖", Required: true, Notes: "客户端安装时自动修补。"},
		{Name: "Git", WingetID: "Git.Git", Purpose: "技能代码与资料版本管理"},
		{Name: "Python 3.12", WingetID: "Python.Python.3.12", Purpose: "本地技能运行环境"},
		{Name: "Ollama", WingetID: "Ollama.Ollama", Purpose: "本地大模型运行服务"},
		{Name: "7-Zip", WingetID: "7zip.7zip", Purpose: "资料压缩与解压"},
	}
	for _, item := range items {
		_, err := a.db.Exec(ctx, `INSERT INTO software (name, winget_id, purpose, required, notes, created_at)
			VALUES ($1, $2, $3, $4, $5, $6) ON CONFLICT (name) DO NOTHING`,
			item.Name, item.WingetID, item.Purpose, item.Required, item.Notes, nowText())
		if err != nil {
			return err
		}
	}
	return nil
}
