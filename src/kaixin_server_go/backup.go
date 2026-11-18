package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// R6 备份与恢复（SYS-02 的最后一块）。
//
// 不依赖 pg_dump：直接把业务表导出成 JSON 快照，恢复时用
// json_populate_recordset 整表回灌。这样在没有客户端工具的机器上也能跑。
//
// 权限（见 docs/15）：Owner 与 Admin 可触发与查看；恢复与下载只给 Owner。

// 备份的表，按外键依赖排序：恢复时先插前面的，删除时反着来。
var backupTables = []string{"users", "software", "software_versions", "version_notes", "audit"}

type backupRecord struct {
	ID        int64  `json:"id"`
	FileName  string `json:"fileName"`
	SizeBytes int64  `json:"sizeBytes"`
	CreatedAt string `json:"createdAt"`
	Actor     string `json:"actor"`
	Tables    int    `json:"tables"`
}

func (a *App) backupDir() string {
	return filepath.Join(a.cfg.DataRoot, "backups")
}

func (a *App) ensureBackupTable(ctx context.Context) error {
	_, err := a.db.Exec(ctx, `CREATE TABLE IF NOT EXISTS backups (
		id BIGSERIAL PRIMARY KEY,
		file_name TEXT NOT NULL,
		size_bytes BIGINT NOT NULL DEFAULT 0,
		created_at TEXT NOT NULL,
		actor TEXT NOT NULL DEFAULT '',
		tables INTEGER NOT NULL DEFAULT 0
	)`)
	return err
}

// createBackup 导出业务表为 JSON 文件并登记。
func (a *App) createBackup(ctx context.Context, actor string) (backupRecord, error) {
	if err := a.ensureBackupTable(ctx); err != nil {
		return backupRecord{}, err
	}
	if err := os.MkdirAll(a.backupDir(), 0o755); err != nil {
		return backupRecord{}, fmt.Errorf("创建备份目录失败：%w", err)
	}

	snapshot := map[string]json.RawMessage{}
	for _, table := range backupTables {
		var raw []byte
		// json_agg 对空表返回 NULL，用 coalesce 兜成空数组
		query := fmt.Sprintf(`SELECT COALESCE(json_agg(t), '[]'::json)::text FROM %s t`, table)
		if err := a.db.QueryRow(ctx, query).Scan(&raw); err != nil {
			return backupRecord{}, fmt.Errorf("导出 %s 失败：%w", table, err)
		}
		snapshot[table] = json.RawMessage(raw)
	}

	payload := map[string]any{
		"version":   1,
		"createdAt": time.Now().UTC().Format(time.RFC3339),
		"actor":     actor,
		"tables":    snapshot,
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return backupRecord{}, err
	}

	name := "backup-" + time.Now().Format("20060102-150405") + ".json"
	path := filepath.Join(a.backupDir(), name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return backupRecord{}, fmt.Errorf("写入备份文件失败：%w", err)
	}

	var id int64
	createdAt := time.Now().UTC().Format(time.RFC3339)
	if err := a.db.QueryRow(ctx,
		`INSERT INTO backups(file_name, size_bytes, created_at, actor, tables)
		 VALUES($1, $2, $3, $4, $5) RETURNING id`,
		name, int64(len(data)), createdAt, actor, len(backupTables)).Scan(&id); err != nil {
		return backupRecord{}, err
	}

	return backupRecord{
		ID: id, FileName: name, SizeBytes: int64(len(data)),
		CreatedAt: createdAt, Actor: actor, Tables: len(backupTables),
	}, nil
}

func (a *App) listBackups(ctx context.Context, limit int) ([]backupRecord, error) {
	if err := a.ensureBackupTable(ctx); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := a.db.Query(ctx,
		`SELECT id, file_name, size_bytes, created_at, actor, tables
		   FROM backups ORDER BY id DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []backupRecord{}
	for rows.Next() {
		var item backupRecord
		if err := rows.Scan(&item.ID, &item.FileName, &item.SizeBytes, &item.CreatedAt,
			&item.Actor, &item.Tables); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (a *App) findBackup(ctx context.Context, id int64) (backupRecord, error) {
	if err := a.ensureBackupTable(ctx); err != nil {
		return backupRecord{}, err
	}
	var item backupRecord
	err := a.db.QueryRow(ctx,
		`SELECT id, file_name, size_bytes, created_at, actor, tables FROM backups WHERE id = $1`, id).
		Scan(&item.ID, &item.FileName, &item.SizeBytes, &item.CreatedAt, &item.Actor, &item.Tables)
	return item, err
}

// restoreBackup 用备份文件整表回灌。先删后插，全程一个事务。
func (a *App) restoreBackup(ctx context.Context, id int64) (int, error) {
	record, err := a.findBackup(ctx, id)
	if err != nil {
		return 0, fmt.Errorf("找不到备份记录")
	}
	path := filepath.Join(a.backupDir(), record.FileName)
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("备份文件不存在或无法读取")
	}

	var payload struct {
		Tables map[string]json.RawMessage `json:"tables"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return 0, fmt.Errorf("备份文件格式不正确")
	}

	tx, err := a.db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// 反序删除，避免外键冲突
	for i := len(backupTables) - 1; i >= 0; i-- {
		if _, err := tx.Exec(ctx, "DELETE FROM "+backupTables[i]); err != nil {
			return 0, fmt.Errorf("清空 %s 失败：%w", backupTables[i], err)
		}
	}
	// 正序插入
	restored := 0
	for _, table := range backupTables {
		body, ok := payload.Tables[table]
		if !ok || len(body) == 0 {
			continue
		}
		if _, err := tx.Exec(ctx,
			fmt.Sprintf(`INSERT INTO %s SELECT * FROM json_populate_recordset(NULL::%s, $1::json)`, table, table),
			string(body)); err != nil {
			return 0, fmt.Errorf("恢复 %s 失败：%w", table, err)
		}
		restored++
	}
	// 重置自增序列，否则新插入会撞主键
	for _, table := range backupTables {
		_, _ = tx.Exec(ctx,
			fmt.Sprintf(`SELECT setval(pg_get_serial_sequence('%s','id'), COALESCE((SELECT MAX(id) FROM %s), 1))`,
				table, table))
	}

	if err := tx.Commit(ctx); err != nil {
		return restored, err
	}
	return restored, nil
}

// ---------- HTTP ----------

func (a *App) handleBackupCreate(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.requireLogin(w, r, "admin.backups.create")
	if !ok {
		return
	}
	if !canTriggerBackup(actor) {
		a.denyWithAudit(w, r, actor, "admin.backups.create", "", reasonNotAdmin)
		return
	}
	record, err := a.createBackup(r.Context(), actor.Account)
	if err != nil {
		fail(w, http.StatusInternalServerError, "备份失败："+err.Error())
		return
	}
	a.audit(r.Context(), actor.Account, "backup_create", record.FileName)
	writeJSON(w, http.StatusOK, record)
}

func (a *App) handleBackupList(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.requireLogin(w, r, "admin.backups.list")
	if !ok {
		return
	}
	if !canTriggerBackup(actor) {
		a.denyWithAudit(w, r, actor, "admin.backups.list", "", reasonNotAdmin)
		return
	}
	items, err := a.listBackups(r.Context(), 50)
	if err != nil {
		fail(w, http.StatusInternalServerError, "读取备份列表失败。")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (a *App) handleBackupRestore(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.requireLogin(w, r, "admin.backups.restore")
	if !ok {
		return
	}
	if !canRestoreBackup(actor) {
		a.denyWithAudit(w, r, actor, "admin.backups.restore", "", reasonOwnerOnly)
		return
	}
	id, valid := parseInt64(r.PathValue("id"))
	if !valid {
		fail(w, http.StatusBadRequest, "备份不存在。")
		return
	}
	restored, err := a.restoreBackup(r.Context(), id)
	if err != nil {
		fail(w, http.StatusInternalServerError, "恢复失败："+err.Error())
		return
	}
	a.audit(r.Context(), actor.Account, "backup_restore", fmt.Sprintf("id=%d tables=%d", id, restored))
	writeMessage(w, http.StatusOK, fmt.Sprintf("已恢复 %d 张表。", restored))
}

func (a *App) handleBackupDownload(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.requireLogin(w, r, "admin.backups.download")
	if !ok {
		return
	}
	if !canRestoreBackup(actor) {
		a.denyWithAudit(w, r, actor, "admin.backups.download", "", reasonOwnerOnly)
		return
	}
	id, valid := parseInt64(r.PathValue("id"))
	if !valid {
		fail(w, http.StatusBadRequest, "备份不存在。")
		return
	}
	record, err := a.findBackup(r.Context(), id)
	if err != nil {
		fail(w, http.StatusNotFound, "备份不存在。")
		return
	}
	path := filepath.Join(a.backupDir(), record.FileName)
	if _, err := os.Stat(path); err != nil {
		fail(w, http.StatusNotFound, "备份文件已丢失。")
		return
	}
	a.audit(r.Context(), actor.Account, "backup_download", record.FileName)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="`+record.FileName+`"`)
	http.ServeFile(w, r, path)
}

// pruneBackups 只保留最近 20 份，避免磁盘被撑满（尽力而为，失败不影响主流程）。
func (a *App) pruneBackups(ctx context.Context) {
	items, err := a.listBackups(ctx, 200)
	if err != nil || len(items) <= 20 {
		return
	}
	old := items[20:]
	sort.Slice(old, func(i, j int) bool { return old[i].ID < old[j].ID })
	for _, item := range old {
		_ = os.Remove(filepath.Join(a.backupDir(), item.FileName))
		_, _ = a.db.Exec(ctx, "DELETE FROM backups WHERE id = $1", item.ID)
	}
}

var _ = strings.TrimSpace
