package main

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type DeployStepStatus struct {
	Order             int    `json:"order"`
	SoftwareID        string `json:"softwareId"`
	Name              string `json:"name"`
	Action            string `json:"action"`
	Status            string `json:"status"` // pending/downloading/verifying/extracting/installing/verifying-install/done/failed/skipped
	Message           string `json:"message"`
	HashVerified      *bool  `json:"hashVerified,omitempty"`
	SizeMb            int    `json:"sizeMb"`
	TotalBytes        int64  `json:"totalBytes,omitempty"`
	DownloadedBytes   int64  `json:"downloadedBytes,omitempty"`
	BytesPerSecond    int64  `json:"bytesPerSecond,omitempty"`
	DownloadStartedAt string `json:"downloadStartedAt,omitempty"`
	SiteID            string `json:"siteId,omitempty"`
	LoginURL          string `json:"loginUrl,omitempty"`
}

type DeployJob struct {
	ID         string              `json:"id"`
	State      string              `json:"state"` // running/done/failed
	StartedAt  string              `json:"startedAt"`
	FinishedAt string              `json:"finishedAt,omitempty"`
	LogPath    string              `json:"logPath"`
	Steps      []*DeployStepStatus `json:"steps"`
}

type deployer struct {
	mu      sync.Mutex
	job     *DeployJob
	running bool
	logFile *os.File
	// 当前部署任务的上下文：用户点「取消」时 cancel，长耗时步骤据此退出。
	ctx    context.Context
	cancel context.CancelFunc
}

// errCancelled 用于把「用户取消」与「执行失败」区分开。
var errCancelled = errors.New("任务已取消")

// cancelled 判断当前任务是否已被取消。
func (a *App) cancelled() bool {
	a.deploy.mu.Lock()
	defer a.deploy.mu.Unlock()
	return a.deploy.ctx != nil && a.deploy.ctx.Err() != nil
}

// deployContext 返回当前任务上下文，未启动过任务时退化为 Background。
func (a *App) deployContext() context.Context {
	a.deploy.mu.Lock()
	defer a.deploy.mu.Unlock()
	if a.deploy.ctx != nil {
		return a.deploy.ctx
	}
	return context.Background()
}

func (a *App) handleDeployExecute(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Steps     []PlanStep     `json:"steps"`
		AuthSteps []PlanSiteAuth `json:"authSteps"`
		Force     bool           `json:"force"`
	}
	if err := decodeJSON(r, &request); err != nil {
		fail(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	if len(request.Steps) == 0 {
		fail(w, http.StatusBadRequest, "方案为空，请先生成技能方案。")
		return
	}
	a.deploy.mu.Lock()
	if a.deploy.running {
		a.deploy.mu.Unlock()
		fail(w, http.StatusConflict, "已有部署任务正在进行，请等待完成。")
		return
	}
	job := &DeployJob{
		ID:        time.Now().Format("20060102-150405"),
		State:     "running",
		StartedAt: time.Now().Format(time.RFC3339),
		LogPath:   filepath.Join(a.logDir, "deploy-"+time.Now().Format("20060102-150405")+".log"),
	}
	for _, step := range request.Steps {
		entry := findSoftware(a.catalog, step.SoftwareID)
		if entry == nil {
			a.deploy.mu.Unlock()
			fail(w, http.StatusBadRequest, "未知软件："+step.SoftwareID)
			return
		}
		sizeMb := step.SizeMb
		if sizeMb <= 0 {
			sizeMb = entry.SizeMb
		}
		// 预填下载体积，让排队中的步骤也能显示文件大小与预计时长。
		totalBytes := int64(0)
		if entry.Install.Type != "manual" {
			totalBytes = int64(sizeMb) * 1024 * 1024
		}
		job.Steps = append(job.Steps, &DeployStepStatus{
			Order: step.Order, SoftwareID: entry.ID, Name: entry.Name, Action: step.Action, Status: "pending",
			SizeMb: sizeMb, TotalBytes: totalBytes,
		})
	}
	// 网站授权排在软件安装之后：未授权的会弹登录框，已授权的自动跳过。
	for _, auth := range request.AuthSteps {
		job.Steps = append(job.Steps, &DeployStepStatus{
			Order: len(job.Steps) + 1, SoftwareID: "site-" + auth.SiteID, Name: auth.Name,
			Action: "authorize", Status: "pending", SiteID: auth.SiteID, LoginURL: auth.LoginURL, Message: auth.Note,
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.deploy.job = job
	a.deploy.running = true
	a.deploy.ctx = ctx
	a.deploy.cancel = cancel
	a.deploy.mu.Unlock()

	go a.runDeploy(job, request.Force)
	writeJSON(w, http.StatusAccepted, map[string]any{"jobId": job.ID, "state": job.State})
}

// handleDeployCancel：请求取消当前部署任务。
// 只是发出取消信号，真正的清理由 runDeploy 在各步骤边界完成。
func (a *App) handleDeployCancel(w http.ResponseWriter, r *http.Request) {
	a.deploy.mu.Lock()
	running := a.deploy.running
	cancel := a.deploy.cancel
	a.deploy.mu.Unlock()

	if !running || cancel == nil {
		fail(w, http.StatusConflict, "当前没有正在进行的部署任务")
		return
	}
	cancel()
	a.deployLog("收到取消请求，正在停止当前任务")
	writeJSON(w, http.StatusAccepted, map[string]any{"state": "cancelling"})
}

func (a *App) handleDeployStatus(w http.ResponseWriter, r *http.Request) {
	a.deploy.mu.Lock()
	defer a.deploy.mu.Unlock()
	if a.deploy.job == nil {
		writeJSON(w, http.StatusOK, map[string]any{"state": "idle"})
		return
	}
	writeJSON(w, http.StatusOK, a.deploy.job)
}

func (a *App) deployLog(format string, args ...any) {
	line := time.Now().Format("15:04:05 ") + fmt.Sprintf(format, args...)
	if a.deploy.logFile != nil {
		_, _ = fmt.Fprintln(a.deploy.logFile, line)
	}
}

func (a *App) setStep(step *DeployStepStatus, status, message string) {
	a.deploy.mu.Lock()
	step.Status = status
	step.Message = message
	a.deploy.mu.Unlock()
	a.deployLog("[%s] %s %s", step.Name, status, message)
}

func (a *App) runDeploy(job *DeployJob, force bool) {
	logFile, err := os.Create(job.LogPath)
	if err == nil {
		a.deploy.logFile = logFile
		defer func() { _ = logFile.Close() }()
	}
	a.deployLog("部署任务 %s 开始，共 %d 步", job.ID, len(job.Steps))
	failed := false
	cancelled := false
	for _, step := range job.Steps {
		if a.cancelled() {
			cancelled = true
			a.setStep(step, "skipped", "任务已取消")
			continue
		}
		if failed {
			a.setStep(step, "skipped", "因上一步失败而跳过")
			continue
		}
		var err error
		if step.Action == "authorize" {
			err = a.runAuthorizeStep(step)
		} else {
			entry := findSoftware(a.catalog, step.SoftwareID)
			err = a.executeStep(step, entry, force)
		}
		if err != nil {
			if errors.Is(err, errCancelled) || a.cancelled() {
				cancelled = true
				a.setStep(step, "skipped", "任务已取消")
				continue
			}
			a.setStep(step, "failed", err.Error())
			failed = true
		}
	}
	a.deploy.mu.Lock()
	a.deploy.running = false
	switch {
	case cancelled:
		job.State = "cancelled"
	case failed:
		job.State = "failed"
	default:
		job.State = "done"
	}
	job.FinishedAt = time.Now().Format(time.RFC3339)
	if a.deploy.cancel != nil {
		a.deploy.cancel()
		a.deploy.cancel = nil
	}
	a.deploy.mu.Unlock()
	a.deployLog("部署任务结束：%s", job.State)
}

const authWaitTimeout = 5 * time.Minute

// authValidity：网站授权标记的有效期，过期后需重新授权。
// 网站登录态本身可能更短，但这里只约束本地"我已授权"标记的生命周期。
const authValidity = 30 * 24 * time.Hour

type authMarker struct {
	SiteID       string `json:"siteId"`
	Name         string `json:"name"`
	URL          string `json:"url"`
	AuthorizedAt string `json:"authorizedAt"`
	ExpiresAt    string `json:"expiresAt,omitempty"`
}

// readAuthMarker 读取授权标记并判断是否在有效期内。
// 文件不存在返回 authorized=false；无日期字段的旧版标记视为仍有效（向后兼容）。
func readAuthMarker(authDir, siteID string) (authorized bool, expiresAt time.Time) {
	data, err := os.ReadFile(filepath.Join(authDir, siteID+".json"))
	if err != nil {
		return false, time.Time{}
	}
	var marker authMarker
	if err := json.Unmarshal(data, &marker); err != nil {
		return false, time.Time{}
	}
	if marker.ExpiresAt != "" {
		if t, err := time.Parse(time.RFC3339, marker.ExpiresAt); err == nil {
			return time.Now().Before(t), t
		}
	}
	if marker.AuthorizedAt != "" {
		if t, err := time.Parse(time.RFC3339, marker.AuthorizedAt); err == nil {
			exp := t.Add(authValidity)
			return time.Now().Before(exp), exp
		}
	}
	// 旧版标记无任何日期字段，视为仍有效，避免强制老用户立即重新授权。
	return true, time.Time{}
}

// runAuthorizeStep：等待前端登录框完成授权；本机已有有效授权记录的直接跳过。
func (a *App) runAuthorizeStep(step *DeployStepStatus) error {
	authorized, expiresAt := readAuthMarker(a.authDir, step.SiteID)
	if authorized {
		a.setStep(step, "skipped", fmt.Sprintf("已授权，有效期至 %s", expiresAt.Format("2006-01-02")))
		return nil
	}
	if !expiresAt.IsZero() {
		a.setStep(step, "authorizing", "授权已于 "+expiresAt.Format("2006-01-02")+" 过期，请重新授权 "+step.Name)
	} else {
		a.setStep(step, "authorizing", "请在弹出的登录框中完成 "+step.Name+" 授权")
	}
	deadline := time.Now().Add(authWaitTimeout)
	for time.Now().Before(deadline) {
		if a.cancelled() {
			return errCancelled
		}
		if ok, _ := readAuthMarker(a.authDir, step.SiteID); ok {
			a.setStep(step, "done", "授权完成")
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("等待 %s 授权超时，请重新部署", step.Name)
}

// handleAuthConfirm：前端点击「我已完成授权」后写入本机授权记录。
func (a *App) handleAuthConfirm(w http.ResponseWriter, r *http.Request) {
	var request struct {
		SiteID string `json:"siteId"`
	}
	if err := decodeJSON(r, &request); err != nil || request.SiteID == "" {
		fail(w, http.StatusBadRequest, "缺少网站标识")
		return
	}
	site := findSite(a.sites, request.SiteID)
	if site == nil {
		fail(w, http.StatusBadRequest, "未知网站："+request.SiteID)
		return
	}
	if err := os.MkdirAll(a.authDir, 0o755); err != nil {
		fail(w, http.StatusInternalServerError, "写入授权记录失败")
		return
	}
	now := time.Now()
	record := map[string]any{
		"siteId": site.ID, "name": site.Name, "url": site.URL,
		"authorizedAt": now.Format(time.RFC3339),
		"expiresAt":    now.Add(authValidity).Format(time.RFC3339),
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err == nil {
		err = os.WriteFile(filepath.Join(a.authDir, site.ID+".json"), data, 0o644)
	}
	if err != nil {
		fail(w, http.StatusInternalServerError, "写入授权记录失败")
		return
	}
	a.deployLog("网站授权完成：%s", site.Name)
	writeJSON(w, http.StatusOK, map[string]any{"state": "authorized", "siteId": site.ID})
}

func (a *App) executeStep(step *DeployStepStatus, entry *SoftwareEntry, force bool) error {
	targetDir := filepath.Join(a.runtimeRoot, entry.ID)
	// AGPL 软件即使目录误配为 portable-zip，也强制走手动引导，杜绝开源传染风险。
	if entry.isAGPL() {
		a.setStep(step, "skipped", "该软件许可证为 "+entry.License+"，需手动安装："+entry.Note)
		return nil
	}
	if entry.Install.Type == "manual" {
		a.setStep(step, "skipped", "该软件需手动安装："+entry.Note)
		return nil
	}
	if !force {
		if _, err := os.Stat(filepath.Join(targetDir, "manifest.json")); err == nil {
			a.setStep(step, "skipped", "已安装，跳过（可勾选强制重装）")
			return nil
		}
	}
	switch entry.Install.Type {
	case "portable-zip":
		return a.installPortableZip(step, entry, targetDir)
	case "cli":
		return a.installByCLI(step, entry, targetDir)
	default:
		return fmt.Errorf("不支持的安装方式：%s", entry.Install.Type)
	}
}

func (a *App) installPortableZip(step *DeployStepStatus, entry *SoftwareEntry, targetDir string) error {
	urls := entry.Install.allURLs()
	if len(urls) == 0 {
		return fmt.Errorf("目录未提供下载地址")
	}
	var lastErr error
	for idx, url := range urls {
		if idx > 0 {
			a.setStep(step, "downloading", fmt.Sprintf("上一源失败，尝试备用源 %d/%d…", idx+1, len(urls)))
		}
		archivePath := filepath.Join(a.downloadDir, entry.ID+".zip")
		if err := a.downloadFile(step, url, archivePath); err != nil {
			if errors.Is(err, errCancelled) || a.cancelled() {
				return errCancelled
			}
			lastErr = err
			a.deployLog("[%s] 下载源 %d 失败：%v", entry.Name, idx+1, err)
			continue
		}
		// 下载成功，校验并解压。
		verified := false
		if entry.Install.SHA256 != "" {
			a.setStep(step, "verifying", "校验 SHA-256")
			sum, err := fileSHA256(archivePath)
			if err != nil {
				return err
			}
			if !strings.EqualFold(sum, entry.Install.SHA256) {
				_ = os.Remove(archivePath)
				return fmt.Errorf("SHA-256 校验不匹配，已删除安装包并中止")
			}
			verified = true
		} else {
			a.deployLog("[WARN] [%s] 未配置 SHA-256，本次安装跳过完整性校验（来源 %s）", entry.Name, url)
		}
		a.deploy.mu.Lock()
		step.HashVerified = &verified
		a.deploy.mu.Unlock()

		a.setStep(step, "extracting", "解压到 "+targetDir)
		if err := os.RemoveAll(targetDir); err != nil {
			return err
		}
		if err := unzip(archivePath, targetDir); err != nil {
			return fmt.Errorf("解压失败：%v", err)
		}
		if err := writeManifest(targetDir, entry); err != nil {
			return err
		}
		return a.verifyInstall(step, entry, targetDir)
	}
	// 所有下载源均失败，引导用户到 fallback 页面。
	msg := fmt.Sprintf("所有下载源均失败：%v", lastErr)
	if entry.Install.Fallback != "" {
		msg += "。请手动访问 " + entry.Install.Fallback + " 下载安装"
	}
	return fmt.Errorf("%s", msg)
}

func (a *App) installByCLI(step *DeployStepStatus, entry *SoftwareEntry, targetDir string) error {
	fields, err := splitCommand(entry.Install.Command)
	if err != nil {
		return fmt.Errorf("安装命令解析失败：%v", err)
	}
	exe, err := findExecutable(a.runtimeRoot, fields[0])
	if err != nil {
		return fmt.Errorf("未找到 %s：请先完成依赖软件的安装", fields[0])
	}
	a.setStep(step, "installing", entry.Install.Command)
	ctx, cancel := context.WithTimeout(a.deployContext(), 30*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, fields[1:]...)
	cmd.Dir = filepath.Dir(exe)
	output, err := cmd.CombinedOutput()
	a.deployLog("[%s] 命令输出：%s", entry.Name, truncate(string(output), 400))
	if err != nil {
		return fmt.Errorf("命令执行失败：%v", err)
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return err
	}
	if err := writeManifest(targetDir, entry); err != nil {
		return err
	}
	return a.verifyInstall(step, entry, filepath.Dir(exe))
}

func (a *App) verifyInstall(step *DeployStepStatus, entry *SoftwareEntry, searchDir string) error {
	if strings.TrimSpace(entry.Verify.Command) == "" {
		a.setStep(step, "done", "安装完成")
		return nil
	}
	fields, err := splitCommand(entry.Verify.Command)
	if err != nil {
		return fmt.Errorf("验证命令解析失败：%v", err)
	}
	a.setStep(step, "verifying-install", entry.Verify.Command)
	exe, err := findExecutable(searchDir, fields[0])
	if err != nil {
		exe, err = findExecutable(a.runtimeRoot, fields[0])
	}
	if err != nil {
		return fmt.Errorf("安装后未找到可执行文件 %s", fields[0])
	}
	ctx, cancel := context.WithTimeout(a.deployContext(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, fields[1:]...)
	cmd.Dir = filepath.Dir(exe)
	output, err := cmd.CombinedOutput()
	a.deployLog("[%s] 验证输出：%s", entry.Name, truncate(string(output), 300))
	if err != nil {
		return fmt.Errorf("安装验证失败：%v", err)
	}
	a.setStep(step, "done", "已安装并验证可用："+exe)
	return nil
}

// downloadFile 下载安装包并持续回报体积、已下载量与速度，前端据此显示百分比与剩余时间。
func (a *App) downloadFile(step *DeployStepStatus, url, target string) error {
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		if a.cancelled() {
			return errCancelled
		}
		lastErr = a.downloadOnce(step, url, target)
		if lastErr == nil {
			return nil
		}
		if errors.Is(lastErr, errCancelled) || a.cancelled() {
			return errCancelled
		}
		if attempt == 0 {
			select {
			case <-time.After(2 * time.Second):
			case <-a.deployContext().Done():
				return errCancelled
			}
		}
	}
	return lastErr
}

func (a *App) downloadOnce(step *DeployStepStatus, url, target string) error {
	parsed, err := neturl.Parse(url)
	if err != nil {
		return fmt.Errorf("下载地址非法：%v", err)
	}
	// 只允许 HTTPS（本机回环除外）：明文传输可能被中间人替换安装包。
	if parsed.Scheme != "https" && parsed.Hostname() != "127.0.0.1" && parsed.Hostname() != "localhost" {
		return fmt.Errorf("下载地址必须是 HTTPS：%s", url)
	}
	request, err := http.NewRequestWithContext(a.deployContext(), http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 30 * time.Minute}
	response, err := client.Do(request)
	if err != nil {
		if a.cancelled() {
			return errCancelled
		}
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", response.StatusCode)
	}
	if response.Request != nil {
		final := response.Request.URL
		if final.Scheme != "https" && final.Hostname() != "127.0.0.1" && final.Hostname() != "localhost" {
			return fmt.Errorf("下载被重定向到非 HTTPS 地址，已中止")
		}
	}
	if response.ContentLength > 0 {
		a.deploy.mu.Lock()
		step.TotalBytes = response.ContentLength
		a.deploy.mu.Unlock()
	}

	part := target + ".part"
	file, err := os.Create(part)
	if err != nil {
		return err
	}
	a.deploy.mu.Lock()
	step.DownloadedBytes = 0
	step.BytesPerSecond = 0
	step.DownloadStartedAt = time.Now().Format(time.RFC3339)
	a.deploy.mu.Unlock()
	lastSample, sampleBytes := time.Now(), int64(0)
	var written int64
	buffer := make([]byte, 128*1024)
	for {
		if a.cancelled() {
			_ = file.Close()
			_ = os.Remove(part)
			return errCancelled
		}
		n, readErr := response.Body.Read(buffer)
		if n > 0 {
			if _, err := file.Write(buffer[:n]); err != nil {
				_ = file.Close()
				return err
			}
			written += int64(n)
			// 每秒采样一次，用区间速度估算剩余时间，避免起步速度拉低整体平均值。
			speed := int64(0)
			if now := time.Now(); now.Sub(lastSample) >= time.Second {
				speed = int64(float64(written-sampleBytes) / now.Sub(lastSample).Seconds())
				lastSample, sampleBytes = now, written
			}
			a.deploy.mu.Lock()
			step.DownloadedBytes = written
			if speed > 0 {
				step.BytesPerSecond = speed
			}
			step.Message = downloadMessage(written, step.TotalBytes, step.BytesPerSecond)
			a.deploy.mu.Unlock()
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			_ = file.Close()
			if a.cancelled() {
				_ = os.Remove(part)
				return errCancelled
			}
			return readErr
		}
	}
	if err := file.Close(); err != nil {
		return err
	}
	a.deploy.mu.Lock()
	step.DownloadedBytes = written
	if step.TotalBytes <= 0 {
		step.TotalBytes = written
	}
	step.Message = downloadMessage(written, step.TotalBytes, step.BytesPerSecond)
	a.deploy.mu.Unlock()
	a.deployLog("[%s] 下载完成：%s", step.Name, humanSize(written))
	return os.Rename(part, target)
}

// downloadMessage 生成下载进度文案，例如「已下载 12.3 MB / 800 MB（2%）· 剩余约 10 分」。
func downloadMessage(downloaded, total, speed int64) string {
	if total > 0 {
		percent := int(float64(downloaded) / float64(total) * 100)
		if percent > 100 {
			percent = 100
		}
		message := fmt.Sprintf("已下载 %s / %s（%d%%）", humanSize(downloaded), humanSize(total), percent)
		if speed > 0 && downloaded < total {
			message += " · 剩余约 " + humanDuration((total-downloaded)/speed)
		}
		return message
	}
	message := "已下载 " + humanSize(downloaded)
	if speed > 0 {
		message += " · " + humanSize(speed) + "/s"
	}
	return message
}

// humanSize 把字节数格式化成人类可读的体积。
func humanSize(bytes int64) string {
	switch {
	case bytes >= 1024*1024*1024:
		return fmt.Sprintf("%.2f GB", float64(bytes)/(1024*1024*1024))
	case bytes >= 1024*1024:
		return fmt.Sprintf("%.1f MB", float64(bytes)/(1024*1024))
	case bytes >= 1024:
		return fmt.Sprintf("%.0f KB", float64(bytes)/1024)
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

// humanDuration 把秒数格式化成「N 秒 / N 分 / N 分 M 秒 / N 小时 M 分」。
func humanDuration(seconds int64) string {
	if seconds < 1 {
		return "几秒"
	}
	switch {
	case seconds < 60:
		return fmt.Sprintf("%d 秒", seconds)
	case seconds < 3600:
		minutes := seconds / 60
		if rest := seconds % 60; rest > 0 {
			return fmt.Sprintf("%d 分 %d 秒", minutes, rest)
		}
		return fmt.Sprintf("%d 分", minutes)
	default:
		hours := seconds / 3600
		minutes := (seconds % 3600) / 60
		if minutes > 0 {
			return fmt.Sprintf("%d 小时 %d 分", hours, minutes)
		}
		return fmt.Sprintf("%d 小时", hours)
	}
}

// splitCommand 按空白切分命令，支持用双引号包裹含空格的路径。
// 例如：`"C:\Program Files\App\app.exe" --version` → 两个参数。
// 之前的 strings.Fields 会把带空格的路径切碎，导致找不到可执行文件。
func splitCommand(command string) ([]string, error) {
	var (
		fields  []string
		current strings.Builder
		inQuote bool
		hasArg  bool
	)
	flush := func() {
		if hasArg {
			fields = append(fields, current.String())
			current.Reset()
			hasArg = false
		}
	}
	for _, r := range command {
		switch {
		case r == '"':
			inQuote = !inQuote
			hasArg = true
		case r == ' ' || r == '\t' || r == '\n' || r == '\r':
			if inQuote {
				current.WriteRune(r)
				hasArg = true
			} else {
				flush()
			}
		default:
			current.WriteRune(r)
			hasArg = true
		}
	}
	if inQuote {
		return nil, fmt.Errorf("命令中的引号未闭合：%s", command)
	}
	flush()
	if len(fields) == 0 {
		return nil, fmt.Errorf("命令为空")
	}
	return fields, nil
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

// unzip：防路径穿越，保留目录结构。
func unzip(archivePath, targetDir string) error {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer reader.Close()
	const (
		maxExtractBytes = int64(3) << 30 // 解压后总大小上限 3 GiB，防 zip bomb
		maxEntries      = 20000          // 文件数量上限
	)
	if len(reader.File) > maxEntries {
		return fmt.Errorf("安装包文件数异常（%d 个），已中止", len(reader.File))
	}
	var total int64
	for _, file := range reader.File {
		clean := filepath.Clean(file.Name)
		if strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) ||
			strings.ContainsAny(clean, ":") || strings.HasPrefix(clean, `\`) {
			return fmt.Errorf("安装包含非法路径：%s", file.Name)
		}
		if file.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("安装包含符号链接，已中止：%s", file.Name)
		}
		total += int64(file.UncompressedSize64)
		if total > maxExtractBytes {
			return fmt.Errorf("解压后体积超过上限（3 GB），已中止")
		}
		target := filepath.Join(targetDir, clean)
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		source, err := file.Open()
		if err != nil {
			return err
		}
		destination, err := os.Create(target)
		if err != nil {
			source.Close()
			return err
		}
		_, err = io.Copy(destination, source)
		source.Close()
		closeErr := destination.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

func writeManifest(targetDir string, entry *SoftwareEntry) error {
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return err
	}
	manifest := map[string]any{
		"id": entry.ID, "name": entry.Name, "kind": entry.Kind,
		"installedAt": time.Now().Format(time.RFC3339),
		"installType": entry.Install.Type, "source": entry.Install.primaryURL(),
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(targetDir, "manifest.json"), data, 0o644)
}

func findExecutable(root, name string) (string, error) {
	found := ""
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || found != "" {
			return nil
		}
		if !entry.IsDir() && strings.EqualFold(entry.Name(), name) {
			found = path
		}
		return nil
	})
	if found != "" {
		return found, nil
	}
	if err != nil {
		return "", err
	}
	return "", fmt.Errorf("未找到 %s", name)
}

func truncate(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	return text[:limit] + "…"
}
