package serverapi

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"

	"pulse/internal/nodes"
)

// RegisterNodeRegisterAPI 注册节点自注册接口（公开，无鉴权）和节点安装证书接口（受保护）。
// publicMux: 直接挂载，无需认证；protectedMux: 挂载后由外层 auth.Middleware 保护。
func RegisterNodeRegisterAPI(publicMux, protectedMux *http.ServeMux, store nodes.Store, certFile string, evictClient func(string)) {
	publicMux.HandleFunc("POST /v1/node-register", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			NodeID  string `json:"node_id"`
			BaseURL string `json:"base_url"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
			return
		}
		if req.NodeID == "" || req.BaseURL == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "node_id and base_url are required"})
			return
		}
		node, err := store.Get(req.NodeID)
		if err != nil {
			if errors.Is(err, nodes.ErrNodeNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]any{"error": "node not found"})
			} else {
				writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			}
			return
		}
		node.BaseURL = strings.TrimRight(req.BaseURL, "/")
		if _, err := store.Upsert(node); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		evictClient(req.NodeID)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	})

	protectedMux.HandleFunc("GET /v1/node-setup/cert", func(w http.ResponseWriter, r *http.Request) {
		data, err := os.ReadFile(certFile)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "cert not available"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"cert_b64": base64.StdEncoding.EncodeToString(data),
		})
	})
}
