package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlanDedupAndTopology(t *testing.T) {
	entries := testCatalog(t)
	skills := []SkillSelection{
		{ID: "a", Name: "写作助手", SoftwareIDs: []string{"qwen2.5-7b"}},
		{ID: "b", Name: "问答助手", SoftwareIDs: []string{"qwen2.5-7b", "ollama"}},
	}
	plan, err := BuildPlan(entries, skills, nil, nil, t.TempDir())
	if err != nil {
		t.Fatalf("生成方案失败: %v", err)
	}
	if len(plan.Steps) != 2 {
		t.Fatalf("重叠软件应只出现一次，实际 %d 步", len(plan.Steps))
	}
	if plan.Steps[0].SoftwareID != "ollama" || plan.Steps[1].SoftwareID != "qwen2.5-7b" {
		t.Fatalf("拓扑顺序错误：%+v", plan.Steps)
	}
	if plan.Steps[0].Action != "extract" || plan.Steps[1].Action != "pull-model" {
		t.Fatalf("动作类型错误：%+v", plan.Steps)
	}
	if plan.TotalSizeMb != 5500 {
		t.Fatalf("体积汇总错误：%d", plan.TotalSizeMb)
	}
}

func TestPlanWarnings(t *testing.T) {
	entries := testCatalog(t)
	vram := 4.0
	hw := &HardwareInfo{
		MemoryTotalGb: 8,
		GPUVramGb:     &vram,
		Disks:         []DiskInfo{{Name: "C:", TotalGb: 100, FreeGb: 1}},
	}
	skills := []SkillSelection{{ID: "a", Name: "写作", SoftwareIDs: []string{"qwen2.5-7b"}}}
	plan, err := BuildPlan(entries, skills, nil, hw, t.TempDir())
	if err != nil {
		t.Fatalf("生成方案失败: %v", err)
	}
	joined := strings.Join(plan.Warnings, "|")
	for _, keyword := range []string{"磁盘", "显存", "内存"} {
		if !strings.Contains(joined, keyword) {
			t.Fatalf("缺少%s警告：%s", keyword, joined)
		}
	}
}

func TestPlanUnmatchedSkillWarning(t *testing.T) {
	entries := testCatalog(t)
	skills := []SkillSelection{{ID: "a", Name: "未知技能", SoftwareIDs: nil}}
	_, err := BuildPlan(entries, skills, nil, nil, t.TempDir())
	if err == nil {
		t.Fatal("完全没有软件时应返回错误提示")
	}
	skills = append(skills, SkillSelection{ID: "b", Name: "剪辑", SoftwareIDs: []string{"ffmpeg"}})
	plan, err := BuildPlan(entries, skills, nil, nil, t.TempDir())
	if err != nil {
		t.Fatalf("生成方案失败: %v", err)
	}
	if !strings.Contains(strings.Join(plan.Warnings, "|"), "未知技能") {
		t.Fatalf("应提示未匹配的技能：%+v", plan.Warnings)
	}
}

func TestBuildAuthStepsDedupAndAuthorized(t *testing.T) {
	sites, err := loadSites()
	if err != nil {
		t.Fatalf("加载网站目录失败: %v", err)
	}
	authDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(authDir, "douyin.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	steps := buildAuthSteps(sites, []string{"douyin", "xiaohongshu", "douyin", "unknown"}, authDir)
	if len(steps) != 2 {
		t.Fatalf("应去重并忽略未知网站，实际 %+v", steps)
	}
	if !steps[0].Authorized {
		t.Fatal("抖音已写入授权记录，应标记为已授权")
	}
	if steps[1].Authorized {
		t.Fatal("小红书未授权，不应标记为已授权")
	}
	if steps[1].LoginURL == "" {
		t.Fatal("授权步骤应带登录地址")
	}
}
