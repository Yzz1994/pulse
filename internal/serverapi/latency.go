package serverapi

import (
	"net/http"
	"time"

	"pulse/internal/nodes"
)

// handleLatency 返回所有节点在指定时间范围内的延迟采样。
//
// GET /v1/nodes/latency?from=RFC3339&to=RFC3339
// 默认 from = 1小时前，to = 现在。
func (a *API) handleLatency(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, http.MethodGet)
		return
	}

	now := time.Now().UTC()
	from := now.Add(-1 * time.Hour)
	to := now

	if s := r.URL.Query().Get("from"); s != "" {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid from: " + err.Error()})
			return
		}
		from = t
	}
	if s := r.URL.Query().Get("to"); s != "" {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid to: " + err.Error()})
			return
		}
		to = t
	}
	if !from.Before(to) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "from must be before to"})
		return
	}

	allNodes, err := a.store.List()
	if err != nil {
		internalError(w, r, err)
		return
	}
	nodeIDs := make([]string, 0, len(allNodes))
	nameMap := make(map[string]string, len(allNodes))
	for _, n := range allNodes {
		nodeIDs = append(nodeIDs, n.ID)
		nameMap[n.ID] = n.Name
	}

	samples, err := a.store.QueryLatencySamples(nodeIDs, from, to)
	if err != nil {
		internalError(w, r, err)
		return
	}
	if samples == nil {
		samples = []nodes.LatencySample{}
	}

	type sampleResp struct {
		NodeID    string    `json:"node_id"`
		NodeName  string    `json:"node_name"`
		ISP       string    `json:"isp"`
		RttMs     *int      `json:"rtt_ms"`
		SampledAt time.Time `json:"sampled_at"`
	}

	out := make([]sampleResp, 0, len(samples))
	for _, s := range samples {
		out = append(out, sampleResp{
			NodeID:    s.NodeID,
			NodeName:  nameMap[s.NodeID],
			ISP:       s.ISP,
			RttMs:     s.RttMs,
			SampledAt: s.SampledAt,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{"samples": out})
}
