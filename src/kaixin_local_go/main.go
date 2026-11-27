package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// 开心同学 · 本地服务：默认只监听 127.0.0.1，显式开启 LAN 模式后允许令牌鉴权的局域网访问。
type App struct {
	runtimeRoot string
	downloadDir string
	root        string
	dataDir     string
	store       *Store
	// 定时搜索的运行态（phase 字符串），只在内存里，重启即清空。
	scheduleRuns sync.Map
	// 定时搜索的取消函数：暂停时调 cancel() 可立即打断正在进行的 HTTP 下载。
	scheduleCancels sync.Map
	// 自动修复后的待处理事项（需要登录 / 修复失败），任务结束后仍保留给前端处理。
	scheduleIssues sync.Map
	// 修复向导在站内找到的可下载结果，恢复任务时优先消费，避免再次走失效搜索路径。
	scheduleAssistResults sync.Map
	// 全流程下载监督：记录当前正在处理的网站，异常时据此进入修复。
	scheduleCurrentSite sync.Map // scheduleID -> siteID
	// 本轮下载预算的指针，监督程序据此判断是否还有下载空间。
	scheduleBudget sync.Map // scheduleID -> *downloadBudget
	// 每次运行 / 每次手动重试都自增，前端用它区分“同一条错误的新一轮”，避免去重导致不再弹修复。
	scheduleRunNonce sync.Map // scheduleID -> int
	// 监督程序发现下载停滞时置位，runScheduleSite 在循环里主动让出进入修复。
	scheduleRepairRequested sync.Map // scheduleID -> bool
	// 监督程序记录最近一次有进展的时间，用来判断停滞。
	scheduleLastProgress sync.Map // scheduleID -> progressStamp
	// 修复向导产出结果后关闭此通道，runScheduleSite 的等待循环被唤醒去消费结果。
	scheduleAssistWait sync.Map // assistKey -> chan struct{}
	// 同一任务连续自动重开修复向导的次数，达到上限后判定放弃，避免无限弹窗。
	scheduleAssistReopens sync.Map // scheduleID -> int
}

func main() {
	listenCfg, err := loadListenConfig()
	if err != nil {
		log.Fatalf("%v", err)
	}
	// KAIXIN_HOME 可整体改运行目录，便于排障与测试
	root := ""
	if custom := os.Getenv("KAIXIN_HOME"); custom != "" {
		root = custom
	} else {
		base := os.Getenv("LOCALAPPDATA")
		if base == "" {
			base = "."
		}
		root = filepath.Join(base, "Kaixin")
	}
	app := &App{
		runtimeRoot: filepath.Join(root, "runtime"),
		downloadDir: filepath.Join(root, "downloads"),
	}
	app.root = root
	app.dataDir = filepath.Join(root, "data")
	for _, dir := range []string{app.runtimeRoot, app.downloadDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			log.Fatalf("创建目录失败 %s: %v", dir, err)
		}
	}

	// 本地库：所有持久化数据的唯一落点（docs/08 第 11 节）
	// 数据目录：环境变量 > 用户选过的位置 > 默认 root\data
	dataDir, firstRun := resolveDataDir(root)
	firstRunPending = firstRun
	app.dataDir = dataDir
	store, err := OpenStore(dataDir)
	if err != nil {
		// 建库失败不阻断整个服务：硬件检测、匹配、方案生成都还能用，
		// 只有数据接口返回 503。这样用户至少不会被"整个程序不能用"卡住。
		log.Printf("[WARN] 本地库初始化失败（数据接口将不可用）: %v", err)
	} else {
		defer store.Close()
		app.store = store
		// 把已保存的站点登录态读进内存，正文抓取时直接带上 Cookie。
		app.loadSiteCookies(context.Background())
		log.Printf("本地库就绪: %s", filepath.Join(dataDir, "app.db"))
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "kaixin-local"})
	})
	mux.HandleFunc("GET /api/system/hardware", app.handleHardware)
	mux.HandleFunc("GET /api/data/info", app.handleDataLocationInfo)
	mux.HandleFunc("POST /api/data/ack", app.handleDataAck)
	mux.HandleFunc("POST /api/data/move", app.handleDataMove)
	mux.HandleFunc("GET /api/library/scan", app.handleLibraryScan)
	mux.HandleFunc("POST /api/library/open", app.handleLibraryOpen)
	mux.HandleFunc("POST /api/library/open-url", app.handleLibraryOpenURL)
	mux.HandleFunc("POST /api/library/open-folder", app.handleLibraryOpenFolder)
	mux.HandleFunc("POST /api/library/set-dir", app.handleLibrarySetDir)
	mux.HandleFunc("GET /api/library/taxonomy", app.handleLibraryTaxonomy)
	mux.HandleFunc("POST /api/library/collect", app.handleWebCollect)
	mux.HandleFunc("GET /api/library/pending", app.handlePendingList)
	mux.HandleFunc("POST /api/library/pending/approve", app.handlePendingApprove)
	mux.HandleFunc("POST /api/library/pending/reject", app.handlePendingReject)
	mux.HandleFunc("GET /api/library/schedules", app.handleScheduleList)
	mux.HandleFunc("POST /api/library/schedules", app.handleScheduleSave)
	mux.HandleFunc("GET /api/library/sites", app.handleSiteList)
	mux.HandleFunc("POST /api/library/sites", app.handleSiteSave)
	mux.HandleFunc("POST /api/library/sites/verify", app.handleSiteVerify)
	mux.HandleFunc("POST /api/library/sites/login", app.handleSiteLogin)
	mux.HandleFunc("GET /api/library/sites/login/status", app.handleSiteLoginStatus)
	mux.HandleFunc("POST /api/library/sites/login/finish", app.handleSiteLoginFinish)
	mux.HandleFunc("POST /api/library/sites/assist/start", app.handleSiteAssistStart)
	mux.HandleFunc("GET /api/library/sites/assist/status", app.handleSiteAssistStatus)
	mux.HandleFunc("POST /api/library/sites/assist/open", app.handleSiteAssistOpen)
	mux.HandleFunc("POST /api/library/sites/assist/search-entry", app.handleSiteAssistEntry)
	mux.HandleFunc("POST /api/library/sites/assist/search-url", app.handleSiteAssistSearchURL)
	mux.HandleFunc("POST /api/library/sites/assist/login-confirm", app.handleSiteAssistLoginConfirm)
	mux.HandleFunc("POST /api/library/sites/assist/login", app.handleSiteAssistLogin)
	mux.HandleFunc("POST /api/library/sites/assist/retry", app.handleSiteAssistRetry)
	mux.HandleFunc("POST /api/library/sites/assist/cancel", app.handleSiteAssistCancel)
	mux.HandleFunc("DELETE /api/library/sites/{id}", app.handleSiteDelete)
	mux.HandleFunc("DELETE /api/library/schedules/{id}", app.handleScheduleDelete)
	mux.HandleFunc("POST /api/library/schedules/{id}/enabled", app.handleScheduleEnabled)
	mux.HandleFunc("POST /api/kb/index", app.handleKBIndex)
	mux.HandleFunc("POST /api/kb/remove", app.handleKBRemove)
	mux.HandleFunc("GET /api/kb/status", app.handleKBStatus)
	mux.HandleFunc("GET /api/kb/search", app.handleKBSearch)
	mux.HandleFunc("GET /api/kb/revisions", app.handleKBRevisions)
	mux.HandleFunc("POST /api/kb/rollback", app.handleKBRollback)
	mux.HandleFunc("GET /api/asr/status", app.handleASRStatus)
	mux.HandleFunc("POST /api/asr/transcribe", app.handleASRTranscribe)
	mux.HandleFunc("GET /api/duplex/status", app.handleDuplexStatus)
	mux.HandleFunc("GET /api/duplex", app.handleDuplex)
	mux.HandleFunc("GET /api/voice/local/status", app.handleVoiceLocalStatus)
	mux.HandleFunc("GET /api/voice/local", app.handleVoiceLocal)
	mux.HandleFunc("POST /api/asr/local", app.handleVoiceLocalTranscribe)
	mux.HandleFunc("GET /api/system/status", app.handleComputerStatus)
	mux.HandleFunc("GET /api/model/config", app.handleModelConfigGet)
	mux.HandleFunc("POST /api/model/config", app.handleModelConfigSave)
	mux.HandleFunc("POST /api/model/test", app.handleModelTest)
	mux.HandleFunc("POST /api/chat", app.handleChat)
	mux.HandleFunc("POST /api/chat/stream", app.handleChatStream)
	mux.HandleFunc("POST /api/chat/summarize", app.handleSummarize)
	mux.HandleFunc("GET /api/chat/usage", app.handleUsageList)
	mux.HandleFunc("GET /api/system/autostart", app.handleAutostartGet)
	mux.HandleFunc("POST /api/system/autostart", app.handleAutostartSet)
	mux.HandleFunc("GET /api/server/config", app.handleServerConfigGet)
	mux.HandleFunc("POST /api/server/config", app.handleServerConfigSet)
	mux.HandleFunc("POST /api/server/test", app.handleServerTest)
	mux.HandleFunc("POST /api/update/check", app.handleUpdateCheck)
	mux.HandleFunc("GET /api/update/available", app.handleUpdateAvailable)
	mux.HandleFunc("POST /api/update/download", app.handleUpdateDownload)
	mux.HandleFunc("POST /api/update/install", app.handleUpdateInstall)
	mux.HandleFunc("POST /api/account/login", app.handleAccountLogin)
	mux.HandleFunc("GET /api/account/me", app.handleAccountMe)
	mux.HandleFunc("POST /api/account/logout", app.handleAccountLogout)
	mux.HandleFunc("POST /api/account/wechat/start", app.handleWechatStartProxy)
	mux.HandleFunc("GET /api/account/wechat/poll", app.handleWechatPollProxy)

	// 显式设置超时：ListenAndServe 默认无 ReadHeaderTimeout，慢连接会一直占着 goroutine。
	server := &http.Server{
		Addr:              listenCfg.addr,
		Handler:           cors(listenCfg.middleware(mux)),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	// 后台预热硬件检测缓存：WMI 冷启动较慢，提前跑完可让前端秒开。
	warmHardwareCache()
	// 定时搜索：应用运行期间由 sidecar 按时抓取；关闭应用即暂停。
	go app.runScheduler(context.Background())
	log.Printf("开心同学本地服务已启动: http://%s （软件运行目录 %s，LAN=%t）", listenCfg.addr, app.runtimeRoot, listenCfg.lan)
	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("服务启动失败: %v", err)
	}
}
