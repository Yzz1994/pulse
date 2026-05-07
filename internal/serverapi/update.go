package serverapi

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"pulse/internal/buildinfo"
)

var (
	releaseCache    *githubRelease
	releaseCacheAt  time.Time
	releaseCacheMu  sync.Mutex
	releaseCacheTTL = 30 * time.Minute
)

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

type githubRelease struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
	Body    string `json:"body"`
}

const pulseRepo = "0xUnixIO/pulse"

func handleUpdateCheck(w http.ResponseWriter, r *http.Request) {
	release, err := fetchLatestReleaseCached()
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
	// 先响应客户端，再执行更新（server 会重启）
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "更新已开始，server 将在数秒后重启"})
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	go func() {
		time.Sleep(500 * time.Millisecond)
		script := fmt.Sprintf(
			`curl -fsSL https://raw.githubusercontent.com/%s/main/scripts/install.sh | sh -s -- server`,
			pulseRepo,
		)
		cmd := exec.Command("sh", "-c", script)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			log.Printf("update: install script failed: %v", err)
		}
	}()
}

// WarmUpdateCache 在服务启动时预热 GitHub release 缓存，避免首次页面加载时检查更新超时。
func WarmUpdateCache() {
	if _, err := fetchLatestReleaseCached(); err != nil {
		log.Printf("update: warm cache failed: %v", err)
	}
}

func fetchLatestReleaseCached() (*githubRelease, error) {
	releaseCacheMu.Lock()
	defer releaseCacheMu.Unlock()
	if releaseCache != nil && time.Since(releaseCacheAt) < releaseCacheTTL {
		return releaseCache, nil
	}
	r, err := fetchLatestRelease()
	if err != nil {
		return nil, err
	}
	releaseCache = r
	releaseCacheAt = time.Now()
	return r, nil
}

func fetchLatestRelease() (*githubRelease, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", pulseRepo)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求 GitHub API 失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API 返回 %d", resp.StatusCode)
	}
	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	return &release, nil
}
