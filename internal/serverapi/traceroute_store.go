package serverapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"pulse/internal/nodes"
)

// handleNodeTracerouteResults 处理 traceroute 结果的存取：
//
//	POST /v1/nodes/{id}/traceroute/results — 保存一条快照
//	GET  /v1/nodes/{id}/traceroute/results — 查询最近 N 条快照
func (a *API) handleNodeTracerouteResults(w http.ResponseWriter, r *http.Request, nodeID string) {
	switch r.Method {
	case http.MethodPost:
		a.saveTracerouteResult(w, r, nodeID)
	case http.MethodGet:
		a.listTracerouteResults(w, r, nodeID)
	default:
		writeMethodNotAllowed(w, http.MethodGet+", "+http.MethodPost)
	}
}

// handleTracerouteResultDelete 处理单条快照删除：DELETE /v1/nodes/{id}/traceroute/results/{snapshotID}
func (a *API) handleTracerouteResultDelete(w http.ResponseWriter, r *http.Request, snapshotID string) {
	if r.Method != http.MethodDelete {
		writeMethodNotAllowed(w, http.MethodDelete)
		return
	}
	if err := a.store.DeleteTracerouteSnapshot(snapshotID); err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *API) saveTracerouteResult(w http.ResponseWriter, r *http.Request, nodeID string) {
	var req struct {
		Direction string `json:"direction"`
		Target    string `json:"target"`
		Hops      string `json:"hops"`
		Quality   string `json:"quality"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json body"})
		return
	}
	if req.Direction == "" || req.Target == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "direction and target are required"})
		return
	}

	snapshot := nodes.TracerouteSnapshot{
		NodeID:    nodeID,
		Direction: req.Direction,
		Target:    req.Target,
		Hops:      req.Hops,
		Quality:   req.Quality,
		CreatedAt: time.Now().UTC(),
	}
	if err := a.store.SaveTracerouteSnapshot(snapshot); err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleAllTracerouteLatest 返回所有节点的最新追踪快照（管理员）。
// GET /v1/nodes/traceroute/latest
func (a *API) handleAllTracerouteLatest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, http.MethodGet)
		return
	}
	snapshots, err := a.store.ListLatestTracerouteSnapshots()
	if err != nil {
		internalError(w, r, err)
		return
	}
	if snapshots == nil {
		snapshots = map[string][]nodes.TracerouteSnapshot{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"snapshots": snapshots})
}

func (a *API) listTracerouteResults(w http.ResponseWriter, r *http.Request, nodeID string) {
	limit := 20
	if lStr := r.URL.Query().Get("limit"); lStr != "" {
		if v, err := strconv.Atoi(lStr); err == nil && v > 0 {
			limit = v
		}
	}
	results, err := a.store.ListNodeTracerouteSnapshots(nodeID, limit)
	if err != nil {
		internalError(w, r, err)
		return
	}
	if results == nil {
		results = []nodes.TracerouteSnapshot{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}
