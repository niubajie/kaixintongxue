package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// 数据位置：数据库放在哪块硬盘，装的时候要告诉用户，
// 状态栏的「硬盘剩余」也必须跟着这块盘走（同一个来源，不能各算各的）。

const dataLocationFile = "data-location.txt"

func dataLocationPath(root string) string {
	return filepath.Join(root, dataLocationFile)
}

// resolveDataDir 决定数据库目录：
// 环境变量 > 用户选过的位置（data-location.txt）> 默认 root\data
func resolveDataDir(root string) (string, bool) {
	if custom := os.Getenv("KAIXIN_DATA_DIR"); custom != "" {
		return custom, false
	}
	if raw, err := os.ReadFile(dataLocationPath(root)); err == nil {
		chosen := strings.TrimSpace(string(raw))
		if chosen != "" {
			return chosen, false
		}
	}
	return filepath.Join(root, "data"), true // 第二个返回值表示「首次运行」
}

// writeDataLocation 记住用户选的目录；失败时只影响下次启动，不影响本次运行。
func writeDataLocation(root, dir string) error {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	return os.WriteFile(dataLocationPath(root), []byte(dir), 0o600)
}

// moveDataDir 把数据库搬到新目录。先复制再切配置，失败不动原文件。
func moveDataDir(ctx context.Context, from, to string) error {
	if err := os.MkdirAll(to, 0o755); err != nil {
		return fmt.Errorf("创建目标目录失败：%w", err)
	}
	// 目标目录必须可写
	probe := filepath.Join(to, ".write-test")
	if err := os.WriteFile(probe, []byte("ok"), 0o600); err != nil {
		return fmt.Errorf("目标目录不可写：%w", err)
	}
	_ = os.Remove(probe)

	for _, name := range []string{"app.db", "app.db-wal", "app.db-shm"} {
		src := filepath.Join(from, name)
		if _, err := os.Stat(src); err != nil {
			continue // wal/shm 可能不存在
		}
		dst := filepath.Join(to, name)
		if err := copyFile(src, dst); err != nil {
			return fmt.Errorf("复制 %s 失败：%w", name, err)
		}
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

// 首次运行标记：本进程内只提示一次
var firstRunPending bool

type dataLocationInfo struct {
	Path        string  `json:"path"`
	DBPath      string  `json:"dbPath"`
	Drive       string  `json:"drive"`
	FreeGb      float64 `json:"freeGb"`
	TotalGb     float64 `json:"totalGb"`
	DBSizeBytes int64   `json:"dbSizeBytes"`
	FirstRun    bool    `json:"firstRun"`
	Message     string  `json:"message"`
}

func (a *App) buildDataLocationInfo() dataLocationInfo {
	info := dataLocationInfo{
		Path:     a.dataDir,
		DBPath:   filepath.Join(a.dataDir, "app.db"),
		Drive:    driveOf(a.dataDir),
		FirstRun: firstRunPending,
	}
	info.FreeGb, info.TotalGb = diskUsage(info.Drive)
	if stat, err := os.Stat(info.DBPath); err == nil {
		info.DBSizeBytes = stat.Size()
	}
	if info.FirstRun {
		info.Message = fmt.Sprintf("数据将保存在 %s 盘（%s），该盘剩余 %.0f GB。可在「系统设置 → 数据位置」更换到其他硬盘。",
			strings.TrimSuffix(info.Drive, ":"), a.dataDir, info.FreeGb)
	} else {
		info.Message = fmt.Sprintf("数据保存在 %s 盘，该盘剩余 %.0f GB。", strings.TrimSuffix(info.Drive, ":"), info.FreeGb)
	}
	return info
}

func (a *App) handleDataLocationInfo(w http.ResponseWriter, r *http.Request) {
	if a.store == nil {
		fail(w, http.StatusServiceUnavailable, "本地库未初始化")
		return
	}
	payload := map[string]any{
		"store":    a.store.Stats(r.Context()),
		"location": a.buildDataLocationInfo(),
	}
	writeJSON(w, http.StatusOK, payload)
}

// handleDataAck 首次启动提示关掉后调用，之后不再重复提示。
func (a *App) handleDataAck(w http.ResponseWriter, r *http.Request) {
	firstRunPending = false
	writeJSON(w, http.StatusOK, map[string]any{"message": "已确认数据位置"})
}

// handleDataMove 把数据库搬到用户选的目录，成功后需要重启应用生效。
func (a *App) handleDataMove(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Dir string `json:"dir"`
	}
	if err := decodeJSON(r, &request); err != nil || strings.TrimSpace(request.Dir) == "" {
		fail(w, http.StatusBadRequest, "缺少目标目录")
		return
	}
	target := strings.TrimSpace(request.Dir)
	if ok, err := containedIn(filepath.Dir(a.dataDir), target); err == nil && ok && target == a.dataDir {
		fail(w, http.StatusBadRequest, "目标目录与当前目录相同")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	if err := moveDataDir(ctx, a.dataDir, target); err != nil {
		fail(w, http.StatusInternalServerError, "迁移数据失败："+err.Error())
		return
	}
	if err := writeDataLocation(a.root, target); err != nil {
		fail(w, http.StatusInternalServerError, "记录新位置失败："+err.Error())
		return
	}
	log.Printf("数据目录已迁移到 %s，重启后生效", target)
	writeJSON(w, http.StatusOK, map[string]any{
		"message":      "已复制到新位置，重启应用后生效；确认无误后可手动删除旧目录。",
		"target":       target,
		"needsRestart": true,
	})
}
