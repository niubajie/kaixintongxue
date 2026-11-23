package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// FR-UPD-02/03：检查更新与下载安装包。
//
// 这一版做的是「下载 + SHA-256 校验 + 交给安装包」，
// 还没接代码签名与静默回滚——那需要发布流程与密钥，单独立项。

type UpdateAvailability struct {
	Configured bool   `json:"configured"`
	Available  bool   `json:"available"`
	Current    string `json:"current"`
	Latest     string `json:"latest"`
	FileName   string `json:"fileName"`
	SizeBytes  int64  `json:"sizeBytes"`
	SHA256     string `json:"sha256"`
	Notes      string `json:"notes"`
	Message    string `json:"message"`
}

type UpdateDownloadResult struct {
	OK        bool   `json:"ok"`
	Path      string `json:"path"`
	FileName  string `json:"fileName"`
	SizeBytes int64  `json:"sizeBytes"`
	Verified  bool   `json:"verified"`
	Message   string `json:"message"`
}

// compareVersion 比较语义化版本，返回 -1 / 0 / 1。
// 只按数字段比较，忽略预发布后缀，够用且不会误判。
func compareVersion(a, b string) int {
	parse := func(v string) []int {
		v = strings.TrimPrefix(strings.TrimSpace(v), "v")
		if idx := strings.IndexAny(v, "-+"); idx >= 0 {
			v = v[:idx]
		}
		parts := strings.Split(v, ".")
		out := make([]int, len(parts))
		for i, part := range parts {
			number := 0
			for _, r := range part {
				if r < '0' || r > '9' {
					break
				}
				number = number*10 + int(r-'0')
			}
			out[i] = number
		}
		return out
	}
	left, right := parse(a), parse(b)
	length := len(left)
	if len(right) > length {
		length = len(right)
	}
	for i := 0; i < length; i++ {
		var x, y int
		if i < len(left) {
			x = left[i]
		}
		if i < len(right) {
			y = right[i]
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}

func (a *App) handleUpdateAvailable(w http.ResponseWriter, r *http.Request) {
	base := a.currentServerURL(r.Context())
	if base == "" {
		writeJSON(w, http.StatusOK, UpdateAvailability{
			Configured: false,
			Message:    "未配置服务器，无法检查新版本",
		})
		return
	}
	current := strings.TrimSpace(r.URL.Query().Get("current"))

	body, status, _, err := httpJSON(r.Context(), http.MethodGet, base+"/api/client/latest", "", nil)
	if err != nil {
		writeJSON(w, http.StatusOK, UpdateAvailability{
			Configured: true, Current: current,
			Message: "连接更新服务失败：" + err.Error(),
		})
		return
	}
	if status == http.StatusNotFound {
		writeJSON(w, http.StatusOK, UpdateAvailability{
			Configured: true, Current: current,
			Message: "服务端尚未发布客户端版本",
		})
		return
	}
	if status != http.StatusOK {
		writeJSON(w, http.StatusOK, UpdateAvailability{
			Configured: true, Current: current,
			Message: fmt.Sprintf("更新服务返回 HTTP %d", status),
		})
		return
	}

	var release struct {
		Version   string `json:"version"`
		FileName  string `json:"fileName"`
		SizeBytes int64  `json:"sizeBytes"`
		SHA256    string `json:"sha256"`
		Notes     string `json:"notes"`
	}
	if err := json.Unmarshal(body, &release); err != nil {
		writeJSON(w, http.StatusOK, UpdateAvailability{
			Configured: true, Current: current,
			Message: "版本信息解析失败",
		})
		return
	}

	result := UpdateAvailability{
		Configured: true, Current: current,
		Latest: release.Version, FileName: release.FileName,
		SizeBytes: release.SizeBytes, SHA256: release.SHA256, Notes: release.Notes,
	}
	switch {
	case compareVersion(current, release.Version) < 0:
		result.Available = true
		result.Message = fmt.Sprintf("发现新版本 %s", release.Version)
	case compareVersion(current, release.Version) == 0:
		result.Message = "已是最新版本"
	default:
		result.Message = fmt.Sprintf("当前版本 %s 比服务端 %s 更新", current, release.Version)
	}
	writeJSON(w, http.StatusOK, result)
}

func (a *App) handleUpdateDownload(w http.ResponseWriter, r *http.Request) {
	base := a.currentServerURL(r.Context())
	if base == "" {
		fail(w, http.StatusBadRequest, "未配置服务器")
		return
	}
	var request struct {
		Version  string `json:"version"`
		FileName string `json:"fileName"`
		SHA256   string `json:"sha256"`
	}
	if err := decodeJSON(r, &request); err != nil || request.Version == "" {
		fail(w, http.StatusBadRequest, "缺少版本信息")
		return
	}
	if request.SHA256 == "" {
		fail(w, http.StatusBadRequest, "该版本缺少校验值，拒绝下载")
		return
	}

	target := filepath.Join(a.downloadDir, "client-"+request.Version+".bin")
	if err := os.MkdirAll(a.downloadDir, 0o755); err != nil {
		fail(w, http.StatusInternalServerError, "创建下载目录失败")
		return
	}

	url := fmt.Sprintf("%s/api/client/download?version=%s", base, request.Version)
	if err := a.downloadToFile(r.Context(), url, target); err != nil {
		fail(w, http.StatusBadGateway, "下载失败："+err.Error())
		return
	}

	sum, err := fileSHA256(target)
	if err != nil {
		fail(w, http.StatusInternalServerError, "读取下载文件失败")
		return
	}
	verified := strings.EqualFold(sum, request.SHA256)
	if !verified {
		// 校验不过就删掉，绝不留一个可能被装上去的坏包
		_ = os.Remove(target)
		fail(w, http.StatusConflict, "安装包校验失败（SHA-256 不匹配），已删除")
		return
	}

	info, _ := os.Stat(target)
	var size int64
	if info != nil {
		size = info.Size()
	}
	log.Printf("更新包已下载并校验：%s", target)
	writeJSON(w, http.StatusOK, UpdateDownloadResult{
		OK: true, Path: target, FileName: request.FileName, SizeBytes: size,
		Verified: true, Message: "下载完成并通过校验",
	})
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// downloadToFile 下载到本地文件，限制 500 MB，避免被超大响应撑爆磁盘。
func (a *App) downloadToFile(ctx context.Context, url, target string) error {
	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("服务器返回 HTTP %d", resp.StatusCode)
	}

	file, err := os.Create(target)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	_, err = io.Copy(file, io.LimitReader(resp.Body, 500<<20))
	return err
}

func (a *App) handleUpdateInstall(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Path string `json:"path"`
	}
	if err := decodeJSON(r, &request); err != nil || request.Path == "" {
		fail(w, http.StatusBadRequest, "缺少安装包路径")
		return
	}
	// 只允许运行下载目录里的文件，防止被当成任意程序启动器
	if ok, err := containedIn(a.downloadDir, request.Path); err != nil || !ok {
		fail(w, http.StatusForbidden, "只能运行下载目录内的安装包")
		return
	}
	if _, err := os.Stat(request.Path); err != nil {
		fail(w, http.StatusNotFound, "安装包不存在")
		return
	}
	if err := openPath(request.Path); err != nil {
		fail(w, http.StatusInternalServerError, "启动安装包失败："+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"message": "已启动安装包，请按提示完成升级；升级后重新打开应用生效。",
	})
}

var _ = sha256.Sum256
var _ = hex.EncodeToString
