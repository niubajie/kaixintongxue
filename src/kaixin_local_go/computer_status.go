package main

import (
	"context"
	"net"
	"net/http"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

// FR-GLB-07 电脑检测：窗口右上角显示 CPU 温度、硬盘剩余、内存剩余、联网状态。
//
// 温度在台式机上常常读不到（没有传感器驱动），这时返回 null，
// 界面显示「—」并说明原因，不编造一个数字。

type ComputerStatus struct {
	CPUTempC    *float64 `json:"cpuTempC"`
	DiskFreeGb  float64  `json:"diskFreeGb"`
	DiskTotalGb float64  `json:"diskTotalGb"`
	DiskName    string   `json:"diskName"`
	MemFreeGb   float64  `json:"memFreeGb"`
	MemTotalGb  float64  `json:"memTotalGb"`
	Online      bool     `json:"online"`
	CPUTempNote string   `json:"cpuTempNote"`
	CheckedAt   string   `json:"checkedAt"`
}

var (
	statusMu        sync.Mutex
	cpuTempCached   *float64
	cpuTempCachedAt time.Time
	cpuTempNote     = "尚未读取"
)

// readCPUTemp 通过 WMI 读 ACPI 温度区。结果缓存 60 秒，失败也缓存，
// 避免每 15 秒拉一次 PowerShell。
func readCPUTemp() (*float64, string) {
	statusMu.Lock()
	defer statusMu.Unlock()

	if time.Since(cpuTempCachedAt) < 60*time.Second && cpuTempCachedAt != (time.Time{}) {
		return cpuTempCached, cpuTempNote
	}

	cpuTempCachedAt = time.Now()
	cpuTempCached = nil
	cpuTempNote = "本机未提供温度传感器数据（台式机常见）"

	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command",
		"(Get-CimInstance -Namespace root/wmi -ClassName MSAcpi_ThermalZoneTemperature -ErrorAction Stop).CurrentTemperature")
	output, err := cmd.Output()
	if err != nil {
		cpuTempNote = "读取温度失败：需要管理员权限或本机不支持 ACPI 温度"
		return nil, cpuTempNote
	}

	// 输出可能是多行（多个热区），取第一行
	text := strings.TrimSpace(string(output))
	if idx := strings.IndexAny(text, "\r\n"); idx >= 0 {
		text = strings.TrimSpace(text[:idx])
	}
	raw, parseErr := strconv.ParseFloat(regexp.MustCompile(`-?\d+(\.\d+)?`).FindString(text), 64)
	if parseErr != nil || raw <= 0 {
		cpuTempNote = "温度数据格式无法识别"
		return nil, cpuTempNote
	}
	// ACPI 返回的是开尔文的十分之一
	celsius := raw/10 - 273.15
	// 台式机主板的 ACPI 热区经常给出无意义读数（例如开机就 17°C）。
	// 低于 20°C 或高于 110°C 一律视为不可信，宁可不显示也不误导。
	if celsius < 20 || celsius > 110 {
		cpuTempNote = "本机温度读数不可信（ACPI 热区异常），已忽略"
		return nil, cpuTempNote
	}
	cpuTempCached = &celsius
	cpuTempNote = "来自 ACPI 热区"
	return cpuTempCached, cpuTempNote
}

// checkOnline 用 TCP 连接常见公网地址，2 秒内连上即视为在线。
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

	// 内存与磁盘复用硬件检测的缓存结果（10 分钟内不重复检测）
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

	if temp, note := readCPUTemp(); temp != nil {
		status.CPUTempC = temp
		status.CPUTempNote = note
	} else {
		status.CPUTempNote = note
	}
	status.Online = checkOnline()

	writeJSON(w, http.StatusOK, status)
}

// driveOf 取路径所在盘符，例如 "C:\Users\..." → "C:"。
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
	root, err := syscall.UTF16PtrFromString(drive + "\\")
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
