//go:build windows

package main

import (
	"os/exec"
	"syscall"
)

// hideConsole 让 sidecar 内部静默调用的 CLI 子进程不弹出控制台窗口。
// CREATE_NO_WINDOW (0x08000000) 不创建新控制台，HideWindow 隐藏可能出现的窗口。
// 仅用于 powershell / reg / ffmpeg / whisper 这类后台静默命令；
// 需要用户交互的第三方安装程序不要调用本函数。
func hideConsole(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000,
	}
}
