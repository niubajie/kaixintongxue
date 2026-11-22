package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

type HardwareInfo struct {
	CPUName       string     `json:"cpuName"`
	CPUCores      int        `json:"cpuCores"`
	CPULogical    int        `json:"cpuLogical"`
	MemoryTotalGb float64    `json:"memoryTotalGb"`
	MemoryFreeGb  float64    `json:"memoryFreeGb"`
	GPUName       string     `json:"gpuName"`
	GPUIntegrated bool       `json:"gpuIntegrated"` // 核显 / APU：与内存共享，显存数值不可信
	GPUVramGb     *float64   `json:"gpuVramGb"`     // WMI 读不到或为核显时为 null
	OSCaption     string     `json:"osCaption"`
	Disks         []DiskInfo `json:"disks"`
	DetectedAt    string     `json:"detectedAt"`
}

type DiskInfo struct {
	Name    string  `json:"name"`
	TotalGb float64 `json:"totalGb"`
	FreeGb  float64 `json:"freeGb"`
}

// 通过 Windows CIM(WMI) 读取，与任务管理器同源。
// 中文系统下 PowerShell 默认按 ANSI(GBK) 输出，先切到无 BOM 的 UTF-8，避免 Go 侧按 UTF-8 解析时乱码。
const hardwareScript = `
[Console]::OutputEncoding = New-Object System.Text.UTF8Encoding($false)
$cpu = Get-CimInstance Win32_Processor | Select-Object -First 1 Name, NumberOfCores, NumberOfLogicalProcessors
$os = Get-CimInstance Win32_OperatingSystem | Select-Object -First 1 Caption, TotalVisibleMemorySize, FreePhysicalMemory
$gpu = Get-CimInstance Win32_VideoController | Where-Object { $_.Name -notmatch 'Remote|Virtual|Microsoft Basic' } | Select-Object -First 1 Name, AdapterRAM
if (-not $gpu) { $gpu = Get-CimInstance Win32_VideoController | Select-Object -First 1 Name, AdapterRAM }
$disks = Get-CimInstance Win32_LogicalDisk -Filter "DriveType=3" | Select-Object DeviceID, Size, FreeSpace
[PSCustomObject]@{
  CpuName = $cpu.Name
  Cores = $cpu.NumberOfCores
  Logical = $cpu.NumberOfLogicalProcessors
  MemoryTotalKb = $os.TotalVisibleMemorySize
  MemoryFreeKb = $os.FreePhysicalMemory
  Caption = $os.Caption
  GpuName = $gpu.Name
  AdapterRam = $gpu.AdapterRAM
  Disks = @($disks)
} | ConvertTo-Json -Depth 4 -Compress
`

type wmiResult struct {
	CpuName       string  `json:"CpuName"`
	Cores         int     `json:"Cores"`
	Logical       int     `json:"Logical"`
	MemoryTotalKb float64 `json:"MemoryTotalKb"`
	MemoryFreeKb  float64 `json:"MemoryFreeKb"`
	Caption       string  `json:"Caption"`
	GpuName       string  `json:"GpuName"`
	AdapterRam    *uint64 `json:"AdapterRam"`
	Disks         []struct {
		DeviceID  string  `json:"DeviceID"`
		Size      float64 `json:"Size"`
		FreeSpace float64 `json:"FreeSpace"`
	} `json:"Disks"`
}

// WMI 偶发失败（如 CIM 服务忙）时自动重试一次，整体不超过前端 30 秒超时。
const hardwareAttempts = 2

func detectHardware(ctx context.Context) (*HardwareInfo, error) {
	if runtime.GOOS != "windows" {
		return nil, fmt.Errorf("当前系统为 %s，硬件检测仅支持 Windows 桌面端", runtime.GOOS)
	}
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	var lastErr error
	for attempt := 1; attempt <= hardwareAttempts; attempt++ {
		info, err := detectHardwareOnce(ctx)
		if err == nil {
			return info, nil
		}
		lastErr = err
		if attempt < hardwareAttempts {
			select {
			case <-ctx.Done():
				return nil, lastErr
			case <-time.After(500 * time.Millisecond):
			}
		}
	}
	return nil, lastErr
}

func detectHardwareOnce(ctx context.Context) (*HardwareInfo, error) {
	output, err := runHardwareScript(ctx)
	if err != nil {
		return nil, fmt.Errorf("调用系统硬件接口失败: %v", err)
	}
	var raw wmiResult
	if err := json.Unmarshal(output, &raw); err != nil {
		return nil, fmt.Errorf("解析硬件信息失败: %v", err)
	}
	const gb = 1024 * 1024 * 1024
	info := &HardwareInfo{
		CPUName:       raw.CpuName,
		CPUCores:      raw.Cores,
		CPULogical:    raw.Logical,
		MemoryTotalGb: raw.MemoryTotalKb * 1024 / gb,
		MemoryFreeGb:  raw.MemoryFreeKb * 1024 / gb,
		GPUName:       raw.GpuName,
		OSCaption:     raw.Caption,
		DetectedAt:    time.Now().Format(time.RFC3339),
	}
	info.GPUIntegrated = isIntegratedGPU(raw.GpuName)
	// 核显/APU 的 AdapterRAM 来自共享内存，WMI 报出的数值不可信（例如 Vega 8 常报 1 GB），
	// 此时不给显存数字，交给上层按"共享内存"处理，避免误导用户。
	if !info.GPUIntegrated && raw.AdapterRam != nil && *raw.AdapterRam > 0 {
		vram := float64(*raw.AdapterRam) / gb
		info.GPUVramGb = &vram
	}
	for _, d := range raw.Disks {
		info.Disks = append(info.Disks, DiskInfo{Name: d.DeviceID, TotalGb: d.Size / gb, FreeGb: d.FreeSpace / gb})
	}
	return info, nil
}

// isIntegratedGPU：按型号名判断是否为核显 / APU。这类显卡没有独立显存。
func isIntegratedGPU(name string) bool {
	lower := strings.ToLower(name)
	for _, marker := range []string{
		"vega", "radeon(tm) graphics", "radeon graphics",
		"uhd graphics", "hd graphics", "iris", "apple m",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

// runHardwareScript 单独抽成变量，便于测试重试逻辑。
var runHardwareScript = func(ctx context.Context) ([]byte, error) {
	execCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(execCtx, "powershell", "-NoProfile", "-NonInteractive", "-Command", hardwareScript)
	hideConsole(cmd)
	return cmd.Output()
}

// 硬件信息在进程内缓存：WMI 检测冷启动可能耗时十几秒，
// 缓存后前端打开技能部署页可以立刻拿到结果，不必每次都看「检测中」。
const hardwareCacheTTL = 10 * time.Minute

var hardwareCache struct {
	mu   sync.Mutex // 同时只允许一次检测，其它请求排队等结果，避免重复拉起 PowerShell
	info *HardwareInfo
	at   time.Time
}

// hardwareSnapshot：命中缓存直接返回；refresh=true 时强制重新检测。
func hardwareSnapshot(ctx context.Context, refresh bool) (*HardwareInfo, bool, error) {
	hardwareCache.mu.Lock()
	defer hardwareCache.mu.Unlock()
	if !refresh && hardwareCache.info != nil && time.Since(hardwareCache.at) < hardwareCacheTTL {
		return hardwareCache.info, true, nil
	}
	info, err := detectHardware(ctx)
	if err != nil {
		// 检测失败时保留旧结果，至少让前端还能显示上次读到的配置。
		if hardwareCache.info != nil && time.Since(hardwareCache.at) < hardwareCacheTTL {
			return hardwareCache.info, true, nil
		}
		return nil, false, err
	}
	hardwareCache.info = info
	hardwareCache.at = time.Now()
	return info, false, nil
}

// warmHardwareCache 在服务启动后后台预热一次，避免用户第一次打开页面时干等。
func warmHardwareCache() {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, _, _ = hardwareSnapshot(ctx, false)
	}()
}

func (a *App) handleHardware(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	refresh := r.URL.Query().Get("refresh") == "1"
	info, cached, err := hardwareSnapshot(r.Context(), refresh)
	if err != nil {
		fail(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"hardware": info, "elapsedMs": time.Since(start).Milliseconds(), "cached": cached,
	})
}
