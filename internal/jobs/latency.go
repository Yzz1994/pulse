package jobs

import (
	"context"
	"log"
	"sync"
	"time"

	"pulse/internal/nodes"
)

const latencyProbeMaxConcurrency = 20

// SampleLatency 并发对所有活跃节点探测上海三网延迟并持久化。
// 由调度器每分钟调用一次。dial 用于获取每个节点的 RPC 客户端
// （生产环境绑定到 nodehub.Hub）。
func SampleLatency(ctx context.Context, nodeStore nodes.Store, dial NodeDialer) error {
	all, err := nodeStore.List()
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	var (
		mu      sync.Mutex
		samples []nodes.LatencySample
		wg      sync.WaitGroup
		sem     = make(chan struct{}, latencyProbeMaxConcurrency)
	)

	for _, n := range all {
		if n.Disabled || n.IsLanding {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(n nodes.Node) {
			defer wg.Done()
			defer func() { <-sem }()

			client, err := dial(n.ID)
			if err != nil {
				log.Printf("[latency] node %s dial error: %v", n.Name, err)
				return
			}
			probeCtx, cancel := context.WithTimeout(ctx, 6*time.Second)
			result, err := client.ProbeLatency(probeCtx)
			cancel()
			if err != nil {
				log.Printf("[latency] node %s probe error: %v", n.Name, err)
				return
			}

			local := make([]nodes.LatencySample, 0, 3)
			for isp, rtt := range map[string]*int{"ct": result.CT, "cu": result.CU, "cm": result.CM} {
				local = append(local, nodes.LatencySample{
					NodeID:    n.ID,
					ISP:       isp,
					RttMs:     rtt,
					SampledAt: now,
				})
			}
			mu.Lock()
			samples = append(samples, local...)
			mu.Unlock()
		}(n)
	}
	wg.Wait()

	if len(samples) == 0 {
		return nil
	}
	return nodeStore.SaveLatencySamples(samples)
}

// CleanupLatencySamples 清理超过 retainDays 天的采样数据。
func CleanupLatencySamples(ctx context.Context, nodeStore nodes.Store, retainDays int) error {
	before := time.Now().UTC().AddDate(0, 0, -retainDays)
	return nodeStore.CleanupOldLatencySamples(before)
}
