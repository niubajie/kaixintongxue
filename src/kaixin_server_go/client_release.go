package main

import (
	"net/http"
	"time"
)

// 客户端更新：从软件目录里取「开心同学 AI 助手」的当前版本，
// 供客户端检查更新与下载。没有发布版本时明确返回 404，不给假数据。

const clientSoftwareName = "开心同学 AI 助手"

type clientRelease struct {
	Version     string `json:"version"`
	FileName    string `json:"fileName"`
	SizeBytes   int64  `json:"sizeBytes"`
	SHA256      string `json:"sha256"`
	PublishedAt string `json:"publishedAt"`
	Notes       string `json:"notes"`
	DownloadURL string `json:"downloadUrl"`
}

func (a *App) handleClientLatest(w http.ResponseWriter, r *http.Request) {
	var release clientRelease
	err := a.db.QueryRow(r.Context(), `
		SELECT v.version, v.file_name, v.file_size, v.sha256, v.published_at, v.release_notes
		  FROM software s
		  JOIN software_versions v ON v.software_id = s.id
		 WHERE s.name = $1 AND v.is_current = TRUE
		 LIMIT 1`, clientSoftwareName).
		Scan(&release.Version, &release.FileName, &release.SizeBytes,
			&release.SHA256, &release.PublishedAt, &release.Notes)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{
			"message": "尚未发布客户端版本。",
			"code":    40401,
		})
		return
	}
	if release.SHA256 == "" {
		// 没有校验值就不让客户端下载，避免装到坏包
		writeJSON(w, http.StatusConflict, map[string]any{
			"message": "该版本缺少校验值，已拒绝提供下载。",
			"code":    40901,
		})
		return
	}
	release.DownloadURL = "/api/client/download?version=" + release.Version
	release.PublishedAt = normalizeTime(release.PublishedAt)
	writeJSON(w, http.StatusOK, release)
}

// handleClientDownload 直接下发安装包文件。
func (a *App) handleClientDownload(w http.ResponseWriter, r *http.Request) {
	version := r.URL.Query().Get("version")
	if version == "" {
		fail(w, http.StatusBadRequest, "缺少版本号。")
		return
	}
	var path, fileName, sha string
	err := a.db.QueryRow(r.Context(), `
		SELECT v.file_path, v.file_name, v.sha256
		  FROM software s
		  JOIN software_versions v ON v.software_id = s.id
		 WHERE s.name = $1 AND v.version = $2
		 LIMIT 1`, clientSoftwareName, version).Scan(&path, &fileName, &sha)
	if err != nil {
		fail(w, http.StatusNotFound, "找不到该版本。")
		return
	}
	if sha == "" {
		fail(w, http.StatusConflict, "该版本缺少校验值，已拒绝下载。")
		return
	}
	a.audit(r.Context(), "anonymous", "client_download", fileName)
	w.Header().Set("Content-Disposition", `attachment; filename="`+fileName+`"`)
	http.ServeFile(w, r, path)
}

// normalizeTime 兼容服务端两种时间写法，输出 RFC3339。
func normalizeTime(value string) string {
	if value == "" {
		return ""
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.UTC().Format(time.RFC3339)
		}
	}
	return value
}
