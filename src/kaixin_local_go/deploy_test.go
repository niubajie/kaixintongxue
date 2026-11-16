package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func makeZip(t *testing.T, files map[string]string) []byte {
	buffer := &bytes.Buffer{}
	writer := zip.NewWriter(buffer)
	for name, content := range files {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatalf("构造 zip 失败: %v", err)
		}
		if _, err := entry.Write([]byte(content)); err != nil {
			t.Fatalf("写入 zip 失败: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("关闭 zip 失败: %v", err)
	}
	return buffer.Bytes()
}

func newDeployTestApp(t *testing.T, payload []byte) (*App, *httptest.Server) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(payload)
	}))
	root := t.TempDir()
	app := &App{
		runtimeRoot: filepath.Join(root, "runtime"),
		downloadDir: filepath.Join(root, "downloads"),
		logDir:      filepath.Join(root, "logs"),
	}
	for _, dir := range []string{app.runtimeRoot, app.downloadDir, app.logDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("创建目录失败: %v", err)
		}
	}
	return app, server
}

func TestPortableZipDeploy(t *testing.T) {
	payload := makeZip(t, map[string]string{"tool/bin/tool.exe": "binary", "tool/readme.txt": "hello"})
	app, server := newDeployTestApp(t, payload)
	defer server.Close()
	sum := sha256.Sum256(payload)
	entry := &SoftwareEntry{
		ID: "tool", Name: "Tool",
		Install: InstallSpec{Type: "portable-zip", URL: server.URL, SHA256: hex.EncodeToString(sum[:])},
	}
	step := &DeployStepStatus{SoftwareID: "tool", Name: "Tool"}
	if err := app.installPortableZip(step, entry, filepath.Join(app.runtimeRoot, "tool")); err != nil {
		t.Fatalf("部署失败: %v", err)
	}
	if step.Status != "done" {
		t.Fatalf("期望 done，实际 %s", step.Status)
	}
	if step.HashVerified == nil || !*step.HashVerified {
		t.Fatal("应完成 SHA-256 校验")
	}
	if _, err := os.Stat(filepath.Join(app.runtimeRoot, "tool", "tool", "bin", "tool.exe")); err != nil {
		t.Fatalf("解压结果缺失: %v", err)
	}
	if _, err := os.Stat(filepath.Join(app.runtimeRoot, "tool", "manifest.json")); err != nil {
		t.Fatalf("manifest 缺失: %v", err)
	}
}

func TestPortableZipHashMismatch(t *testing.T) {
	payload := makeZip(t, map[string]string{"a.txt": "a"})
	app, server := newDeployTestApp(t, payload)
	defer server.Close()
	entry := &SoftwareEntry{
		ID: "bad", Name: "Bad",
		Install: InstallSpec{Type: "portable-zip", URL: server.URL, SHA256: "deadbeef"},
	}
	step := &DeployStepStatus{SoftwareID: "bad", Name: "Bad"}
	err := app.installPortableZip(step, entry, filepath.Join(app.runtimeRoot, "bad"))
	if err == nil {
		t.Fatal("哈希不匹配必须中止")
	}
	if _, statErr := os.Stat(filepath.Join(app.downloadDir, "bad.zip")); !os.IsNotExist(statErr) {
		t.Fatal("校验失败后应删除安装包")
	}
}

func TestUnzipRejectsTraversal(t *testing.T) {
	payload := makeZip(t, map[string]string{"../evil.txt": "x"})
	archive := filepath.Join(t.TempDir(), "evil.zip")
	if err := os.WriteFile(archive, payload, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := unzip(archive, t.TempDir()); err == nil {
		t.Fatal("路径穿越必须被拒绝")
	}
}

func TestFindExecutableNested(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(nested, "ffmpeg.exe")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	found, err := findExecutable(root, "ffmpeg.exe")
	if err != nil || found != target {
		t.Fatalf("递归查找失败: %v %s", err, found)
	}
}

// TestRunAuthorizeStepSkipsAuthorized 验证已有授权记录时直接跳过，不等待登录。
func TestRunAuthorizeStepSkipsAuthorized(t *testing.T) {
	authDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(authDir, "douyin.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	app := &App{authDir: authDir}
	step := &DeployStepStatus{Name: "抖音", Action: "authorize", SiteID: "douyin"}
	if err := app.runAuthorizeStep(step); err != nil {
		t.Fatalf("已授权不应报错: %v", err)
	}
	if step.Status != "skipped" {
		t.Fatalf("已授权应跳过，实际 %s", step.Status)
	}
}

// TestDownloadProgressReportsBytes 验证下载过程中会持续回报总量、已下载量与百分比。
func TestDownloadProgressReportsBytes(t *testing.T) {
	payload := bytes.Repeat([]byte("k"), 1024*1024)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
		flusher, _ := w.(http.Flusher)
		for start := 0; start < len(payload); start += 64 * 1024 {
			end := start + 64*1024
			if end > len(payload) {
				end = len(payload)
			}
			if _, err := w.Write(payload[start:end]); err != nil {
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
			time.Sleep(5 * time.Millisecond)
		}
	}))
	defer server.Close()

	root := t.TempDir()
	app := &App{downloadDir: filepath.Join(root, "downloads"), logDir: filepath.Join(root, "logs")}
	for _, dir := range []string{app.downloadDir, app.logDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	step := &DeployStepStatus{SoftwareID: "tool", Name: "Tool"}
	done := make(chan error, 1)
	go func() {
		done <- app.downloadFile(step, server.URL, filepath.Join(app.downloadDir, "tool.zip"))
	}()

	sawProgress := false
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) && !sawProgress {
		app.deploy.mu.Lock()
		downloaded, total, message := step.DownloadedBytes, step.TotalBytes, step.Message
		app.deploy.mu.Unlock()
		if downloaded > 0 && downloaded < int64(len(payload)) &&
			total == int64(len(payload)) && strings.Contains(message, "%") {
			sawProgress = true
		}
		time.Sleep(time.Millisecond)
	}
	if err := <-done; err != nil {
		t.Fatalf("下载失败: %v", err)
	}
	if !sawProgress {
		t.Fatal("下载过程中未上报已下载字节与百分比")
	}
	if step.DownloadedBytes != int64(len(payload)) || step.TotalBytes != int64(len(payload)) {
		t.Fatalf("下载完成后体积不符：已下载 %d，总量 %d", step.DownloadedBytes, step.TotalBytes)
	}
	if !strings.Contains(step.Message, "100%") {
		t.Fatalf("下载完成后文案应包含 100%%，实际 %s", step.Message)
	}
	if step.DownloadStartedAt == "" {
		t.Fatal("应记录下载开始时间")
	}
}

// TestDownloadCancellationStopsRequest 验证阻塞中的下载会立即响应取消，且不再重试。
func TestDownloadCancellationStopsRequest(t *testing.T) {
	var requests atomic.Int32
	started := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Length", strconv.Itoa(1024*1024))
		_, _ = w.Write([]byte("first chunk"))
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		select {
		case started <- struct{}{}:
		default:
		}
		<-r.Context().Done()
	}))
	defer server.Close()

	app := &App{downloadDir: t.TempDir()}
	ctx, cancel := context.WithCancel(context.Background())
	app.deploy.ctx = ctx
	step := &DeployStepStatus{SoftwareID: "tool", Name: "Tool"}
	target := filepath.Join(app.downloadDir, "tool.zip")
	done := make(chan error, 1)
	go func() {
		done <- app.downloadFile(step, server.URL, target)
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("下载未开始")
	}
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, errCancelled) {
			t.Fatalf("期望任务取消，实际 %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("下载未在取消后及时退出")
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("取消后不应重试，实际请求 %d 次", got)
	}
	if _, err := os.Stat(target + ".part"); !os.IsNotExist(err) {
		t.Fatal("取消后应清理临时下载文件")
	}
}
