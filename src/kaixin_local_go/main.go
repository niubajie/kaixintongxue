package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
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
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		base = "."
	}
	root := filepath.Join(base, "Kaixin")
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

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "kaixin-local"})
	})
	mux.HandleFunc("GET /api/system/hardware", app.handleHardware)
	mux.HandleFunc("GET /api/skills/catalog", app.handleCatalog)
	mux.HandleFunc("GET /api/sites/catalog", app.handleSites)
	mux.HandleFunc("POST /api/skills/match-software", app.handleMatchSoftware)
	mux.HandleFunc("POST /api/auth/confirm", app.handleAuthConfirm)
	mux.HandleFunc("POST /api/auth/open", app.handleAuthOpen)
	mux.HandleFunc("POST /api/deploy/plan", app.handlePlan)
	mux.HandleFunc("POST /api/deploy/execute", app.handleDeployExecute)
	mux.HandleFunc("GET /api/deploy/status", app.handleDeployStatus)

	// 端口优先从环境变量 KAIXIN_PORT 读取（桌面端 Tauri sidecar 会注入一个空闲端口），
	// 未设置时回退到 5085，方便网页端与手动调试。
	port := os.Getenv("KAIXIN_PORT")
	if port == "" {
		port = "5085"
	}
	addr := "127.0.0.1:" + port
	log.Printf("开心同学本地部署服务已启动: http://%s （软件运行目录 %s）", addr, app.runtimeRoot)
	if err := http.ListenAndServe(addr, cors(mux)); err != nil {
		log.Fatalf("服务启动失败: %v", err)
	}
}
