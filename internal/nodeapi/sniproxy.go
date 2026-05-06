package nodeapi

import (
	"encoding/json"
	"log"
	"net/http"

	"pulse/internal/sniproxy"
)

// handleSNIProxySync 接收 pulse-server 推送的 SNI 代理完整配置，
// 通过 sniproxy.Manager 热更新节点代理。
func (a *API) handleSNIProxySync(w http.ResponseWriter, r *http.Request) {
	if a.sniManager == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error": "sni proxy manager not configured on this node",
		})
		return
	}
	var req sniproxy.ManagerConfig
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json body"})
		return
	}
	log.Printf("sniproxy apply: listen=%q routes=%d cert_domains=%v cf_token_set=%v",
		req.Listen, len(req.Routes), req.CertDomains, req.CloudflareToken != "")
	if err := a.sniManager.Apply(req); err != nil {
		log.Printf("sniproxy apply error: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"listen":  req.Listen,
		"routes":  len(req.Routes),
		"managed": a.sniManager.Config().Listen != "",
	})
}

// handleSNIProxyStatus 返回当前生效的配置摘要和实时运行状态。
// Status 字段里 Listen 为空 + LastError 非空表示 Serve 失败（端口冲突等）；
// 运维凭此判断新代理是否真的接管了流量。
func (a *API) handleSNIProxyStatus(w http.ResponseWriter, r *http.Request) {
	if a.sniManager == nil {
		writeJSON(w, http.StatusOK, map[string]any{"enabled": false})
		return
	}
	status := a.sniManager.Status()
	cfg := a.sniManager.Config()
	// 脱敏：API Token 不回显
	cfg.CloudflareToken = ""
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled": status.Listen != "",
		"status":  status,
		"config":  cfg,
	})
}
