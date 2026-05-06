package serverapi

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"pulse/internal/geoip"
	"pulse/internal/nodes"
)

// GeoIPSettingsStore 提供 MaxMind key 的持久化读写。
type GeoIPSettingsStore interface {
	GetSetting(key string) (string, bool)
	SetSetting(key, value string) error
}

// RegisterGeoIPAPI 注册 MaxMind key 管理和节点 GeoIP 分析路由。
func RegisterGeoIPAPI(mux *http.ServeMux, settings GeoIPSettingsStore, nodeStore nodes.Store, db *geoip.DB) {
	// GET/PUT /v1/settings/maxmind — license key 管理
	mux.HandleFunc("GET /v1/settings/maxmind", func(w http.ResponseWriter, r *http.Request) {
		key, _ := settings.GetSetting("maxmind_license_key")
		writeJSON(w, http.StatusOK, map[string]any{
			"has_key":  key != "",
			"key":      key,
			"db_ready": db.Ready(),
		})
	})
	mux.HandleFunc("PUT /v1/settings/maxmind", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Key string `json:"key"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
			return
		}
		if err := settings.SetSetting("maxmind_license_key", body.Key); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	})

	// POST /v1/system/geoip/download — 下载 / 更新 mmdb 文件
	mux.HandleFunc("POST /v1/system/geoip/download", func(w http.ResponseWriter, r *http.Request) {
		key, _ := settings.GetSetting("maxmind_license_key")
		if key == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "未配置 MaxMind License Key"})
			return
		}
		// 异步下载，立即响应
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "开始下载数据库，请稍候…"})
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		go func() {
			if err := db.Download(key); err == nil {
				_ = settings.SetSetting("maxmind_last_download", time.Now().UTC().Format(time.RFC3339))
			}
		}()
	})

	// GET /v1/system/geoip/lookup?host= — 查询单个地址的 GeoIP 信息
	mux.HandleFunc("GET /v1/system/geoip/lookup", func(w http.ResponseWriter, r *http.Request) {
		if !db.Ready() {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "GeoIP 数据库未就绪，请先下载"})
			return
		}
		host := strings.TrimSpace(r.URL.Query().Get("host"))
		if host == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "host 参数不能为空"})
			return
		}
		info, err := db.LookupHost(host)
		if err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, info)
	})

	// GET /v1/nodes/geoip — 分析所有节点 IP
	mux.HandleFunc("GET /v1/nodes/geoip", func(w http.ResponseWriter, r *http.Request) {
		if !db.Ready() {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "GeoIP 数据库未就绪，请先下载"})
			return
		}
		nodeList, err := nodeStore.List()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}

		type result struct {
			NodeID string        `json:"node_id"`
			Info   *geoip.Info   `json:"info,omitempty"`
			Error  string        `json:"error,omitempty"`
		}

		results := make([]result, len(nodeList))
		var wg sync.WaitGroup
		for i, node := range nodeList {
			wg.Add(1)
			go func(idx int, n nodes.Node) {
				defer wg.Done()
				// 优先使用 ip_override，回退到 base_url 的 host
				host := strings.TrimSpace(n.IPOverride)
				if host == "" {
					host = extractHost(n.BaseURL)
				}
				if host == "" {
					results[idx] = result{NodeID: n.ID, Error: "无法解析地址"}
					return
				}
				info, err := db.LookupHost(host)
				if err != nil {
					results[idx] = result{NodeID: n.ID, Error: err.Error()}
					return
				}
				results[idx] = result{NodeID: n.ID, Info: &info}
			}(i, node)
		}
		wg.Wait()

		writeJSON(w, http.StatusOK, map[string]any{"results": results})
	})
}

// extractHost 从 BaseURL 中提取 hostname（去掉 scheme 和 port）。
func extractHost(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return u.Hostname()
}
