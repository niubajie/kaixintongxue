package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

// 资料目录约定：%USERPROFILE%\Documents\开心同学资料
// 不引入文件夹选择器依赖，用户把文件放进去或用「打开文件夹」按钮打开后拖入。
// 可用 KAIXIN_LIBRARY_DIR 覆盖，便于测试与他人环境。

const (
	libraryMaxItems = 2000
	libraryMaxDepth = 4
)

type LibraryItem struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	RelPath  string `json:"relPath"`
	Group    string `json:"group"`
	Type     string `json:"type"`
	Size     int64  `json:"size"`
	SizeText string `json:"sizeText"`
	ModTime  string `json:"modTime"`
	Date     string `json:"date"`
}

type LibraryScan struct {
	Root      string        `json:"root"`
	Exists    bool          `json:"exists"`
	Items     []LibraryItem `json:"items"`
	Total     int           `json:"total"`
	Groups    int           `json:"groups"`
	ThisWeek  int           `json:"thisWeek"`
	SizeBytes int64         `json:"sizeBytes"`
	SizeText  string        `json:"sizeText"`
	Truncated bool          `json:"truncated"`
	ScannedAt string        `json:"scannedAt"`
}

const libraryDirKey = "library.dir"

// defaultLibraryRoot：约定的默认目录。
func defaultLibraryRoot() string {
	if custom := os.Getenv("KAIXIN_LIBRARY_DIR"); custom != "" {
		return custom
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", "开心同学资料")
	}
	return filepath.Join(home, "Documents", "开心同学资料")
}

func formatBytes(bytes int64) string {
	const (
		kb = 1024
		mb = kb * 1024
		gb = mb * 1024
	)
	switch {
	case bytes >= gb:
		return fmt.Sprintf("%.1f GB", float64(bytes)/gb)
	case bytes >= mb:
		return fmt.Sprintf("%.1f MB", float64(bytes)/mb)
	case bytes >= kb:
		return fmt.Sprintf("%.0f KB", float64(bytes)/kb)
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

// resolveLibraryRoot：环境变量 > 本地库设置 > 默认约定目录。
func (a *App) resolveLibraryRoot() string {
	if custom := os.Getenv("KAIXIN_LIBRARY_DIR"); custom != "" {
		return custom
	}
	if a.store != nil {
		if value, err := a.store.GetSetting(context.Background(), libraryDirKey); err == nil && strings.TrimSpace(value) != "" {
			return value
		}
	}
	return defaultLibraryRoot()
}

// fileType 把扩展名转成界面上好认的类型名。
func fileType(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".pdf":
		return "PDF"
	case ".doc", ".docx":
		return "Word"
	case ".xls", ".xlsx", ".csv":
		return "Excel"
	case ".ppt", ".pptx":
		return "PPT"
	case ".md", ".markdown":
		return "Markdown"
	case ".txt":
		return "文本"
	case ".png", ".jpg", ".jpeg", ".webp", ".gif", ".bmp":
		return "图片"
	case ".mp4", ".mov", ".avi", ".mkv", ".webm":
		return "视频"
	case ".mp3", ".wav", ".m4a", ".flac":
		return "音频"
	case ".zip", ".7z", ".rar":
		return "压缩包"
	default:
		return "文件"
	}
}

// scanLibrary 遍历资料目录。只读，不修改任何文件。
func scanLibrary(root string) LibraryScan {
	scan := LibraryScan{
		Root:      root,
		Items:     []LibraryItem{},
		ScannedAt: time.Now().UTC().Format(time.RFC3339),
	}

	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return scan
	}
	scan.Exists = true

	groups := map[string]bool{}
	weekAgo := time.Now().AddDate(0, 0, -7)

	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() {
			if path == root {
				return nil
			}
			if strings.HasPrefix(entry.Name(), ".") {
				return filepath.SkipDir
			}
			rel, relErr := filepath.Rel(root, path)
			if relErr == nil && strings.Count(rel, string(filepath.Separator)) >= libraryMaxDepth {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(entry.Name(), ".") || strings.HasPrefix(entry.Name(), "~$") {
			return nil // 跳过隐藏文件与 Office 临时锁文件
		}
		if len(scan.Items) >= libraryMaxItems {
			scan.Truncated = true
			return filepath.SkipAll
		}
		fileInfo, statErr := entry.Info()
		if statErr != nil {
			return nil
		}

		rel, _ := filepath.Rel(root, path)
		parts := strings.Split(rel, string(filepath.Separator))
		group := "未分类"
		if len(parts) > 1 {
			group = parts[0]
		}
		groups[group] = true

		mod := fileInfo.ModTime()
		if mod.After(weekAgo) {
			scan.ThisWeek++
		}
		scan.SizeBytes += fileInfo.Size()
		scan.Items = append(scan.Items, LibraryItem{
			Name:     entry.Name(),
			Path:     path,
			RelPath:  rel,
			Group:    group,
			Type:     fileType(entry.Name()),
			Size:     fileInfo.Size(),
			SizeText: formatBytes(fileInfo.Size()),
			ModTime:  mod.UTC().Format(time.RFC3339),
			Date:     mod.Format("01-02"),
		})
		return nil
	})

	sort.Slice(scan.Items, func(i, j int) bool { return scan.Items[i].ModTime > scan.Items[j].ModTime })
	scan.Total = len(scan.Items)
	scan.Groups = len(groups)
	scan.SizeText = formatBytes(scan.SizeBytes)
	return scan
}

// ensureInside 防止通过接口打开资料目录以外的任意文件。
func ensureInside(root, target string) error {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(rootAbs, targetAbs)
	if err != nil {
		return err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return fmt.Errorf("只能打开资料目录内的文件")
	}
	return nil
}

// handleLibrarySetDir：把用户选中的目录记为资料库根目录。
func (a *App) handleLibrarySetDir(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Path string `json:"path"`
	}
	if err := decodeJSON(r, &request); err != nil || strings.TrimSpace(request.Path) == "" {
		fail(w, http.StatusBadRequest, "缺少目录路径")
		return
	}
	info, err := os.Stat(request.Path)
	if err != nil || !info.IsDir() {
		fail(w, http.StatusBadRequest, "目录不存在或不是文件夹")
		return
	}
	if a.store == nil {
		fail(w, http.StatusServiceUnavailable, "本地库未初始化，无法保存设置")
		return
	}
	if err := a.store.SetSetting(r.Context(), libraryDirKey, request.Path); err != nil {
		fail(w, http.StatusInternalServerError, "保存资料目录失败："+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, scanLibrary(request.Path))
}

func openPath(path string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", path).Start()
	case "darwin":
		return exec.Command("open", path).Start()
	default:
		return exec.Command("xdg-open", path).Start()
	}
}

func openFolder(path string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("explorer.exe", path).Start()
	case "darwin":
		return exec.Command("open", path).Start()
	default:
		return exec.Command("xdg-open", path).Start()
	}
}

// ---------- HTTP ----------

func (a *App) handleLibraryScan(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, scanLibrary(a.resolveLibraryRoot()))
}

func (a *App) handleLibraryOpen(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Path string `json:"path"`
	}
	if err := decodeJSON(r, &request); err != nil || strings.TrimSpace(request.Path) == "" {
		fail(w, http.StatusBadRequest, "缺少文件路径")
		return
	}
	root := a.resolveLibraryRoot()
	if err := ensureInside(root, request.Path); err != nil {
		fail(w, http.StatusForbidden, err.Error())
		return
	}
	if _, err := os.Stat(request.Path); err != nil {
		fail(w, http.StatusNotFound, "文件不存在或已被移动")
		return
	}
	if err := openPath(request.Path); err != nil {
		fail(w, http.StatusInternalServerError, "打开失败："+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"state": "opened", "path": request.Path})
}

func (a *App) handleLibraryOpenFolder(w http.ResponseWriter, r *http.Request) {
	root := a.resolveLibraryRoot()
	if err := os.MkdirAll(root, 0o755); err != nil {
		fail(w, http.StatusInternalServerError, "创建资料目录失败："+err.Error())
		return
	}
	if err := openFolder(root); err != nil {
		fail(w, http.StatusInternalServerError, "打开文件夹失败："+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"state": "opened", "root": root})
}
