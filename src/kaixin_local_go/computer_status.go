package main

import (
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

// FR-GLB-07 电脑检测：窗口右上角显示硬盘剩余、内存剩余、联网状态。
//
// 温度不显示：Windows 上读 CPU 温度必须加载内核驱动并要管理员授权，
// 本应用走的是用户态、免管理员的安装方式，因此不做温度（需求方已确认去掉）。

type ComputerStatus struct {
	DiskFreeGb  float64 `json:"diskFreeGb"`
	DiskTotalGb float64 `json:"diskTotalGb"`
	DiskName    string  `json:"diskName"`
	MemFreeGb   float64 `json:"memFreeGb"`
	MemTotalGb  float64 `json:"memTotalGb"`
	Online      bool    `json:"online"`
	CheckedAt   string  `json:"checkedAt"`
}

// checkOnline 用 TCP 连接常见公网地址，1.5 秒内连上即视为在线。
func checkOnline() bool {
	targets := []string{"223.5.5.5:443", "119.29.29.29:443"}
	for _, target := range targets {
		conn, err := net.DialTimeout("tcp", target, 1500*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return true
		}
	}
	return false
}

func (a *App) handleComputerStatus(w http.ResponseWriter, r *http.Request) {
	status := ComputerStatus{CheckedAt: time.Now().UTC().Format(time.RFC3339)}

	// 硬盘显示「数据库所在盘」，不是写死的 C:。
	// 数据目录换到 D: 时，这里也要跟着变。
	wantDrive := driveOf(a.dataDir)
	if hw, err := detectHardware(r.Context()); err == nil && hw != nil {
		status.MemTotalGb = hw.MemoryTotalGb
		status.MemFreeGb = hw.MemoryFreeGb
		for _, disk := range hw.Disks {
			if wantDrive == "" || strings.EqualFold(disk.Name, wantDrive) {
				status.DiskFreeGb = disk.FreeGb
				status.DiskTotalGb = disk.TotalGb
				status.DiskName = disk.Name
				break
			}
		}
		// 数据目录在硬件检测没覆盖到的盘上时，直接读该盘容量
		if status.DiskName == "" {
			status.DiskName = wantDrive
			status.DiskFreeGb, status.DiskTotalGb = diskUsage(wantDrive)
		}
	}
	status.Online = checkOnline()

	writeJSON(w, http.StatusOK, status)
}

// driveOf 取路径所在盘符，例如 "C:\\Users\\..." → "C:"。
func driveOf(path string) string {
	if path == "" {
		return ""
	}
	volume := filepath.VolumeName(path)
	if volume == "" {
		return ""
	}
	return strings.ToUpper(volume)
}

// diskUsage 读某个盘符的可用与总量（GB）。读不到返回 0。
func diskUsage(drive string) (float64, float64) {
	if drive == "" {
		return 0, 0
	}
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	getDiskFreeSpaceEx := kernel32.NewProc("GetDiskFreeSpaceExW")
	if err := getDiskFreeSpaceEx.Find(); err != nil {
		return 0, 0
	}
	root, err := syscall.UTF16PtrFromString(drive + "\\\\")
	if err != nil {
		return 0, 0
	}
	var freeBytesAvailable, totalBytes, totalFreeBytes uint64
	ret, _, _ := getDiskFreeSpaceEx.Call(
		uintptr(unsafe.Pointer(root)),
		uintptr(unsafe.Pointer(&freeBytesAvailable)),
		uintptr(unsafe.Pointer(&totalBytes)),
		uintptr(unsafe.Pointer(&totalFreeBytes)),
	)
	if ret == 0 {
		return 0, 0
	}
	const gb = 1024 * 1024 * 1024
	return float64(freeBytesAvailable) / gb, float64(totalBytes) / gb
}
