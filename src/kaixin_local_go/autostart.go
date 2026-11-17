package main

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
)

// 开机启动：写 HKCU 下的 Run 键，不申请管理员权限、不写 HKLM。
// 用 reg.exe 而不是注册表库，避免为这一件事引入新依赖。

const (
	autostartValueName = "KaixinAssistant"
	autostartRunKey    = `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`
)

type AutostartState struct {
	Enabled bool   `json:"enabled"`
	Command string `json:"command"`
	KeyPath string `json:"keyPath"`
	Message string `json:"message"`
}

// queryAutostart 读取当前启动项。
func queryAutostart() AutostartState {
	state := AutostartState{KeyPath: autostartRunKey}
	out, err := exec.Command("reg", "query", autostartRunKey, "/v", autostartValueName).Output()
	if err != nil {
		state.Message = "未设置开机启动"
		return state
	}
	// 输出形如：    KaixinAssistant    REG_SZ    C:\path\app.exe
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 3 && fields[0] == autostartValueName {
			state.Enabled = true
			state.Command = strings.Join(fields[2:], " ")
			state.Message = "已设置开机启动"
			return state
		}
	}
	state.Message = "未设置开机启动"
	return state
}

// setAutostart 增加或删除启动项；exePath 必须是存在的绝对路径。
func setAutostart(exePath string, enabled bool) error {
	if !enabled {
		cmd := exec.Command("reg", "delete", autostartRunKey, "/v", autostartValueName, "/f")
		if out, err := cmd.CombinedOutput(); err != nil {
			text := string(out)
			// 值本来就不存在时不算失败
			if strings.Contains(text, "找不到") || strings.Contains(text, "unable to find") {
				return nil
			}
			return fmt.Errorf("删除启动项失败: %v %s", err, truncate(text, 160))
		}
		return nil
	}

	if !strings.HasPrefix(exePath, "/") && !strings.Contains(exePath, ":\\") {
		return fmt.Errorf("需要主程序的绝对路径")
	}
	if info, err := os.Stat(exePath); err != nil || info.IsDir() {
		return fmt.Errorf("主程序路径不存在：%s", exePath)
	}

	// 路径含空格时 reg.exe 需要带引号的字符串
	value := fmt.Sprintf("\"%s\"", exePath)
	cmd := exec.Command("reg", "add", autostartRunKey, "/v", autostartValueName,
		"/t", "REG_SZ", "/d", value, "/f")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("写入启动项失败: %v %s", err, truncate(string(out), 160))
	}
	return nil
}

func (a *App) handleAutostartGet(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, queryAutostart())
}

func (a *App) handleAutostartSet(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Enabled bool   `json:"enabled"`
		ExePath string `json:"exePath"`
	}
	if err := decodeJSON(r, &request); err != nil {
		fail(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	if err := setAutostart(request.ExePath, request.Enabled); err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	state := queryAutostart()
	writeJSON(w, http.StatusOK, state)
}
