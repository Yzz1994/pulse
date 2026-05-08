package serverapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"pulse/internal/nodes"
)

// RegisterNodeRegisterAPI 注册节点自注册接口（公开，无鉴权）。节点启动时
// 通过它把自身 BaseURL 上报给控制面，控制面 evict 节点 client 缓存。
func RegisterNodeRegisterAPI(publicMux *http.ServeMux, store nodes.Store, evictClient func(string)) {
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
}
