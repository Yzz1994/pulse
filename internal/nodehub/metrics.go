package nodehub

import (
	"sync"
	"sync/atomic"
	"time"
)

// metrics 是 Hub 的运行时计数器集合。
// 全部使用 atomic 字段，热路径不持锁；perNodeLastSeen 用 RWMutex 保护。
type metrics struct {
	onlineGauge       atomic.Int64  // 当前在线节点数
	callsTotal        atomic.Uint64 // 累计 Call 调用数
	callsErrTotal     atomic.Uint64 // Call 错误数（含 ErrNodeOffline、node 返回 not-ok、ctx 取消等）
	callsOfflineTotal atomic.Uint64 // Call 因为节点离线失败
	pushUsageTotal    atomic.Uint64 // 收到 usage_push 帧数
	pushUsageAckTotal atomic.Uint64 // 成功 ack 的 usage_push 数
	reconnectTotal    atomic.Uint64 // 同 nodeID 重连踢旧连接的次数
	reapedTotal       atomic.Uint64 // 因 keepalive 超时被 reaper 关闭的连接数

	callLatencyNanosSum atomic.Uint64
	callLatencyCount    atomic.Uint64

	mu              sync.RWMutex
	perNodeLastSeen map[string]time.Time
}

func newMetrics() *metrics {
	return &metrics{perNodeLastSeen: make(map[string]time.Time)}
}

func (m *metrics) markSeen(nodeID string) {
	now := time.Now()
	m.mu.Lock()
	m.perNodeLastSeen[nodeID] = now
	m.mu.Unlock()
}

func (m *metrics) forgetNode(nodeID string) {
	m.mu.Lock()
	delete(m.perNodeLastSeen, nodeID)
	m.mu.Unlock()
}

func (m *metrics) recordCallLatency(d time.Duration) {
	if d < 0 {
		d = 0
	}
	m.callLatencyNanosSum.Add(uint64(d.Nanoseconds()))
	m.callLatencyCount.Add(1)
}

// Snapshot 是 Hub 指标的对外只读视图。字段为 JSON 友好格式。
type Snapshot struct {
	OnlineCount       int64    `json:"online_count"`
	OnlineNodeIDs     []string `json:"online_node_ids"`
	CallsTotal        uint64   `json:"calls_total"`
	CallsErrTotal     uint64   `json:"calls_err_total"`
	CallsOfflineTotal uint64   `json:"calls_offline_total"`
	PushUsageTotal    uint64   `json:"push_usage_total"`
	PushUsageAckTotal uint64   `json:"push_usage_ack_total"`
	ReconnectTotal    uint64   `json:"reconnect_total"`
	ReapedTotal       uint64   `json:"reaped_total"`
	CallLatencyAvgNs  uint64   `json:"call_latency_avg_ns"`
	// LastSeen 单位为 unix 秒，缺失或 0 表示未观察到帧。
	LastSeen map[string]int64 `json:"last_seen"`
}

// Snapshot 返回当前 Hub 指标的快照。
func (h *Hub) Snapshot() Snapshot {
	online := h.OnlineNodes()

	sum := h.metrics.callLatencyNanosSum.Load()
	cnt := h.metrics.callLatencyCount.Load()
	var avg uint64
	if cnt > 0 {
		avg = sum / cnt
	}

	h.metrics.mu.RLock()
	lastSeen := make(map[string]int64, len(h.metrics.perNodeLastSeen))
	for k, v := range h.metrics.perNodeLastSeen {
		lastSeen[k] = v.Unix()
	}
	h.metrics.mu.RUnlock()

	return Snapshot{
		OnlineCount:       h.metrics.onlineGauge.Load(),
		OnlineNodeIDs:     online,
		CallsTotal:        h.metrics.callsTotal.Load(),
		CallsErrTotal:     h.metrics.callsErrTotal.Load(),
		CallsOfflineTotal: h.metrics.callsOfflineTotal.Load(),
		PushUsageTotal:    h.metrics.pushUsageTotal.Load(),
		PushUsageAckTotal: h.metrics.pushUsageAckTotal.Load(),
		ReconnectTotal:    h.metrics.reconnectTotal.Load(),
		ReapedTotal:       h.metrics.reapedTotal.Load(),
		CallLatencyAvgNs:  avg,
		LastSeen:          lastSeen,
	}
}
