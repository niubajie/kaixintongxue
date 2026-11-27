package main

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
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
	SchemaVersion int    `json:"schemaVersion"`
}

func (s *Store) Stats(ctx context.Context) StoreStats {
	stats := StoreStats{Path: s.path}
	if info, err := os.Stat(s.path); err == nil {
		stats.SizeKb = info.Size() / 1024
	}
	_ = s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version),0) FROM schema_migrations`).Scan(&stats.SchemaVersion)
	return stats
}
