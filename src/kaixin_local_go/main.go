package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// 开心同学 · 本地部署服务：纯本地、无数据库，只监听 127.0.0.1。
// 前三步（硬件/匹配/方案）零副作用，第四步（部署）才写入本机目录。
type App struct {
	catalog     []SoftwareEntry
	sites       []SiteEntry
	runtimeRoot string
	downloadDir string
	logDir      string
	authDir     string
	deploy      deployer
	store       *Store
}

func main() {
	catalog, err := loadCatalog()
	if err != nil {
		log.Fatalf("%v", err)
	}
	sites, err := loadSites()
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
		catalog:     catalog,
		sites:       sites,
		runtimeRoot: filepath.Join(root, "runtime"),
		downloadDir: filepath.Join(root, "downloads"),
		logDir:      filepath.Join(root, "logs"),
		authDir:     filepath.Join(root, "auth"),
	}
	for _, dir := range []string{app.runtimeRoot, app.downloadDir, app.logDir, app.authDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			log.Fatalf("创建目录失败 %s: %v", dir, err)
		}
	}

	// 本地库：所有持久化数据的唯一落点（docs/08 第 11 节）
	// KAIXIN_DATA_DIR 可用于排障或测试时指定其他目录。
	dataDir := filepath.Join(root, "data")
	if custom := os.Getenv("KAIXIN_DATA_DIR"); custom != "" {
		dataDir = custom
	}
	store, err := OpenStore(dataDir)
	if err != nil {
		// 建库失败不阻断整个服务：硬件检测、匹配、方案生成都还能用，
		// 只有数据接口返回 503。这样用户至少不会被"整个程序不能用"卡住。
		log.Printf("[WARN] 本地库初始化失败（数据接口将不可用）: %v", err)
	} else {
		defer store.Close()
		app.store = store
		log.Printf("本地库就绪: %s", filepath.Join(dataDir, "app.db"))
	}

	// 启动时回收上次异常退出留下的「进行中」部署记录（租约已过期）
	if app.store != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if n, err := app.store.SweepStaleDeployRuns(ctx); err == nil && n > 0 {
			log.Printf("回收 %d 条中断的部署记录", n)
		}
		cancel()
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "kaixin-local"})
	})
	mux.HandleFunc("GET /api/system/hardware", app.handleHardware)
	mux.HandleFunc("GET /api/skills/catalog", app.handleCatalog)
	mux.HandleFunc("GET /api/sites/catalog", app.handleSites)
	mux.HandleFunc("POST /api/sites/resolve", app.handleSitesResolve)
	mux.HandleFunc("POST /api/sites/open", app.handleSiteOpenPage)
	mux.HandleFunc("POST /api/skills/match-software", app.handleMatchSoftware)
	mux.HandleFunc("POST /api/auth/confirm", app.handleAuthConfirm)
	mux.HandleFunc("POST /api/auth/open", app.handleAuthOpen)
	mux.HandleFunc("POST /api/deploy/plan", app.handlePlan)
	mux.HandleFunc("POST /api/deploy/execute", app.handleDeployExecute)
	mux.HandleFunc("GET /api/deploy/status", app.handleDeployStatus)
	mux.HandleFunc("POST /api/deploy/cancel", app.handleDeployCancel)
	mux.HandleFunc("GET /api/deploy/history", app.handleDeployHistory)
	mux.HandleFunc("POST /api/deploy/resume", app.handleDeployResume)
	mux.HandleFunc("POST /api/deploy/retry", app.handleDeployRetryStep)
	mux.HandleFunc("GET /api/data/info", app.handleDataInfo)
	mux.HandleFunc("GET /api/library/scan", app.handleLibraryScan)
	mux.HandleFunc("POST /api/library/open", app.handleLibraryOpen)
	mux.HandleFunc("POST /api/library/open-folder", app.handleLibraryOpenFolder)
	mux.HandleFunc("POST /api/library/set-dir", app.handleLibrarySetDir)
	mux.HandleFunc("POST /api/kb/index", app.handleKBIndex)
	mux.HandleFunc("POST /api/kb/remove", app.handleKBRemove)
	mux.HandleFunc("GET /api/kb/status", app.handleKBStatus)
	mux.HandleFunc("GET /api/kb/search", app.handleKBSearch)
	mux.HandleFunc("GET /api/kb/revisions", app.handleKBRevisions)
	mux.HandleFunc("POST /api/kb/rollback", app.handleKBRollback)
	mux.HandleFunc("GET /api/asr/status", app.handleASRStatus)
	mux.HandleFunc("POST /api/asr/transcribe", app.handleASRTranscribe)
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
	mux.HandleFunc("GET /api/skills/custom", app.handleListCustomSkills)
	mux.HandleFunc("POST /api/skills/custom", app.handleSaveCustomSkill)
	mux.HandleFunc("DELETE /api/skills/custom/{id}", app.handleDeleteCustomSkill)

	// 端口优先从环境变量 KAIXIN_PORT 读取（桌面端 Tauri sidecar 会注入一个空闲端口），
	// 未设置时回退到 5085，方便网页端与手动调试。
	port := os.Getenv("KAIXIN_PORT")
	if port == "" {
		port = "5085"
	}
	addr := "127.0.0.1:" + port
	// 显式设置超时：ListenAndServe 默认无 ReadHeaderTimeout，慢连接会一直占着 goroutine。
	server := &http.Server{
		Addr:              addr,
		Handler:           cors(mux),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	// 后台预热硬件检测缓存：WMI 冷启动较慢，提前跑完可让前端秒开。
	warmHardwareCache()
	log.Printf("开心同学本地部署服务已启动: http://%s （软件运行目录 %s）", addr, app.runtimeRoot)
	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("服务启动失败: %v", err)
	}
}
