package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// 语音转写：用本地 Whisper.cpp + ffmpeg，全程离线，音频不出本机。
// 两者都是「按需下载」的组件，缺失时如实报错，不假装可用。

const (
	asrMaxBytes   = 25 << 20 // 25 MB base64 上限
	asrMaxSeconds = 180      // 单次最长 3 分钟，避免本地 CPU 长时间占用
)

type ASRStatus struct {
	Available   bool   `json:"available"`
	WhisperPath string `json:"whisperPath"`
	ModelPath   string `json:"modelPath"`
	FFmpegPath  string `json:"ffmpegPath"`
	Message     string `json:"message"`
}

type ASRResult struct {
	Text      string `json:"text"`
	ElapsedMs int64  `json:"elapsedMs"`
	Language  string `json:"language"`
}

func runtimeRootDir() string {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		base = "."
	}
	return filepath.Join(base, "Kaixin", "runtime")
}

// findFirst 在目录下递归找指定文件名，找不到返回空串。
func findFirst(root, name string) string {
	found := ""
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || found != "" {
			return nil
		}
		if !entry.IsDir() && strings.EqualFold(entry.Name(), name) {
			found = path
		}
		return nil
	})
	return found
}

// asrStatus 检查转写链路是否就绪：二进制 + 模型 + ffmpeg 三者缺一不可。
func asrStatus() ASRStatus {
	root := runtimeRootDir()
	whisperRoot := filepath.Join(root, "whisper")

	status := ASRStatus{}
	status.WhisperPath = findFirst(whisperRoot, "whisper-cli.exe")
	status.FFmpegPath = findFirst(filepath.Join(root, "ffmpeg"), "ffmpeg.exe")

	modelCandidate := filepath.Join(whisperRoot, "models", "ggml-base.bin")
	if info, err := os.Stat(modelCandidate); err == nil && !info.IsDir() {
		status.ModelPath = modelCandidate
	} else {
		status.ModelPath = findFirst(whisperRoot, "ggml-base.bin")
	}

	switch {
	case status.WhisperPath == "":
		status.Message = "未安装 whisper-cli，无法转写"
	case status.FFmpegPath == "":
		status.Message = "未安装 ffmpeg，无法转换音频格式"
	case status.ModelPath == "":
		status.Message = "缺少 Whisper 模型（ggml-base.bin），无法转写"
	default:
		status.Available = true
		status.Message = "本地 Whisper 就绪（离线转写）"
	}
	return status
}

// asrTempDir：优先系统临时目录，失败时退回自有数据目录。
// 某些受限环境下系统 temp 不可写，而自有目录（由本程序创建）通常可以。
func asrTempDir() (string, error) {
	if dir, err := os.MkdirTemp("", "kaixin-asr-"); err == nil {
		return dir, nil
	}
	candidates := []string{filepath.Join(runtimeRootDir(), "tmp")}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "tmp"))
	}
	var lastErr error
	for _, dir := range candidates {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			lastErr = err
			continue
		}
		if created, err := os.MkdirTemp(dir, "kaixin-asr-"); err == nil {
			return created, nil
		} else {
			lastErr = err
		}
	}
	return "", lastErr
}

// transcribe 把音频转成文字：ffmpeg 转 16k 单声道 wav，再交给 whisper-cli。
func transcribe(ctx context.Context, audio []byte, language string) (ASRResult, error) {
	status := asrStatus()
	if !status.Available {
		return ASRResult{}, fmt.Errorf("%s", status.Message)
	}
	if language == "" {
		language = "zh"
	}

	tempDir, err := asrTempDir()
	if err != nil {
		return ASRResult{}, fmt.Errorf("创建临时目录失败: %w", err)
	}
	defer os.RemoveAll(tempDir)

	srcPath := filepath.Join(tempDir, "input.bin")
	wavPath := filepath.Join(tempDir, "input.wav")
	if err := os.WriteFile(srcPath, audio, 0o600); err != nil {
		return ASRResult{}, fmt.Errorf("写入音频失败: %w", err)
	}

	started := time.Now()

	// 1) 转码：whisper 只吃 16 kHz 单声道 PCM
	ffCtx, ffCancel := context.WithTimeout(ctx, 60*time.Second)
	defer ffCancel()
	ff := exec.CommandContext(ffCtx, status.FFmpegPath,
		"-y", "-hide_banner", "-loglevel", "error",
		"-i", srcPath, "-ar", "16000", "-ac", "1", "-c:a", "pcm_s16le", wavPath)
	if out, err := ff.CombinedOutput(); err != nil {
		return ASRResult{}, fmt.Errorf("音频转码失败: %v %s", err, truncate(string(out), 200))
	}

	// 2) 转写：-oj 输出 json，比解析 stdout 稳
	asrCtx, cancel := context.WithTimeout(ctx, 150*time.Second)
	defer cancel()
	cmd := exec.CommandContext(asrCtx, status.WhisperPath,
		"-m", status.ModelPath,
		"-f", wavPath,
		"-l", language,
		"-t", "4",
		"-nt",
		"-oj")
	cmd.Dir = filepath.Dir(status.WhisperPath) // DLL 与可执行文件同目录
	out, err := cmd.CombinedOutput()
	if asrCtx.Err() == context.DeadlineExceeded {
		return ASRResult{}, fmt.Errorf("转写超时（超过 150 秒）")
	}
	if err != nil {
		return ASRResult{}, fmt.Errorf("转写失败: %v %s", err, truncate(string(out), 200))
	}

	text := readWhisperJSON(wavPath + ".json")
	if text == "" {
		text = parseWhisperStdout(string(out))
	}
	return ASRResult{
		Text:      strings.TrimSpace(text),
		ElapsedMs: time.Since(started).Milliseconds(),
		Language:  language,
	}, nil
}

// readWhisperJSON 读 -oj 生成的 json；结构为 {transcription:[{text:"..."}]}
func readWhisperJSON(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var payload struct {
		Transcription []struct {
			Text string `json:"text"`
		} `json:"transcription"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ""
	}
	parts := make([]string, 0, len(payload.Transcription))
	for _, item := range payload.Transcription {
		if text := strings.TrimSpace(item.Text); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, " ")
}

// parseWhisperStdout：兜底解析，去掉 [00:00:00.000 --> ...] 这类时间标记。
func parseWhisperStdout(output string) string {
	lines := []string{}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "[") || strings.HasPrefix(line, "whisper_") {
			continue
		}
		if strings.Contains(line, "-->") {
			continue
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, " ")
}

// ---------- HTTP ----------

func (a *App) handleASRStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, asrStatus())
}

func (a *App) handleASRTranscribe(w http.ResponseWriter, r *http.Request) {
	var request struct {
		AudioBase64 string `json:"audioBase64"`
		Format      string `json:"format"`
		Language    string `json:"language"`
	}
	if err := decodeJSON(r, &request); err != nil {
		fail(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	if request.AudioBase64 == "" {
		fail(w, http.StatusBadRequest, "音频为空")
		return
	}
	if len(request.AudioBase64) > asrMaxBytes {
		fail(w, http.StatusRequestEntityTooLarge,
			fmt.Sprintf("音频过大（约 %.1f MB），单次上限 25 MB（约 %d 秒）",
				float64(len(request.AudioBase64))/(1<<20)*0.75, asrMaxSeconds))
		return
	}

	audio, err := base64.StdEncoding.DecodeString(request.AudioBase64)
	if err != nil {
		fail(w, http.StatusBadRequest, "音频解码失败")
		return
	}

	result, err := transcribe(r.Context(), audio, request.Language)
	if err != nil {
		fail(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}
