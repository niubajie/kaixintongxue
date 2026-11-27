// Command api is the gocircle HTTP server entry point.
//
// Wiring order: config → db pools → auth/audit → modules → router.
// Each module registers its own routes; main only assembles dependencies.
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"gocircle/server/internal/account"
	"gocircle/server/internal/admin"
	"gocircle/server/internal/audit"
	"gocircle/server/internal/auditadmin"
	"gocircle/server/internal/auth"
	"gocircle/server/internal/category"
	"gocircle/server/internal/config"
	"gocircle/server/internal/db"
	"gocircle/server/internal/httpapi"
	"gocircle/server/internal/resource"
	"gocircle/server/internal/risk"
	"gocircle/server/internal/settings"
	"gocircle/server/internal/useradmin"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config.Load: %v", err)
	}

	pools, err := db.New(cfg)
	if err != nil {
		log.Fatalf("db.New: %v", err)
	}
	defer pools.Close()

	authSvc := auth.New(cfg)
	auditSvc := audit.New(pools.PG)

	// ---- account ----
	accRepo := account.NewRepository(pools.PG)
	accSvc := account.NewService(cfg, pools.PG, accRepo, authSvc)
	accHandler := account.NewHandler(accSvc)

	// ---- admin ----
	adminRepo := admin.NewRepository(pools.PG)
	adminSvc := admin.NewService(adminRepo, authSvc, auditSvc)
	adminHandler := admin.NewHandler(adminSvc)
	adminMW := admin.NewMiddleware(authSvc)

	// ---- category ----
	catRepo := category.NewRepository(pools.PG)
	catSvc := category.NewService(catRepo, auditSvc)
	catHandler := category.NewHandler(catSvc)

	// ---- resource ----
	resRepo := resource.NewRepository(pools.PG)
	resSvc := resource.NewService(cfg, pools.PG, resRepo, auditSvc)
	resHandler := resource.NewHandler(resSvc)

	// ---- 后台：用户 / 风控 / 配置 / 审计 ----
	userAdminSvc := useradmin.NewService(useradmin.NewRepository(pools.PG), auditSvc)
	userAdminHandler := useradmin.NewHandler(userAdminSvc)

	riskSvc := risk.New(pools.PG)
	riskHandler := risk.NewHandler(riskSvc)

	settingsSvc := settings.NewService(settings.NewRepository(pools.PG), auditSvc)
	settingsHandler := settings.NewHandler(settingsSvc)

	auditHandler := auditadmin.NewHandler(auditSvc)

	r := httpapi.NewRouter()
	accHandler.Register(r, authSvc)
	adminHandler.Register(r)
	catHandler.Register(r, authSvc, adminMW)
	resHandler.Register(r, authSvc, adminMW)
	userAdminHandler.Register(r, adminMW)
	riskHandler.Register(r, adminMW)
	settingsHandler.Register(r, adminMW)
	auditHandler.Register(r, adminMW)

	// Health — no auth, no DB: probe only.
	r.GET("/api/v1/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "time": time.Now().UTC().Format(time.RFC3339)})
	})

	// 首次启动引导管理员（无管理员时用环境变量创建 super_admin）。
	bootCtx, bootCancel := context.WithTimeout(context.Background(), 10*time.Second)
	if created, err := adminSvc.EnsureBootstrap(bootCtx, cfg.AdminBootstrapUsername, cfg.AdminBootstrapPassword); err != nil {
		log.Printf("[admin] bootstrap failed: %v", err)
	} else if created {
		log.Printf("[admin] bootstrap super_admin created: %s", cfg.AdminBootstrapUsername)
	}
	bootCancel()

	// 到期资源自动下架：启动即跑一次，之后每 10 分钟一次。
	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		for {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			if n, err := resSvc.ExpireDue(ctx); err != nil {
				log.Printf("[resource] expire due failed: %v", err)
			} else if n > 0 {
				log.Printf("[resource] expired %d resources", n)
			}
			cancel()
			<-ticker.C
		}
	}()

	srv := &http.Server{
		Addr:         cfg.Addr,
		Handler:      r,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	}

	go func() {
		log.Printf("server listening on %s", cfg.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("ListenAndServe: %v", err)
		}
	}()

	// Graceful shutdown.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("shutdown: %v", err)
	}
	log.Println("server stopped")
}
