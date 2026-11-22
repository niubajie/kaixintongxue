//go:build !windows

package main

import "os/exec"

// hideConsole 非 Windows 平台为空实现（本产品仅发布 Windows 桌面端）。
func hideConsole(cmd *exec.Cmd) {}
