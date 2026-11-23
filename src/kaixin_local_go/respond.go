package main

import (
	"encoding/json"
	"net/http"
	"net/url"
	"unicode/utf8"
)

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func fail(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"message": message})
}

func decodeJSON(r *http.Request, target any) error {
	return json.NewDecoder(http.MaxBytesReader(nil, r.Body, 4<<20)).Decode(target)
}

func truncate(text string, max int) string {
	if max <= 0 {
		return ""
	}
	if len(text) <= max {
		return text
	}
	end := max
	for end > 0 && !utf8.ValidString(text[:end]) {
		end--
	}
	return text[:end]
}

// originAllowed 只放行本机页面与 Tauri / Capacitor 壳的来源。
//
// 本地服务具备"下载并安装软件到本机"的能力，若允许任意 Origin，
// 用户访问的任何一个网页都能在后台调用 /api/deploy/execute，
// 等于把安装器暴露给整个互联网。这里必须白名单。
func originAllowed(origin string) bool {
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Scheme == "" {
		return false
	}
	switch parsed.Scheme {
	case "tauri", "capacitor", "ionic":
		return true
	case "http", "https":
		switch parsed.Hostname() {
		case "localhost", "127.0.0.1", "::1", "tauri.localhost":
			return true
		}
	}
	return false
}

// cors：白名单校验 + 预检处理。
// 没有 Origin 头的请求（curl、Tauri sidecar、本机脚本）放行——
// 这些进程本来就具备与用户同等的本机权限，不构成本地服务的额外威胁。
func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			if !originAllowed(origin) {
				fail(w, http.StatusForbidden, "来源不被允许：本地服务只接受开心同学应用自身的请求")
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Kaixin-Token")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
