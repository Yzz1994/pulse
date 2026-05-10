package serverapi

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"pulse/internal/buildinfo"
	"pulse/internal/selfupdate"
)

var serverUpdating atomic.Bool

// UpdateSettingsStore 提供设置的持久化读写。
type UpdateSettingsStore interface {
	GetSetting(key string) (string, bool)
	SetSetting(key, value string) error
}

// RegisterUpdateAPI 注册检查更新、触发更新路由。
func RegisterUpdateAPI(mux *http.ServeMux, settings UpdateSettingsStore) {
	// Cloudflare API Token（用于 DNS-01 证书申请）
	mux.HandleFunc("GET /v1/settings/cf-token", func(w http.ResponseWriter, r *http.Request) {
		token, _ := settings.GetSetting("cf_token")
		writeJSON(w, http.StatusOK, map[string]any{"has_token": token != "", "token": token})
	})
	mux.HandleFunc("PUT /v1/settings/cf-token", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Token string `json:"token"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json body"})
			return
		}
		if err := settings.SetSetting("cf_token", strings.TrimSpace(body.Token)); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	})

	mux.HandleFunc("GET /v1/system/update/check", handleUpdateCheck)
	mux.HandleFunc("POST /v1/system/update/apply", handleUpdateApply)
}

func handleUpdateCheck(w http.ResponseWriter, r *http.Request) {
	release, err := selfupdate.FetchLatestReleaseCached()
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
		return
	}
	latest := strings.TrimPrefix(release.TagName, "v")
	current := strings.TrimPrefix(buildinfo.Version, "v")
	writeJSON(w, http.StatusOK, map[string]any{
		"current":    buildinfo.Version,
		"latest":     release.TagName,
		"has_update": latest != current && buildinfo.Version != "dev",
		"url":        release.HTMLURL,
		"notes":      release.Body,
	})
}

func handleUpdateApply(w http.ResponseWriter, r *http.Request) {
	if !serverUpdating.CompareAndSwap(false, true) {
		writeJSON(w, http.StatusConflict, map[string]any{"error": "更新已在进行中"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "更新已开始，server 将在数秒后重启"})
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	go func() {
		defer serverUpdating.Store(false)
		time.Sleep(500 * time.Millisecond)
		if err := selfupdate.Apply(context.Background(), "server"); err != nil {
			log.Printf("update: server 更新失败: %v", err)
		}
	}()
}

// WarmUpdateCache 在服务启动时预热 GitHub release 缓存，避免首次页面加载时检查更新超时。
func WarmUpdateCache() {
	if _, err := selfupdate.FetchLatestReleaseCached(); err != nil {
		log.Printf("update: warm cache failed: %v", err)
	}
}
