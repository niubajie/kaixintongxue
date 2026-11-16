package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
)

//go:embed catalog.json
var catalogJSON []byte

type InstallSpec struct {
	Type     string   `json:"type"`               // portable-zip / cli / manual
	URL      string   `json:"url,omitempty"`      // 兼容旧数据，等价于 Sources[0]
	Sources  []string `json:"sources,omitempty"`  // 多个下载源，按顺序尝试
	Fallback string   `json:"fallback,omitempty"` // 全部下载失败时引导用户访问的页面
	SHA256   string   `json:"sha256,omitempty"`
	Command  string   `json:"command,omitempty"`
}

// primaryURL 返回第一个可用的下载地址。
func (s InstallSpec) primaryURL() string {
	for _, u := range s.Sources {
		if u != "" {
			return u
		}
	}
	return s.URL
}

// allURLs 返回所有可用下载源，用于逐个尝试。
func (s InstallSpec) allURLs() []string {
	urls := make([]string, 0, len(s.Sources)+1)
	for _, u := range s.Sources {
		if u != "" {
			urls = append(urls, u)
		}
	}
	if s.URL != "" {
		urls = append(urls, s.URL)
	}
	return urls
}

type VerifySpec struct {
	Command string `json:"command"`
}

type SoftwareEntry struct {
	ID           string      `json:"id"`
	Name         string      `json:"name"`
	Kind         string      `json:"kind"` // runtime / model / tool
	License      string      `json:"license,omitempty"`
	Capabilities []string    `json:"capabilities"`
	Keywords     []string    `json:"keywords"`
	SizeMb       int         `json:"sizeMb"`
	Requires     []string    `json:"requires"`
	MinMemoryGb  int         `json:"minMemoryGb"`
	MinVramGb    int         `json:"minVramGb"`
	Install      InstallSpec `json:"install"`
	Verify       VerifySpec  `json:"verify"`
	Note         string      `json:"note"`
}

// isAGPL 判定许可证是否属于 AGPL 家族（AGPL-3.0、AGPL-3.0-only 等）。
// AGPL 软件不应自动下载安装，必须走 manual 引导，避免开源传染风险。
func (e *SoftwareEntry) isAGPL() bool {
	lic := strings.ToLower(strings.TrimSpace(e.License))
	return strings.Contains(lic, "agpl")
}

func loadCatalog() ([]SoftwareEntry, error) {
	var entries []SoftwareEntry
	if err := json.Unmarshal(catalogJSON, &entries); err != nil {
		return nil, fmt.Errorf("解析软件目录失败: %w", err)
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("软件目录为空")
	}
	return entries, nil
}

func findSoftware(entries []SoftwareEntry, id string) *SoftwareEntry {
	for i := range entries {
		if entries[i].ID == id {
			return &entries[i]
		}
	}
	return nil
}
