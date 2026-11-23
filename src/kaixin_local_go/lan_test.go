package main

import (
	"crypto/sha256"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLoadListenConfigDefaultsToLoopback(t *testing.T) {
	t.Setenv("KAIXIN_PORT", "")
	t.Setenv("KAIXIN_LAN", "")
	t.Setenv("KAIXIN_LAN_TOKEN", "")
	cfg, err := loadListenConfig()
	if err != nil {
		t.Fatalf("默认配置不应失败: %v", err)
	}
	if cfg.addr != "127.0.0.1:5085" || cfg.lan {
		t.Fatalf("默认监听配置错误: %+v", cfg)
	}
}

func TestLoadListenConfigLANRequiresToken(t *testing.T) {
	t.Setenv("KAIXIN_LAN", "1")
	t.Setenv("KAIXIN_LAN_TOKEN", "")
	if _, err := loadListenConfig(); err == nil {
		t.Fatal("LAN 模式缺少令牌时必须拒绝启动")
	}

	t.Setenv("KAIXIN_LAN_TOKEN", "test-token")
	t.Setenv("KAIXIN_PORT", "5090")
	cfg, err := loadListenConfig()
	if err != nil {
		t.Fatalf("有效 LAN 配置不应失败: %v", err)
	}
	if cfg.addr != "0.0.0.0:5090" || !cfg.lan {
		t.Fatalf("LAN 监听配置错误: %+v", cfg)
	}
}

func TestLANMiddlewareAuthentication(t *testing.T) {
	cfg := listenConfig{lan: true, tokenHash: sha256.Sum256([]byte("test-token"))}
	handler := cfg.middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	cases := []struct {
		name   string
		method string
		path   string
		token  string
		want   int
	}{
		{name: "缺少令牌", path: "/api/data/info", want: http.StatusUnauthorized},
		{name: "错误令牌", path: "/api/data/info", token: "wrong", want: http.StatusUnauthorized},
		{name: "请求头令牌", path: "/api/data/info", token: "test-token", want: http.StatusNoContent},
		{name: "WebSocket 查询令牌", path: "/api/duplex?token=test-token", want: http.StatusNoContent},
		{name: "普通接口不允许查询令牌", path: "/api/data/info?token=test-token", want: http.StatusUnauthorized},
		{name: "健康检查放行", path: "/api/health", want: http.StatusNoContent},
		{name: "预检放行", method: http.MethodOptions, path: "/api/data/info", want: http.StatusNoContent},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			method := item.method
			if method == "" {
				method = http.MethodGet
			}
			req := httptest.NewRequest(method, item.path, nil)
			if item.token != "" {
				req.Header.Set("X-Kaixin-Token", item.token)
			}
			resp := httptest.NewRecorder()
			handler.ServeHTTP(resp, req)
			if resp.Code != item.want {
				t.Fatalf("状态码 = %d，期望 %d", resp.Code, item.want)
			}
		})
	}
}

func TestLANMiddlewareAllowsLoopback(t *testing.T) {
	cfg := listenConfig{lan: true, tokenHash: sha256.Sum256([]byte("test-token"))}
	handler := cfg.middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/data/info", nil)
	req.RemoteAddr = "127.0.0.1:53210"
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusNoContent {
		t.Fatalf("本机回环请求应放行，实际 %d", resp.Code)
	}
}

func TestLANMiddlewareKeepsOriginAllowlist(t *testing.T) {
	cfg := listenConfig{lan: true, tokenHash: sha256.Sum256([]byte("test-token"))}
	handler := cors(cfg.middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})))

	req := httptest.NewRequest(http.MethodGet, "/api/duplex/status", nil)
	req.Header.Set("Origin", "https://localhost")
	req.Header.Set("X-Kaixin-Token", "test-token")
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusNoContent || resp.Header().Get("Access-Control-Allow-Origin") != "https://localhost" {
		t.Fatalf("Capacitor 来源应放行: code=%d headers=%v", resp.Code, resp.Header())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/duplex/status", nil)
	req.Header.Set("Origin", "https://evil.example")
	req.Header.Set("X-Kaixin-Token", "test-token")
	resp = httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusForbidden {
		t.Fatalf("未知来源应拒绝，实际 %d", resp.Code)
	}
}
