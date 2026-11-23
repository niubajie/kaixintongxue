package main

import (
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
)

type listenConfig struct {
	addr      string
	lan       bool
	tokenHash [32]byte
}

func loadListenConfig() (listenConfig, error) {
	port := strings.TrimSpace(os.Getenv("KAIXIN_PORT"))
	if port == "" {
		port = "5085"
	}
	if os.Getenv("KAIXIN_LAN") != "1" {
		return listenConfig{addr: "127.0.0.1:" + port}, nil
	}
	token := strings.TrimSpace(os.Getenv("KAIXIN_LAN_TOKEN"))
	if token == "" {
		return listenConfig{}, fmt.Errorf("KAIXIN_LAN=1 时必须设置非空 KAIXIN_LAN_TOKEN")
	}
	return listenConfig{
		addr:      "0.0.0.0:" + port,
		lan:       true,
		tokenHash: sha256.Sum256([]byte(token)),
	}, nil
}

// isLoopbackAddr 判断请求是否来自本机：局域网模式下本机调用（桌面端 sidecar 自身）不受令牌约束。
func isLoopbackAddr(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (cfg listenConfig) middleware(next http.Handler) http.Handler {
	if !cfg.lan {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions || r.URL.Path == "/api/health" || isLoopbackAddr(r.RemoteAddr) {
			next.ServeHTTP(w, r)
			return
		}
		token := r.Header.Get("X-Kaixin-Token")
		if token == "" && r.URL.Path == "/api/duplex" {
			token = r.URL.Query().Get("token")
		}
		candidate := sha256.Sum256([]byte(token))
		if subtle.ConstantTimeCompare(candidate[:], cfg.tokenHash[:]) != 1 {
			fail(w, http.StatusUnauthorized, "局域网访问令牌缺失或错误")
			return
		}
		next.ServeHTTP(w, r)
	})
}
