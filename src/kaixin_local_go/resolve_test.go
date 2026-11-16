package main

import (
	"strings"
	"testing"
)

func testSites(t *testing.T) []SiteEntry {
	t.Helper()
	sites, err := loadSites()
	if err != nil {
		t.Fatalf("加载站点失败: %v", err)
	}
	if len(sites) == 0 {
		t.Fatal("站点目录为空")
	}
	return sites
}

// 需求里同时提到「店铺」「销量」「评论」，应落到抖店的商品分析或评价管理，并先给登录步骤。
func TestResolveShopSalesAndComments(t *testing.T) {
	sites := testSites(t)
	result := ResolveNeed(sites, "查看店铺每天的销量以及评论")

	if result.Site == nil {
		t.Fatal("应该识别出抖店")
	}
	if result.Site.ID != "douyin-shop" {
		t.Fatalf("站点识别错误，得到 %s", result.Site.ID)
	}
	if result.Intent != string(IntentRead) {
		t.Fatalf("意图应为 read，得到 %s", result.Intent)
	}
	if len(result.Steps) < 2 {
		t.Fatalf("应先登录再打开页面，实际步骤数 %d", len(result.Steps))
	}
	if result.Steps[0].Kind != "login" {
		t.Fatalf("第一步应为登录，得到 %s", result.Steps[0].Kind)
	}
	last := result.Steps[len(result.Steps)-1]
	if last.Kind != "open-page" {
		t.Fatalf("最后一步应为打开页面，得到 %s", last.Kind)
	}
	if last.URL == "" || !strings.HasPrefix(last.URL, "https://") {
		t.Fatalf("页面地址异常: %s", last.URL)
	}
	if last.SuccessWhen == "" {
		t.Fatal("必须有到位判据（successWhen）")
	}
}

// 播放量/点赞量应落到创作服务平台的作品数据，而不是内容端首页。
func TestResolveVideoStats(t *testing.T) {
	sites := testSites(t)
	result := ResolveNeed(sites, "查看我发布的短视频有多少播放量和点赞量")

	if result.Site == nil || result.Site.ID != "douyin-creator" {
		t.Fatalf("应识别为创作服务平台，得到 %+v", result.Site)
	}
	last := result.Steps[len(result.Steps)-1]
	if !strings.Contains(last.URL, "/content/manage") {
		t.Fatalf("应落到作品数据页，实际 %s", last.URL)
	}
	if !strings.Contains(last.Why, "播放量") {
		t.Fatalf("应说明命中的关键词，实际 %q", last.Why)
	}
}

// 发布类需求必须落到发布页，不能被数据页抢走。
func TestResolvePublishGoesToUploadPage(t *testing.T) {
	sites := testSites(t)
	result := ResolveNeed(sites, "把做好的视频发布到抖音")

	if result.Intent != string(IntentPublish) {
		t.Fatalf("意图应为 publish，得到 %s", result.Intent)
	}
	last := result.Steps[len(result.Steps)-1]
	if !strings.Contains(last.URL, "/content/upload") {
		t.Fatalf("应落到发布页，实际 %s", last.URL)
	}
}

// 没有登记页面的站点，应退化成站内搜索，而不是直接给首页。
func TestResolveFallsBackToSiteSearch(t *testing.T) {
	sites := testSites(t)
	result := ResolveNeed(sites, "在哔哩哔哩看看视频播放量")

	if result.Site == nil {
		t.Fatal("应识别出 B 站")
	}
	hasSearch := false
	for _, step := range result.Steps {
		if step.Kind == "search" && strings.Contains(step.URL, "search.bilibili.com") {
			hasSearch = true
		}
	}
	if !hasSearch {
		t.Fatalf("应退化为站内搜索，实际步骤 %+v", result.Steps)
	}
}

// 识别不出站点时，必须明确告诉用户补充平台，而不是瞎猜一个。
func TestResolveUnknownSiteGivesAdvice(t *testing.T) {
	sites := testSites(t)
	result := ResolveNeed(sites, "帮我写一篇小红书风格的文案并统计")

	// 这里命中的是小红书，但需求本身不是「站点操作」，应给出可执行步骤或建议
	if result.Site == nil && result.Advice == "" {
		t.Fatal("识别不到站点时必须给出建议")
	}

	empty := ResolveNeed(sites, "")
	if empty.Advice == "" {
		t.Fatal("空需求应给提示")
	}
}

// 页面地图的数据必须完整：每条页面的 url / successWhen / keywords 都不能为空。
func TestSitePagesAreWellFormed(t *testing.T) {
	sites := testSites(t)
	seenKeys := map[string]bool{}
	for _, site := range sites {
		if site.ID == "" || site.Name == "" || site.LoginURL == "" {
			t.Fatalf("站点基础字段缺失: %+v", site)
		}
		for _, page := range site.Pages {
			if page.Key == "" || page.Title == "" || page.URL == "" {
				t.Fatalf("%s 的页面缺少基础字段: %+v", site.Name, page)
			}
			if page.SuccessWhen == "" {
				t.Fatalf("%s/%s 缺少 successWhen，程序无法判断是否到位", site.Name, page.Key)
			}
			if len(page.Keywords) == 0 {
				t.Fatalf("%s/%s 缺少关键词，无法被需求命中", site.Name, page.Key)
			}
			id := site.ID + "/" + page.Key
			if seenKeys[id] {
				t.Fatalf("页面 key 重复: %s", id)
			}
			seenKeys[id] = true
		}
	}
}
