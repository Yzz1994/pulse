package serverapi

import (
	"context"
	"log"
	"net/http"
	"strings"
	"time"

	"pulse/internal/jobs"
	"pulse/internal/nodes"
)

// doSyncNodeSNIProxy 从当前数据库状态生成节点的 sniproxy 配置并推送。
// 统一调用 jobs.BuildSNIProxySyncReq，与 ApplyNodeUsers 内的自动下发走同一套
// builder 避免两份逻辑漂移。
func (a *API) doSyncNodeSNIProxy(ctx context.Context, nodeID string) error {
	client, err := a.clientFor(nodeID)
	if err != nil {
		return err
	}

	node, err := a.store.Get(nodeID)
	if err != nil {
		return err
	}

	nodeInbounds, err := a.inboundStore.ListInboundsByNode(nodeID)
	if err != nil {
		return err
	}

	allNodeMap := make(map[string]nodes.Node)
	if list, nErr := a.store.List(); nErr == nil {
		for _, n := range list {
			allNodeMap[n.ID] = n
		}
	}

	cfToken := ""
	if a.settings != nil {
		if tok, _ := a.settings.GetSetting("cf_token"); tok != "" {
			cfToken = strings.TrimSpace(tok)
		}
	}

	req := jobs.BuildSNIProxySyncReq(node, nodeInbounds, a.inboundStore, allNodeMap, cfToken, ServerPort())
	log.Printf("sniproxy sync node=%s routes=%d cf_token_set=%v",
		nodeID, len(req.Routes), cfToken != "")
	return client.SyncSNIProxy(ctx, req)
}

// handleNodeSNIProxySync 暴露给面板的手动同步入口，POST /v1/nodes/{id}/sniproxy/sync。
// 处理节点 NodeGate (sniproxy) 同步请求。
func (a *API) handleNodeSNIProxySync(w http.ResponseWriter, r *http.Request, nodeID string) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, http.MethodPost)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	if err := a.doSyncNodeSNIProxy(ctx, nodeID); err != nil {
		writeNodeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleNodeSNIProxyStatus 代理到节点读取当前 SNI 代理的运行状态。
// 面板用这个接口展示监听端口、路由表、证书明细和错误。
func (a *API) handleNodeSNIProxyStatus(w http.ResponseWriter, r *http.Request, nodeID string) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, http.MethodGet)
		return
	}
	client, err := a.clientFor(nodeID)
	if err != nil {
		writeNodeError(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	status, err := client.SNIProxyStatus(ctx)
	if err != nil {
		writeNodeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}
