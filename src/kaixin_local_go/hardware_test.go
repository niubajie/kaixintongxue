package main

import (
	"context"
	"errors"
	"runtime"
	"testing"
)

const fakeHardwareJSON = `{"CpuName":"Test CPU","Cores":4,"Logical":8,"MemoryTotalKb":16777216,"MemoryFreeKb":8388608,"Caption":"Microsoft Windows 10 专业版","GpuName":"Test GPU","AdapterRam":null,"Disks":[{"DeviceID":"C:","Size":1073741824,"FreeSpace":536870912}]}`

func stubHardwareScript(t *testing.T, stub func(ctx context.Context) ([]byte, error)) {
	t.Helper()
	original := runHardwareScript
	runHardwareScript = stub
	t.Cleanup(func() { runHardwareScript = original })
}

func TestDetectHardwareRetriesOnce(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("硬件检测仅支持 Windows")
	}
	calls := 0
	stubHardwareScript(t, func(ctx context.Context) ([]byte, error) {
		calls++
		if calls == 1 {
			return nil, errors.New("WMI 暂时不可用")
		}
		return []byte(fakeHardwareJSON), nil
	})

	info, err := detectHardware(context.Background())
	if err != nil {
		t.Fatalf("首次失败后应自动重试成功，实际报错: %v", err)
	}
	if calls != 2 {
		t.Fatalf("期望调用 2 次，实际 %d 次", calls)
	}
	if info.CPUName != "Test CPU" || info.CPUCores != 4 || info.MemoryTotalGb != 16 {
		t.Fatalf("解析结果不正确: %+v", info)
	}
	if info.OSCaption != "Microsoft Windows 10 专业版" {
		t.Fatalf("中文系统名称应保持完整，实际 %q", info.OSCaption)
	}
}

func TestDetectHardwareFailsAfterRetries(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("硬件检测仅支持 Windows")
	}
	calls := 0
	stubHardwareScript(t, func(ctx context.Context) ([]byte, error) {
		calls++
		return nil, errors.New("WMI 失败")
	})

	if _, err := detectHardware(context.Background()); err == nil {
		t.Fatal("两次都失败时应返回错误")
	}
	if calls != hardwareAttempts {
		t.Fatalf("期望调用 %d 次，实际 %d 次", hardwareAttempts, calls)
	}
}
