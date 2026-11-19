package main

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type PlanStep struct {
	Order            int      `json:"order"`
	SoftwareID       string   `json:"softwareId"`
	Name             string   `json:"name"`
	Action           string   `json:"action"` // extract / pull-model / manual
	SizeMb           int      `json:"sizeMb"`
	URL              string   `json:"url,omitempty"`
	Requires         []string `json:"requires"`
	AlreadyInstalled bool     `json:"alreadyInstalled"`
	Note             string   `json:"note"`
}

type Plan struct {
	Steps            []PlanStep     `json:"steps"`
	AuthSteps        []PlanSiteAuth `json:"authSteps"`
	TotalSizeMb      int            `json:"totalSizeMb"`
	EstimatedMinutes int            `json:"estimatedMinutes"`
	Warnings         []string       `json:"warnings"`
}

// PlanSiteAuth：技能涉及指定网站时的授权步骤，已授权的不再重复登录。
type PlanSiteAuth struct {
	SiteID     string `json:"siteId"`
	Name       string `json:"name"`
	URL        string `json:"url"`
	LoginURL   string `json:"loginUrl"`
	Note       string `json:"note"`
	Authorized bool   `json:"authorized"`
	ExpiresAt  string `json:"expiresAt,omitempty"`
}

// buildAuthSteps：按请求顺序去重，并检查本机是否已有有效授权记录。
func buildAuthSteps(sites []SiteEntry, ids []string, authDir string) []PlanSiteAuth {
	steps := []PlanSiteAuth{}
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] {
			continue
		}
		site := findSite(sites, id)
		if site == nil {
			continue
		}
		seen[id] = true
		authorized, expiresAt := readAuthMarker(authDir, site.ID)
		note := site.Note
		expiresAtStr := ""
		if !expiresAt.IsZero() {
			expiresAtStr = expiresAt.Format(time.RFC3339)
		}
		if !authorized && !expiresAt.IsZero() {
			note = "授权已于 " + expiresAt.Format("2006-01-02") + " 过期，请重新授权。" + site.Note
		}
		steps = append(steps, PlanSiteAuth{
			SiteID: site.ID, Name: site.Name, URL: site.URL, LoginURL: site.LoginURL,
			Note: note, Authorized: authorized, ExpiresAt: expiresAtStr,
		})
	}
	return steps
}

// BuildPlan：展开软件集合 → 去重 → 补齐依赖并拓扑排序 → 汇总与冲突检测。
func BuildPlan(entries []SoftwareEntry, skills []SkillSelection, extra []string, hw *HardwareInfo, runtimeRoot string) (*Plan, error) {
	wanted := map[string]bool{}
	warnings := []string{}
	for _, skill := range skills {
		if len(skill.SoftwareIDs) == 0 {
			warnings = append(warnings, fmt.Sprintf("技能「%s」未匹配到软件：可换个说法重新匹配，或手动添加软件。", skill.Name))
		}
		for _, id := range skill.SoftwareIDs {
			if findSoftware(entries, id) == nil {
				return nil, fmt.Errorf("未知软件：%s", id)
			}
			wanted[id] = true
		}
	}
	for _, id := range extra {
		if findSoftware(entries, id) == nil {
			return nil, fmt.Errorf("未知软件：%s", id)
		}
		wanted[id] = true
	}
	// 补齐依赖闭包。
	var addRequires func(id string) error
	addRequires = func(id string) error {
		entry := findSoftware(entries, id)
		if entry == nil {
			return fmt.Errorf("未知软件：%s", id)
		}
		for _, req := range entry.Requires {
			if !wanted[req] {
				wanted[req] = true
				if err := addRequires(req); err != nil {
					return err
				}
			}
		}
		return nil
	}
	for id := range wanted {
		if err := addRequires(id); err != nil {
			return nil, err
		}
	}
	if len(wanted) == 0 {
		return nil, fmt.Errorf("没有可部署的软件：请先在技能内容里配置 AI 软件，或手动添加。")
	}

	// Kahn 拓扑排序，保证先装运行时再装模型。
	ordered := []string{}
	indegree := map[string]int{}
	dependents := map[string][]string{}
	for id := range wanted {
		entry := findSoftware(entries, id)
		count := 0
		for _, req := range entry.Requires {
			if wanted[req] {
				count++
				dependents[req] = append(dependents[req], id)
			}
		}
		indegree[id] = count
	}
	queue := []string{}
	for id, degree := range indegree {
		if degree == 0 {
			queue = append(queue, id)
		}
	}
	sort.Strings(queue)
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		ordered = append(ordered, id)
		next := dependents[id]
		sort.Strings(next)
		for _, child := range next {
			indegree[child]--
			if indegree[child] == 0 {
				queue = append(queue, child)
			}
		}
		sort.Strings(queue)
	}
	if len(ordered) != len(wanted) {
		return nil, fmt.Errorf("软件依赖存在循环，请检查目录配置")
	}

	plan := &Plan{}
	maxMemory := 0
	maxVram := 0
	for _, id := range ordered {
		entry := findSoftware(entries, id)
		action := "extract"
		stepNote := entry.Note
		// AGPL 家族许可证或需要驱动/系统服务的软件，禁止自动部署，走手动引导。
		if entry.isAGPL() {
			action = "manual"
			stepNote = "许可证为 " + entry.License + "，需手动安装：" + entry.Note
			if entry.Install.Fallback != "" {
				stepNote += "（请访问 " + entry.Install.Fallback + "）"
			}
		} else {
			switch entry.Install.Type {
			case "cli":
				action = "pull-model"
			case "manual":
				action = "manual"
			}
		}
		step := PlanStep{
			Order:      len(plan.Steps) + 1,
			SoftwareID: entry.ID,
			Name:       entry.Name,
			Action:     action,
			SizeMb:     entry.SizeMb,
			URL:        entry.Install.primaryURL(),
			Requires:   entry.Requires,
			Note:       stepNote,
		}
		if _, err := os.Stat(filepath.Join(runtimeRoot, entry.ID, "manifest.json")); err == nil {
			step.AlreadyInstalled = true
		}
		plan.Steps = append(plan.Steps, step)
		if !step.AlreadyInstalled {
			plan.TotalSizeMb += entry.SizeMb
		}
		if entry.MinMemoryGb > maxMemory {
			maxMemory = entry.MinMemoryGb
		}
		if entry.MinVramGb > maxVram {
			maxVram = entry.MinVramGb
		}
	}
	// 按 20 MB/s 估算下载耗时，另加 1 分钟解压与校验。
	plan.EstimatedMinutes = plan.TotalSizeMb/20/60 + 1

	// 目录里没配 SHA-256 的软件，安装时无法校验完整性，提前提示。
	for _, id := range ordered {
		entry := findSoftware(entries, id)
		if entry != nil && entry.Install.Type == "portable-zip" && entry.Install.SHA256 == "" {
			warnings = append(warnings, fmt.Sprintf("%s 未配置校验值，安装时将跳过完整性校验。", entry.Name))
		}
	}

	if hw != nil {
		if hw.GPUIntegrated {
			warnings = append(warnings, fmt.Sprintf("检测到集成显卡（%s，与内存共享）：本地大模型建议使用独立显卡，或改用云端模型。", hw.GPUName))
		}
		needGb := float64(plan.TotalSizeMb) * 2 / 1024 // 下载 + 解压预留两倍空间
		for _, disk := range hw.Disks {
			if disk.Name == "C:" && disk.FreeGb < needGb {
				warnings = append(warnings, fmt.Sprintf("磁盘空间不足：本方案约需 %.1f GB（含解压临时空间），C: 仅剩 %.1f GB。", needGb, disk.FreeGb))
			}
		}
		if maxVram > 0 && !hw.GPUIntegrated {
			if hw.GPUVramGb != nil && *hw.GPUVramGb < float64(maxVram) {
				warnings = append(warnings, fmt.Sprintf("当前显存 %.1f GB，低于方案建议的 %d GB，本地大模型可能运行缓慢或无法加载。", *hw.GPUVramGb, maxVram))
			} else if hw.GPUVramGb == nil {
				warnings = append(warnings, fmt.Sprintf("未能读取显存容量：方案建议显存 %d GB，请自行确认。", maxVram))
			}
		}
		if maxMemory > 0 && hw.MemoryTotalGb < float64(maxMemory) {
			warnings = append(warnings, fmt.Sprintf("当前内存 %.1f GB，低于方案建议的 %d GB。", hw.MemoryTotalGb, maxMemory))
		}
	}
	plan.Warnings = warnings
	// FR-SKL-07：本地硬件不足以流畅运行本地模型时，补三个云端模型
	// （免费 / 有免费额度 / 收费最强各一个），由用户按需选用其一。
	if hw != nil && containsLocalModel(entries, plan.Steps) && localInsufficient(hw) {
		added := 0
		for _, id := range []string{"cloud-free", "cloud-quota", "cloud-best"} {
			entry := findSoftware(entries, id)
			if entry == nil {
				continue
			}
			plan.Steps = append(plan.Steps, PlanStep{
				Order:      len(plan.Steps) + 1,
				SoftwareID: entry.ID,
				Name:       entry.Name,
				Action:     "cloud",
				SizeMb:     0,
				Requires:   entry.Requires,
				Note:       entry.Note,
			})
			added++
		}
		if added > 0 {
			warnings = append(warnings, fmt.Sprintf(
				"本地硬件不足以流畅运行所选模型，已补充 %d 个云端模型（免费 / 有免费额度 / 收费最强），选其一填好 API Key 即可。",
				added))
			plan.Warnings = warnings
		}
	}

	return plan, nil
}

type SkillSelection struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	SoftwareIDs []string `json:"softwareIds"`
}

func (a *App) handlePlan(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Skills           []SkillSelection `json:"skills"`
		ExtraSoftwareIDs []string         `json:"extraSoftwareIds"`
		SiteIDs          []string         `json:"siteIds"`
	}
	if err := decodeJSON(r, &request); err != nil {
		fail(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	hw, err := detectHardware(r.Context())
	if err != nil {
		hw = nil // 硬件读取失败不阻断方案，只做不了冲突检测
	}
	plan, err := BuildPlan(a.catalog, request.Skills, request.ExtraSoftwareIDs, hw, a.runtimeRoot)
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	plan.AuthSteps = buildAuthSteps(a.sites, request.SiteIDs, a.authDir)
	writeJSON(w, http.StatusOK, plan)
}

// containsLocalModel：方案里是否包含需要下载的本地模型。
func containsLocalModel(entries []SoftwareEntry, steps []PlanStep) bool {
	for _, step := range steps {
		entry := findSoftware(entries, step.SoftwareID)
		if entry == nil || entry.Kind != "model" {
			continue
		}
		if entry.Install.Type != "cloud" {
			return true
		}
	}
	return false
}

// localInsufficient：判定本地是否跑不动本地模型。
// 集成显卡、显存不足 6 GB、内存不足 16 GB 任一成立即视为不够。
func localInsufficient(hw *HardwareInfo) bool {
	if hw == nil {
		return false
	}
	if hw.GPUIntegrated {
		return true
	}
	if hw.GPUVramGb != nil && *hw.GPUVramGb > 0 && *hw.GPUVramGb < 6 {
		return true
	}
	if hw.MemoryTotalGb > 0 && hw.MemoryTotalGb < 16 {
		return true
	}
	return false
}
