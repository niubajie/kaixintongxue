package main

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite" // 纯 Go 驱动，不需要 CGO，交叉编译无负担
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// Store 是本地唯一的数据落点（%LOCALAPPDATA%\Kaixin\data\app.db）。
// 前端不直接碰这个文件，一律通过 sidecar 的 HTTP 接口读写。
type Store struct {
	db   *sql.DB
	path string
}

// OpenStore 打开（必要时创建）本地库并执行迁移。
func OpenStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("创建数据目录失败: %w", err)
	}
	path := filepath.Join(dir, "app.db")

	// 直接把 Windows 路径交给驱动。
	// 注意：不要写成 "file:C:/..."，那种 URI 形式在 Windows 上会被解析失败（unable to open database file）。
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("打开本地库失败: %w", err)
	}
	// SQLite 同一时刻只允许一个写者，连接数压到 1 最省心（本地单用户足够）。
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("本地库探活失败（路径 %s，目录可写=%v）: %w", path, dirWritable(filepath.Dir(path)), err)
	}

	// WAL：读写并发不互相阻塞；busy_timeout：写锁竞争时等待而不是直接报错。
	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA foreign_keys=ON",
	} {
		if _, err := db.ExecContext(ctx, pragma); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("设置 %s 失败: %w", pragma, err)
		}
	}

	if err := migrate(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Store{db: db, path: path}, nil
}

// dirWritable 用于排障：报告目录是否真的可写（Windows 上权限错误常被 SQLite 报成 14）。
func dirWritable(dir string) bool {
	probe := filepath.Join(dir, ".write-probe")
	if err := os.WriteFile(probe, []byte("ok"), 0o644); err != nil {
		return false
	}
	_ = os.Remove(probe)
	return true
}

// migrate 依次执行 migrations/*.sql，已执行过的版本记录在 schema_migrations 中。
func migrate(db *sql.DB) error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version    INTEGER PRIMARY KEY,
		applied_at TEXT NOT NULL
	)`); err != nil {
		return fmt.Errorf("创建迁移记录表失败: %w", err)
	}

	applied := map[int]bool{}
	rows, err := db.Query(`SELECT version FROM schema_migrations`)
	if err != nil {
		return fmt.Errorf("读取迁移记录失败: %w", err)
	}
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			rows.Close()
			return err
		}
		applied[v] = true
	}
	rows.Close()

	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("读取迁移脚本失败: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	for _, name := range names {
		var version int
		if _, err := fmt.Sscanf(name, "%04d", &version); err != nil {
			return fmt.Errorf("迁移文件名必须以 4 位版本号开头: %s", name)
		}
		if applied[version] {
			continue
		}
		script, err := migrationFS.ReadFile("migrations/" + name)
		if err != nil {
			return err
		}
		if _, err := db.Exec(string(script)); err != nil {
			return fmt.Errorf("执行迁移 %s 失败: %w", name, err)
		}
		if _, err := db.Exec(`INSERT INTO schema_migrations(version, applied_at) VALUES(?, ?)`,
			version, time.Now().UTC().Format(time.RFC3339)); err != nil {
			return fmt.Errorf("记录迁移 %s 失败: %w", name, err)
		}
	}
	return nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// ---------- 自定义技能 ----------

type CustomSkill struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	SoftwareIDs []string `json:"softwareIds"`
	SiteIDs     []string `json:"siteIds"`
	CreatedAt   string   `json:"createdAt"`
	UpdatedAt   string   `json:"updatedAt"`
}

func (s *Store) ListCustomSkills(ctx context.Context) ([]CustomSkill, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, description, software_ids, site_ids, created_at, updated_at
		   FROM custom_skills ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []CustomSkill{}
	for rows.Next() {
		var (
			item                 CustomSkill
			softwareJSON, siteJS string
		)
		if err := rows.Scan(&item.ID, &item.Name, &item.Description, &softwareJSON, &siteJS,
			&item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		item.SoftwareIDs = decodeStringArray(softwareJSON)
		item.SiteIDs = decodeStringArray(siteJS)
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) SaveCustomSkill(ctx context.Context, item CustomSkill) (CustomSkill, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	if item.ID == "" {
		item.ID = fmt.Sprintf("skill-%d", time.Now().UnixNano())
	}
	if item.Name == "" {
		return item, fmt.Errorf("技能名称不能为空")
	}
	if item.CreatedAt == "" {
		item.CreatedAt = now
	}
	item.UpdatedAt = now

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO custom_skills(id, name, description, software_ids, site_ids, created_at, updated_at)
		 VALUES(?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   name = excluded.name,
		   description = excluded.description,
		   software_ids = excluded.software_ids,
		   site_ids = excluded.site_ids,
		   updated_at = excluded.updated_at`,
		item.ID, item.Name, item.Description,
		encodeStringArray(item.SoftwareIDs), encodeStringArray(item.SiteIDs),
		item.CreatedAt, item.UpdatedAt)
	if err != nil {
		return item, err
	}
	return item, nil
}

func (s *Store) DeleteCustomSkill(ctx context.Context, id string) (bool, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM custom_skills WHERE id = ?`, id)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// ---------- 部署历史 ----------

type DeployRun struct {
	ID          string `json:"id"`
	State       string `json:"state"`
	StartedAt   string `json:"startedAt"`
	FinishedAt  string `json:"finishedAt"`
	TotalSizeMb int    `json:"totalSizeMb"`
	LogPath     string `json:"logPath"`
	Attempt     int    `json:"attempt"`
	Error       string `json:"error"`
	StepsJSON   string `json:"-"`
}

const deployLease = 90 * time.Second

// StartDeployRun 在执行前就落一条 running 记录并领取租约。
// 进程崩溃时租约到期，由 SweepStaleDeployRuns 回收，界面不会永远停在「进行中」。
func (s *Store) StartDeployRun(ctx context.Context, run DeployRun) error {
	now := time.Now().UTC()
	leaseUntil := now.Add(deployLease).Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO deploy_runs(id, state, started_at, finished_at, total_size_mb, log_path, steps_json,
		                          attempt, heartbeat_at, lease_until, error)
		 VALUES(?, 'running', ?, '', ?, ?, ?, 1, ?, ?, '')
		 ON CONFLICT(id) DO UPDATE SET
		   state = 'running', finished_at = '', attempt = deploy_runs.attempt + 1,
		   heartbeat_at = excluded.heartbeat_at, lease_until = excluded.lease_until, error = ''`,
		run.ID, run.StartedAt, run.TotalSizeMb, run.LogPath, run.StepsJSON,
		now.Format(time.RFC3339), leaseUntil)
	return err
}

// HeartbeatDeployRun 续租；返回 false 表示这条记录已不存在或被回收。
func (s *Store) HeartbeatDeployRun(ctx context.Context, id string) (bool, error) {
	now := time.Now().UTC()
	leaseUntil := now.Add(deployLease).Format(time.RFC3339)
	res, err := s.db.ExecContext(ctx,
		`UPDATE deploy_runs SET heartbeat_at = ?, lease_until = ? WHERE id = ? AND state = 'running'`,
		now.Format(time.RFC3339), leaseUntil, id)
	if err != nil {
		return false, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

// SweepStaleDeployRuns 把租约过期的 running 记录标记为中断。
// 与 Rakazo 的 reconciler 一样，回收的是「没人再续租」的任务，而不是靠进程内状态。
func (s *Store) SweepStaleDeployRuns(ctx context.Context) (int64, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.ExecContext(ctx,
		`UPDATE deploy_runs
		    SET state = 'failed',
		        finished_at = ?,
		        error = '任务中断：进程退出或崩溃，未收到结束信号'
		  WHERE state = 'running' AND lease_until != '' AND lease_until < ?`,
		now, now)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *Store) RecordDeployRun(ctx context.Context, run DeployRun) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO deploy_runs(id, state, started_at, finished_at, total_size_mb, log_path, steps_json, error)
		 VALUES(?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   state = excluded.state,
		   finished_at = excluded.finished_at,
		   total_size_mb = excluded.total_size_mb,
		   steps_json = excluded.steps_json,
		   error = excluded.error`,
		run.ID, run.State, run.StartedAt, run.FinishedAt, run.TotalSizeMb, run.LogPath, run.StepsJSON, run.Error)
	return err
}

// LoadDeployRun 读取单条运行记录（含步骤快照），续装与单步重试要用。
func (s *Store) LoadDeployRun(ctx context.Context, id string) (DeployRun, error) {
	var run DeployRun
	err := s.db.QueryRowContext(ctx,
		`SELECT id, state, started_at, COALESCE(finished_at,''), total_size_mb, log_path,
		        steps_json, COALESCE(attempt, 1), COALESCE(error, '')
		   FROM deploy_runs WHERE id = ?`, id).
		Scan(&run.ID, &run.State, &run.StartedAt, &run.FinishedAt, &run.TotalSizeMb,
			&run.LogPath, &run.StepsJSON, &run.Attempt, &run.Error)
	return run, err
}

// SaveDeploySteps 每完成一步就落一次快照，崩溃后能知道做到哪一步。
func (s *Store) SaveDeploySteps(ctx context.Context, id, stepsJSON, state, logPath string) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx,
		`UPDATE deploy_runs
		    SET steps_json = ?, state = ?, log_path = ?, heartbeat_at = ?, lease_until = ?
		  WHERE id = ?`,
		stepsJSON, state, logPath, now.Format(time.RFC3339),
		now.Add(deployLease).Format(time.RFC3339), id)
	return err
}

func (s *Store) RecentDeployRuns(ctx context.Context, limit int) ([]DeployRun, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, state, started_at, COALESCE(finished_at,''), total_size_mb, log_path,
		        COALESCE(attempt, 1), COALESCE(error, ''), COALESCE(steps_json, '[]')
		   FROM deploy_runs ORDER BY started_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []DeployRun{}
	for rows.Next() {
		var r DeployRun
		if err := rows.Scan(&r.ID, &r.State, &r.StartedAt, &r.FinishedAt, &r.TotalSizeMb, &r.LogPath, &r.Attempt, &r.Error, &r.StepsJSON); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ---------- 键值设置 ----------

func (s *Store) GetSetting(ctx context.Context, key string) (string, error) {
	var value string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&value)
	if err != nil {
		return "", err
	}
	return value, nil
}

func (s *Store) SetSetting(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO settings(key, value, updated_at) VALUES(?, ?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		key, value, time.Now().UTC().Format(time.RFC3339))
	return err
}

// ---------- 统计 ----------

type StoreStats struct {
	Path          string `json:"path"`
	SizeKb        int64  `json:"sizeKb"`
	CustomSkills  int    `json:"customSkills"`
	DeployRuns    int    `json:"deployRuns"`
	SchemaVersion int    `json:"schemaVersion"`
}

func (s *Store) Stats(ctx context.Context) StoreStats {
	stats := StoreStats{Path: s.path}
	if info, err := os.Stat(s.path); err == nil {
		stats.SizeKb = info.Size() / 1024
	}
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM custom_skills`).Scan(&stats.CustomSkills)
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM deploy_runs`).Scan(&stats.DeployRuns)
	_ = s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version),0) FROM schema_migrations`).Scan(&stats.SchemaVersion)
	return stats
}

// ---------- JSON 数组编解码（避免引入额外依赖） ----------

func encodeStringArray(items []string) string {
	if len(items) == 0 {
		return "[]"
	}
	var b strings.Builder
	b.WriteByte('[')
	for i, item := range items {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteByte('"')
		for _, r := range item {
			switch r {
			case '"':
				b.WriteString(`\"`)
			case '\\':
				b.WriteString(`\\`)
			default:
				b.WriteRune(r)
			}
		}
		b.WriteByte('"')
	}
	b.WriteByte(']')
	return b.String()
}

func decodeStringArray(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "[]" {
		return []string{}
	}
	raw = strings.TrimPrefix(raw, "[")
	raw = strings.TrimSuffix(raw, "]")
	parts := strings.Split(raw, `","`)
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.Trim(part, `"`)
		part = strings.ReplaceAll(part, `\"`, `"`)
		part = strings.ReplaceAll(part, `\\`, `\`)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

// ---------- HTTP ----------

func (a *App) handleDataInfo(w http.ResponseWriter, r *http.Request) {
	if a.store == nil {
		fail(w, http.StatusServiceUnavailable, "本地库未初始化")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"store": a.store.Stats(r.Context())})
}

func (a *App) handleListCustomSkills(w http.ResponseWriter, r *http.Request) {
	if a.store == nil {
		fail(w, http.StatusServiceUnavailable, "本地库未初始化")
		return
	}
	items, err := a.store.ListCustomSkills(r.Context())
	if err != nil {
		fail(w, http.StatusInternalServerError, "读取自定义技能失败: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"skills": items})
}

func (a *App) handleSaveCustomSkill(w http.ResponseWriter, r *http.Request) {
	if a.store == nil {
		fail(w, http.StatusServiceUnavailable, "本地库未初始化")
		return
	}
	var item CustomSkill
	if err := decodeJSON(r, &item); err != nil {
		fail(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	saved, err := a.store.SaveCustomSkill(r.Context(), item)
	if err != nil {
		fail(w, http.StatusBadRequest, "保存自定义技能失败: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"skill": saved})
}

func (a *App) handleDeleteCustomSkill(w http.ResponseWriter, r *http.Request) {
	if a.store == nil {
		fail(w, http.StatusServiceUnavailable, "本地库未初始化")
		return
	}
	id := r.PathValue("id")
	if id == "" {
		fail(w, http.StatusBadRequest, "缺少技能 id")
		return
	}
	ok, err := a.store.DeleteCustomSkill(r.Context(), id)
	if err != nil {
		fail(w, http.StatusInternalServerError, "删除失败: "+err.Error())
		return
	}
	if !ok {
		fail(w, http.StatusNotFound, "技能不存在")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": id})
}

func (a *App) handleDeployHistory(w http.ResponseWriter, r *http.Request) {
	if a.store == nil {
		fail(w, http.StatusServiceUnavailable, "本地库未初始化")
		return
	}
	// 列历史前先回收过期租约，保证界面看到的状态是可信的
	_, _ = a.store.SweepStaleDeployRuns(r.Context())
	runs, err := a.store.RecentDeployRuns(r.Context(), 20)
	if err != nil {
		fail(w, http.StatusInternalServerError, "读取部署历史失败: "+err.Error())
		return
	}
	// 附带「能否续装 / 失败在第几步」，前端据此显示继续与单步重试按钮
	type runView struct {
		DeployRun
		Resumable   bool `json:"resumable"`
		FailedOrder int  `json:"failedOrder"`
	}
	views := make([]runView, 0, len(runs))
	for _, run := range runs {
		view := runView{DeployRun: run}
		var steps []DeployStepStatus
		if run.StepsJSON != "" && json.Unmarshal([]byte(run.StepsJSON), &steps) == nil {
			pending := 0
			for _, step := range steps {
				if step.Status == "done" {
					continue
				}
				pending++
				if step.Status == "failed" && view.FailedOrder == 0 {
					view.FailedOrder = step.Order
				}
			}
			view.Resumable = pending > 0 && run.State != "running"
		}
		views = append(views, view)
	}
	writeJSON(w, http.StatusOK, map[string]any{"runs": views})
}
