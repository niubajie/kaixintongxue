package main

import (
	"context"
	"net"
	"net/http"
	"os/exec"
	"path/filepath"
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

// readCPUTemp 按可靠度依次尝试：
//  1. LibreHardwareMonitor / OpenHardwareMonitor 的 WMI 命名空间（有真实传感器时最准）
//  2. ACPI 热区（很多台式机主板这块没接传感器，会给固定值）
// 都拿不到可信值时返回 nil 与原因，界面显示「—」，不编数字。
func readCPUTemp() (*float64, string) {
	statusMu.Lock()
	defer statusMu.Unlock()

	if time.Since(cpuTempCachedAt) < 60*time.Second && cpuTempCachedAt != (time.Time{}) {
		return cpuTempCached, cpuTempNote
	}
	cpuTempCachedAt = time.Now()
	cpuTempCached = nil

	if temp, ok := readTempFromHardwareMonitor(); ok {
		cpuTempCached = &temp
		cpuTempNote = "来自硬件监控（LibreHardwareMonitor）"
		return cpuTempCached, cpuTempNote
	}

	temp, note := readTempFromACPI()
	cpuTempCached = temp
	cpuTempNote = note
	return cpuTempCached, cpuTempNote
}

// readTempFromHardwareMonitor 读 LibreHardwareMonitor / OpenHardwareMonitor 的传感器。
// 没装这类工具时直接跳过，不报错。
func readTempFromHardwareMonitor() (float64, bool) {
	script := `
$found = $null
foreach ($ns in @("root/LibreHardwareMonitor", "root/OpenHardwareMonitor")) {
  try {
    $sensors = Get-CimInstance -Namespace $ns -ClassName Sensor -ErrorAction Stop |
      Where-Object { $_.SensorType -eq "Temperature" -and $_.Name -match "CPU|Package|Core" }
    if ($sensors) {
      $found = ($sensors | Measure-Object -Property Value -Maximum).Maximum
      break
    }
  } catch { }
}
if ($found) { "{0:N1}" -f $found }
`
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	output, err := cmd.Output()
	if err != nil {
		return 0, false
	}
	value, parseErr := strconv.ParseFloat(strings.TrimSpace(string(output)), 64)
	if parseErr != nil || value < 20 || value > 110 {
		return 0, false
	}
	return value, true
}

// readTempFromACPI 读 ACPI 热区，并用「临界温度」交叉验证。
// 有些主板不接传感器，会同时给出一个不可能的低临界温度——这种情况整块数据判为无效。
func readTempFromACPI() (*float64, string) {
	script := `
$z = Get-CimInstance -Namespace root/wmi -ClassName MSAcpi_ThermalZoneTemperature -ErrorAction SilentlyContinue
if (-not $z) { return }
$z | ForEach-Object { "{0} {1}" -f $_.CurrentTemperature, $_.CriticalTripPoint }
`
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	output, err := cmd.Output()
	if err != nil || strings.TrimSpace(string(output)) == "" {
		return nil, "本机没有可用的温度传感器（未安装硬件监控工具）"
	}

	best := -1.0
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 2 {
			continue
		}
		current, err1 := strconv.ParseFloat(fields[0], 64)
		critical, err2 := strconv.ParseFloat(fields[1], 64)
		if err1 != nil || err2 != nil {
			continue
		}
		celsius := current/10 - 273.15
		criticalC := critical/10 - 273.15
		// 临界温度低于 60°C 说明这个热区没有接真实传感器，整条丢弃
		if criticalC < 60 {
			continue
		}
		if celsius >= 20 && celsius <= 110 && celsius > best {
			best = celsius
		}
	}

	if best < 0 {
		return nil, "本机 ACPI 热区未接真实传感器（临界温度不合理），读数已丢弃"
	}
	return &best, "来自 ACPI 热区"
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
