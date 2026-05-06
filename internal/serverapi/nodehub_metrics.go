package serverapi

import (
	"net/http"

	"pulse/internal/nodehub"
)

// nodeHubMetricsResponse 是 /v1/system/nodehub/metrics 的响应体。
// 在 nodehub.Snapshot 基础上合并 enroll 计数（避免再增加一个端点）。
type nodeHubMetricsResponse struct {
	nodehub.Snapshot
	EnrollSuccessTotal uint64 `json:"enroll_success_total"`
	EnrollFailureTotal uint64 `json:"enroll_failure_total"`
}

// RegisterNodeHubMetrics 在 protected mux 上注册 GET /v1/system/nodehub/metrics。
// 调用方负责把 mux 包在 admin auth middleware 中（与 /v1/system/info 一致）。
func RegisterNodeHubMetrics(mux *http.ServeMux, hub *nodehub.Hub) {
	if hub == nil {
		return
	}
	mux.HandleFunc("GET /v1/system/nodehub/metrics", func(w http.ResponseWriter, r *http.Request) {
		snap := hub.Snapshot()
		success, failure := EnrollMetrics()
		writeJSON(w, http.StatusOK, nodeHubMetricsResponse{
			Snapshot:           snap,
			EnrollSuccessTotal: success,
			EnrollFailureTotal: failure,
		})
	})
}
