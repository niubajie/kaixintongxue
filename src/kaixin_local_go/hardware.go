package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"runtime"
	"time"
)

type HardwareInfo struct {
	CPUName       string     `json:"cpuName"`
	CPUCores      int        `json:"cpuCores"`
	CPULogical    int        `json:"cpuLogical"`
	MemoryTotalGb float64    `json:"memoryTotalGb"`
	MemoryFreeGb  float64    `json:"memoryFreeGb"`
	GPUName       string     `json:"gpuName"`
	GPUVramGb     *float64   `json:"gpuVramGb"` // WMI 读不到时为 null
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
	if raw.AdapterRam != nil && *raw.AdapterRam > 0 {
		vram := float64(*raw.AdapterRam) / gb
		info.GPUVramGb = &vram
	}
	for _, d := range raw.Disks {
		info.Disks = append(info.Disks, DiskInfo{Name: d.DeviceID, TotalGb: d.Size / gb, FreeGb: d.FreeSpace / gb})
	}
	return info, nil
}

// runHardwareScript 单独抽成变量，便于测试重试逻辑。
var runHardwareScript = func(ctx context.Context) ([]byte, error) {
	execCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(execCtx, "powershell", "-NoProfile", "-NonInteractive", "-Command", hardwareScript)
	return cmd.Output()
}

func (a *App) handleHardware(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	info, err := detectHardware(r.Context())
	if err != nil {
		fail(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"hardware": info, "elapsedMs": time.Since(start).Milliseconds()})
}
