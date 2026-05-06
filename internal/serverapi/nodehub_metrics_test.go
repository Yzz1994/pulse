package serverapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"pulse/internal/nodehub"
)

func TestRegisterNodeHubMetrics_OK(t *testing.T) {
	hub := nodehub.New(nodehub.Options{})
	mux := http.NewServeMux()
	RegisterNodeHubMetrics(mux, hub)

	// 触发一次 enroll 失败计数（包级原子，影响测试间状态；这里只检查字段存在）
	enrollFailureTotal.Add(1)

	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/system/nodehub/metrics")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, k := range []string{
		"online_count", "online_node_ids", "calls_total", "calls_err_total",
		"calls_offline_total", "push_usage_total", "push_usage_ack_total",
		"reconnect_total", "call_latency_avg_ns", "last_seen",
		"enroll_success_total", "enroll_failure_total",
	} {
		if _, ok := body[k]; !ok {
			t.Errorf("missing key %q in response: %v", k, body)
		}
	}
}

func TestRegisterNodeHubMetrics_NilHub(t *testing.T) {
	mux := http.NewServeMux()
	RegisterNodeHubMetrics(mux, nil) // 不应 panic 也不应注册路由

	srv := httptest.NewServer(mux)
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/v1/system/nodehub/metrics")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}
