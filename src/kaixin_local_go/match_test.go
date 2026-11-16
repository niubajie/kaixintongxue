package main

import (
	"strings"
	"testing"
)

func testCatalog(t *testing.T) []SoftwareEntry {
	entries, err := loadCatalog()
	if err != nil {
		t.Fatalf("加载目录失败: %v", err)
	}
	return entries
}

func TestMatchOfflineLLM(t *testing.T) {
	entries := testCatalog(t)
	result := MatchContent(entries, "帮我做一个本地离线跑大模型的工具")
	if len(result.Lines) != 1 {
		t.Fatalf("期望 1 行，实际 %d", len(result.Lines))
	}
	line := result.Lines[0]
	if line.SoftwareID != "ollama" && line.SoftwareID != "qwen2.5-7b" {
		t.Fatalf("期望匹配本地大模型，实际 %+v", line)
	}
	if len(line.Reasons) == 0 {
		t.Fatal("必须给出匹配理由")
	}
	if !strings.Contains(result.Content, "【") {
		t.Fatalf("输出应带【软件名】：%s", result.Content)
	}
}

func TestMatchVideoAndSpeech(t *testing.T) {
	entries := testCatalog(t)
	result := MatchContent(entries, "剪辑视频\n把录音转成文字")
	if result.Lines[0].SoftwareID != "ffmpeg" {
		t.Fatalf("剪辑视频应匹配 FFmpeg，实际 %s", result.Lines[0].Software)
	}
	if result.Lines[1].SoftwareID != "whisper" {
		t.Fatalf("录音转文字应匹配 Whisper，实际 %s", result.Lines[1].Software)
	}
}

func TestMatchNone(t *testing.T) {
	entries := testCatalog(t)
	result := MatchContent(entries, "asdfqwerzxcv乱码")
	if result.Lines[0].Software != "无" {
		t.Fatalf("乱码应显示无，实际 %s", result.Lines[0].Software)
	}
	if !strings.HasSuffix(result.Content, "【无】") {
		t.Fatalf("无匹配行应以【无】结尾：%s", result.Content)
	}
}

func TestMatchIdempotent(t *testing.T) {
	entries := testCatalog(t)
	first := MatchContent(entries, "剪辑视频")
	second := MatchContent(entries, first.Content)
	if first.Content != second.Content {
		t.Fatalf("重复匹配不应追加括号：%s vs %s", first.Content, second.Content)
	}
	if second.Lines[0].SoftwareID != "ffmpeg" {
		t.Fatalf("重复匹配应重新给出软件，实际 %+v", second.Lines[0])
	}
}

func TestMatchUpdatesStaleSuffix(t *testing.T) {
	entries := testCatalog(t)
	result := MatchContent(entries, "剪辑视频【Ollama】")
	if result.Lines[0].SoftwareID != "ffmpeg" {
		t.Fatalf("旧标记应被重新匹配为 FFmpeg，实际 %+v", result.Lines[0])
	}
	if result.Content != "剪辑视频【FFmpeg】" {
		t.Fatalf("应替换旧标记，实际 %s", result.Content)
	}
}

func TestMatchSiteAppendsURL(t *testing.T) {
	entries := testCatalog(t)
	sites, err := loadSites()
	if err != nil {
		t.Fatalf("加载网站目录失败: %v", err)
	}
	result := MatchContentWithSites(entries, sites, "到抖音自动发布商品视频")
	line := result.Lines[0]
	if line.SiteID != "douyin" || line.Site != "抖音" {
		t.Fatalf("应识别抖音，实际 %+v", line)
	}
	if line.SiteURL != "https://www.douyin.com" {
		t.Fatalf("网站网址错误：%s", line.SiteURL)
	}
	if !strings.HasSuffix(result.Content, "https://www.douyin.com") {
		t.Fatalf("网址应追加在技能说明后面：%s", result.Content)
	}

	// 重复匹配不应再次追加网址或括号。
	second := MatchContentWithSites(entries, sites, result.Content)
	if second.Content != result.Content {
		t.Fatalf("重复匹配不应重复追加：%s vs %s", result.Content, second.Content)
	}
	if second.Lines[0].SiteID != "douyin" {
		t.Fatalf("重复匹配应保留网站识别，实际 %+v", second.Lines[0])
	}
}

func TestMatchSitePrefersLongestKeyword(t *testing.T) {
	sites, err := loadSites()
	if err != nil {
		t.Fatalf("加载网站目录失败: %v", err)
	}
	if site := matchSite(sites, "发布到微信公众号"); site == nil || site.ID != "wechat-mp" {
		t.Fatalf("微信公众号不应被「微信」抢占：%+v", site)
	}
}
