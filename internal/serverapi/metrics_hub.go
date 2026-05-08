package serverapi

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"pulse/internal/nodes"
)

type nodeMetricsItem struct {
	NodeID        string `json:"node_id"`
	Running       bool   `json:"running"`
	UploadSpeed   int64  `json:"upload_speed"`
	DownloadSpeed int64  `json:"download_speed"`
	Connections   int    `json:"connections"`
}

// metricsHub 集中采集节点指标，广播给所有 SSE 订阅方。
// 替代每个 SSE 连接独立轮询 DB 的模式，防止大量并发 SSE 耗尽 pgxpool 连接。
type metricsHub struct {
	mu   sync.Mutex
	subs map[chan []byte]struct{}
	last []byte // 最近一帧，新订阅者立即收到
}

func newMetricsHub() *metricsHub {
	return &metricsHub{
		subs: make(map[chan []byte]struct{}),
	}
}

func (h *metricsHub) subscribe() chan []byte {
	ch := make(chan []byte, 1)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	if h.last != nil {
		ch <- h.last // 立即发送最近一帧，不让新连接等最多 2 秒
	}
	h.mu.Unlock()
	return ch
}

func (h *metricsHub) unsubscribe(ch chan []byte) {
	h.mu.Lock()
	delete(h.subs, ch)
	h.mu.Unlock()
}

func (h *metricsHub) broadcast(data []byte) {
	h.mu.Lock()
	h.last = data
	subs := make([]chan []byte, 0, len(h.subs))
	for ch := range h.subs {
		subs = append(subs, ch)
	}
	h.mu.Unlock()
	for _, ch := range subs {
		select {
		case ch <- data:
		default:
			// 慢消费者跳过，下一帧再发
		}
	}
}

// Run 在后台持续采集节点指标并广播，ctx 取消时退出。
// 应在 nodeHub 注入完毕后调用（通过 API.StartMetricsHub）。
func (h *metricsHub) Run(ctx context.Context, store nodes.Store, clientFor func(string) (*nodes.Client, error)) {
	collect := func() {
		nodeList, err := store.List()
		if err != nil {
			return
		}
		results := make([]nodeMetricsItem, 0, len(nodeList))
		var mu sync.Mutex
		var wg sync.WaitGroup
		for _, node := range nodeList {
			if node.Disabled {
				continue
			}
			wg.Add(1)
			go func(n nodes.Node) {
				defer wg.Done()
				client, err := clientFor(n.ID)
				if err != nil {
					return
				}
				cctx, cancel := context.WithTimeout(ctx, 2*time.Second)
				defer cancel()
				stats, err := client.Usage(cctx, false)
				if err != nil {
					return
				}
				mu.Lock()
				results = append(results, nodeMetricsItem{
					NodeID:        n.ID,
					Running:       stats.Running,
					UploadSpeed:   stats.UploadSpeed,
					DownloadSpeed: stats.DownloadSpeed,
					Connections:   stats.Connections,
				})
				mu.Unlock()
			}(node)
		}
		wg.Wait()
		data, err := json.Marshal(results)
		if err != nil {
			return
		}
		h.broadcast(data)
	}

	collect()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			collect()
		}
	}
}
