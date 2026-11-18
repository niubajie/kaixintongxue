package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type App struct {
	cfg Config
	db  *pgxpool.Pool
}

func main() {
	cfg := loadConfig()
	if err := os.MkdirAll(cfg.DataRoot, 0o755); err != nil {
		log.Fatalf("创建数据目录失败: %v", err)
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("连接数据库失败: %v", err)
	}
	defer pool.Close()

	if err := initSchema(ctx, pool); err != nil {
		log.Fatalf("%v", err)
	}
	app := &App{cfg: cfg, db: pool}
	if err := app.seed(ctx); err != nil {
		log.Printf("初始化示例数据失败: %v", err)
	}
	// 首个账号自动成为 Owner，否则没人能管理成员与目录
	if err := app.ensureOwner(ctx); err != nil {
		log.Printf("检查 Owner 失败：%v", err)
	}

	mux := http.NewServeMux()
	app.routes(mux)

	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           logging(mux),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("开心同学服务端（Go）已启动: http://127.0.0.1:%s/admin/", cfg.Port)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("服务启动失败: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	_ = server.Shutdown(shutdownCtx)
	log.Println("服务已停止。")
}

func (a *App) routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "runtime": "go"})
	})

	mux.HandleFunc("POST /api/auth/register", a.handleRegister)
	mux.HandleFunc("POST /api/auth/login", a.handleLogin)
	mux.HandleFunc("POST /api/auth/logout", a.handleLogout)
	mux.HandleFunc("GET /api/auth/me", a.handleMe)

	mux.HandleFunc("GET /api/software", a.handleSoftwareList)
	mux.HandleFunc("GET /api/skills/packages", a.handleSkillPackages)
	mux.HandleFunc("GET /api/version-notes", a.handleNotesList)
	mux.HandleFunc("GET /api/client/latest", a.handleClientLatest)
	mux.HandleFunc("GET /api/client/download", a.handleClientDownload)

	mux.HandleFunc("GET /api/admin/users", a.handleAdminUsers)
	mux.HandleFunc("POST /api/admin/users/{id}/role", a.handleUserRole)
	mux.HandleFunc("POST /api/admin/users/{id}/status", a.handleUserStatus)
	mux.HandleFunc("DELETE /api/admin/users/{id}", a.handleUserDelete)
	mux.HandleFunc("GET /api/admin/audit", a.handleAudit)
	mux.HandleFunc("POST /api/admin/backups", a.handleBackupCreate)
	mux.HandleFunc("GET /api/admin/backups", a.handleBackupList)
	mux.HandleFunc("POST /api/admin/backups/{id}/restore", a.handleBackupRestore)
	mux.HandleFunc("GET /api/admin/backups/{id}/download", a.handleBackupDownload)

	mux.HandleFunc("POST /api/admin/software", a.handleSoftwareCreate)
	mux.HandleFunc("PUT /api/admin/software/{id}", a.handleSoftwareUpdate)
	mux.HandleFunc("DELETE /api/admin/software/{id}", a.handleSoftwareDelete)
	mux.HandleFunc("GET /api/admin/software/{id}/next-version", a.handleNextVersion)
	mux.HandleFunc("POST /api/admin/software/{id}/publish", a.handlePublish)
	mux.HandleFunc("POST /api/admin/software/{id}/versions/{version}/confirm", a.handleConfirmVersion)
	mux.HandleFunc("POST /api/admin/software/{id}/rollback", a.handleRollback)
	mux.HandleFunc("DELETE /api/admin/software/{id}/versions/{version}", a.handleVersionDelete)
	mux.HandleFunc("POST /api/admin/software/{id}/scan", a.handleRescan)
	mux.HandleFunc("GET /api/admin/software/{id}/history", a.handleHistory)
	mux.HandleFunc("POST /api/admin/version-notes", a.handleNoteCreate)

	adminIndex := filepath.Join(a.cfg.AdminDist, "index.html")
	if _, err := os.Stat(adminIndex); err != nil {
		log.Printf("未找到后台构建产物 %s，请先在 src/kaixin_admin 执行 npm run build", adminIndex)
	}

	mux.HandleFunc("GET /admin.html", func(w http.ResponseWriter, r *http.Request) {
		if _, err := os.Stat(adminIndex); err != nil {
			writeMessage(w, http.StatusServiceUnavailable, "后台页面尚未构建，请先执行 npm run build。")
			return
		}
		http.ServeFile(w, r, adminIndex)
	})
	mux.Handle("GET /admin/", http.StripPrefix("/admin/", http.FileServer(http.Dir(a.cfg.AdminDist))))
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/admin/", http.StatusFound)
	})
}

func logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(recorder, r)
		log.Printf("%s %s %d %s", r.Method, r.URL.Path, recorder.status, time.Since(start).Round(time.Millisecond))
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}
