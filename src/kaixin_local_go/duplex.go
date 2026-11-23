package main

import (
	"net/http"
	"os"
	"strings"
)

const (
	duplexModel       = "MiniCPM-o-4.5-Realtime"
	duplexRealtimeURL = "wss://api.modelbest.cn/v1/realtime?mode=audio&model=MiniCPM-o-4.5-Realtime"
)

type DuplexStatus struct {
	Configured bool   `json:"configured"`
	Model      string `json:"model"`
}

func duplexAPIKey() string {
	return strings.TrimSpace(os.Getenv("MAP_API_KEY"))
}

func (a *App) handleDuplexStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, DuplexStatus{
		Configured: duplexAPIKey() != "",
		Model:      duplexModel,
	})
}

func (a *App) handleDuplex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("mode") != "audio" {
		fail(w, http.StatusBadRequest, "实时语音仅支持 mode=audio")
		return
	}
	apiKey := duplexAPIKey()
	if apiKey == "" {
		fail(w, http.StatusServiceUnavailable, "未配置 MAP_API_KEY，无法启动实时语音")
		return
	}
	a.serveDuplex(w, r, duplexRealtimeURL, apiKey)
}
