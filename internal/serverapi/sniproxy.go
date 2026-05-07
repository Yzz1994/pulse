package serverapi

import (
	"context"
	"log"
	"net/http"
	"strconv"
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

// handleNodeTraffic 返回节点最近 N 天的日流量趋势：GET /v1/nodes/:id/traffic?days=30。
// 数据来源：node_daily_usage（由 SyncUsage 任务每分钟累加）。
func (a *API) handleNodeTraffic(w http.ResponseWriter, r *http.Request, nodeID string) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, http.MethodGet)
		return
	}
	days := 30
	if v := strings.TrimSpace(r.URL.Query().Get("days")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 365 {
			days = n
		}
	}
	now := time.Now().UTC()
	until := now.Format("2006-01-02")
	since := now.AddDate(0, 0, -(days - 1)).Format("2006-01-02")
	rows, err := a.store.ListNodeDailyUsageRange(nodeID, since, until)
	if err != nil {
		writeNodeError(w, err)
		return
	}
	type point struct {
		Date          string `json:"date"`
		UploadBytes   int64  `json:"upload_bytes"`
		DownloadBytes int64  `json:"download_bytes"`
	}
	out := make([]point, 0, len(rows))
	for _, r := range rows {
		out = append(out, point{Date: r.Date, UploadBytes: r.UploadBytes, DownloadBytes: r.DownloadBytes})
	}
	writeJSON(w, http.StatusOK, map[string]any{"days": days, "points": out})
}

// handleNodeChecksGet 返回节点最新解锁检测结果：GET /v1/nodes/:id/checks
func (a *API) handleNodeChecksGet(w http.ResponseWriter, r *http.Request, nodeID string) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, http.MethodGet)
		return
	}
	all, err := a.store.ListAllNodeCheckResults()
	if err != nil {
		writeNodeError(w, err)
		return
	}
	results := all[nodeID]
	type item struct {
		Service   string `json:"service"`
		CheckType string `json:"check_type"`
		Unlocked  bool   `json:"unlocked"`
		Region    string `json:"region,omitempty"`
		Note      string `json:"note,omitempty"`
		CheckedAt string `json:"checked_at"`
	}
	out := make([]item, 0, len(results))
	for _, r := range results {
		out = append(out, item{
			Service:   r.Service,
			CheckType: r.CheckType,
			Unlocked:  r.Unlocked,
			Region:    r.Region,
			Note:      r.Note,
			CheckedAt: r.CheckedAt.UTC().Format(time.RFC3339),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": out})
}

// handleNodeSpeedTestGet 返回节点最近一次测速结果：GET /v1/nodes/:id/speedtest
func (a *API) handleNodeSpeedTestGet(w http.ResponseWriter, r *http.Request, nodeID string) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, http.MethodGet)
		return
	}
	all, err := a.store.ListAllNodeSpeedTests()
	if err != nil {
		writeNodeError(w, err)
		return
	}
	if v, ok := all[nodeID]; ok {
		writeJSON(w, http.StatusOK, map[string]any{
			"down_bps":  v.DownBps,
			"up_bps":    v.UpBps,
			"tested_at": v.TestedAt.UTC().Format(time.RFC3339),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{})
}
