package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// scanPackage 调用本机杀毒引擎扫描安装包，返回 clean / infected / unavailable。
func (a *App) scanPackage(path string) (string, string) {
	switch a.cfg.Scanner {
	case "none":
		return "unavailable", "已通过 KAIXIN_SCANNER=none 关闭安全审查。"
	case "clamav":
		return scanWithClamAV(path)
	}

	if runtime.GOOS == "windows" {
		if engine := findDefender(); engine != "" {
			return scanWithDefender(engine, path)
		}
	}
	if _, err := exec.LookPath("clamscan"); err == nil {
		return scanWithClamAV(path)
	}
	return "unavailable", "本机未找到可用的杀毒引擎（Windows Defender 或 ClamAV）。"
}

// findDefender 优先使用 Platform 目录下版本号最大的 MpCmdRun.exe。
func findDefender() string {
	candidates := []string{
		filepath.Join(os.Getenv("ProgramFiles"), "Windows Defender", "MpCmdRun.exe"),
	}
	platformRoot := filepath.Join(os.Getenv("ProgramData"), "Microsoft", "Windows Defender", "Platform")
	if entries, err := os.ReadDir(platformRoot); err == nil {
		latest := ""
		for _, entry := range entries {
			if entry.IsDir() && entry.Name() > latest {
				latest = entry.Name()
			}
		}
		if latest != "" {
			candidates = append(candidates, filepath.Join(platformRoot, latest, "MpCmdRun.exe"))
		}
	}
	for index := len(candidates) - 1; index >= 0; index-- {
		if info, err := os.Stat(candidates[index]); err == nil && !info.IsDir() {
			return candidates[index]
		}
	}
	return ""
}

func scanWithDefender(engine, path string) (string, string) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	command := exec.CommandContext(ctx, engine, "-Scan", "-ScanType", "3", "-File", path, "-DisableRemediation")
	output, err := command.CombinedOutput()
	detail := tailText(string(output), 200)
	if err == nil {
		return "clean", ""
	}
	if exitError, ok := err.(*exec.ExitError); ok && exitError.ExitCode() == 2 {
		if detail == "" {
			detail = "Windows Defender 报告发现威胁。"
		}
		return "infected", detail
	}
	if detail == "" {
		detail = "Windows Defender 未返回结果：" + err.Error()
	}
	return "unavailable", detail
}

func scanWithClamAV(path string) (string, string) {
	engine, err := exec.LookPath("clamscan")
	if err != nil {
		return "unavailable", "未安装 ClamAV。"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	command := exec.CommandContext(ctx, engine, "--no-summary", path)
	output, err := command.CombinedOutput()
	detail := tailText(string(output), 200)
	if err == nil {
		return "clean", ""
	}
	if exitError, ok := err.(*exec.ExitError); ok && exitError.ExitCode() == 1 {
		if detail == "" {
			detail = "ClamAV 报告发现威胁。"
		}
		return "infected", detail
	}
	if detail == "" {
		detail = "ClamAV 未返回结果：" + err.Error()
	}
	return "unavailable", detail
}

func tailText(text string, limit int) string {
	cleaned := strings.TrimSpace(strings.ReplaceAll(text, "\r\n", "\n"))
	runes := []rune(cleaned)
	if len(runes) <= limit {
		return cleaned
	}
	return string(runes[len(runes)-limit:])
}
