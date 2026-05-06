package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"sync/atomic"
	"time"

	"pulse/internal/inbounds"
	"pulse/internal/nodes"
	"pulse/internal/outbounds"
	"pulse/internal/users"
)

// HubCaller 是 self-sync 调用 node 的最小接口。
//
// 真实实现是 *nodehub.Hub.Call；用接口包装是为了避免 jobs 反向依赖 nodehub
// 形成循环 import，并方便测试 mock。
type HubCaller interface {
	Call(ctx context.Context, nodeID, method string, body any) (json.RawMessage, error)
}

// SelfSyncHandler 实现 nodehub.PushHandler 的 OnHello 逻辑：
// 比对 hello 帧上报的 config_hash 与 server 端期望 hash，不一致时异步触发
// 一次完整配置下发（ApplyNode）。
//
// 设计点：
//   - OnHello 必须立即返回，不阻塞 hub 的 recv loop；耗时操作 go func。
//   - 使用独立的 5 分钟 ctx，不依赖 stream lifecycle。
//   - ApplyNode 失败仅记日志；下一次 hello 自然重试。
type SelfSyncHandler struct {
	UserStore     users.Store
	InboundStore  inbounds.InboundStore
	OutboundStore outbounds.Store
	NodeStore     nodes.Store
	HubCaller     HubCaller
	ApplyOpts     ApplyOptions

	// Logger 可选，零值用 log 包默认 logger。
	Logger *log.Logger

	// ApplyTimeout 单次 self-sync 配置下发超时。零值默认 5 分钟。
	ApplyTimeout time.Duration

	// 测试钩子：每次异步 ApplyNode 完成后回调（成功/失败均回调）。零值不调用。
	OnApplyDone func(nodeID string, err error)

	// 计数器，便于测试断言与可观测。
	helloCount    atomic.Int64
	mismatchCount atomic.Int64
	applyOKCount  atomic.Int64
	applyErrCount atomic.Int64
}

// HelloCount 返回处理过的 hello 总数（含 hash 匹配 / 不匹配）。
func (s *SelfSyncHandler) HelloCount() int64 { return s.helloCount.Load() }

// MismatchCount 返回 hash 不匹配（即触发了配置下发）的次数。
func (s *SelfSyncHandler) MismatchCount() int64 { return s.mismatchCount.Load() }

// ApplyOKCount 返回成功完成下发的次数。
func (s *SelfSyncHandler) ApplyOKCount() int64 { return s.applyOKCount.Load() }

// ApplyErrCount 返回下发失败的次数。
func (s *SelfSyncHandler) ApplyErrCount() int64 { return s.applyErrCount.Load() }

// OnHello 实现 nodehub.PushHandler 的 OnHello 帧处理。
// 同步路径只做解析 + 期望 hash 计算（DB 读），不命中才异步 ApplyNode。
func (s *SelfSyncHandler) OnHello(nodeID string, body []byte) {
	s.helloCount.Add(1)
	logger := s.logger()

	var hello struct {
		NodeID     string `json:"node_id"`
		ConfigHash string `json:"config_hash"`
		Version    string `json:"version"`
	}
	if len(body) > 0 {
		if err := json.Unmarshal(body, &hello); err != nil {
			logger.Printf("selfsync: node %s hello parse failed: %v", nodeID, err)
			return
		}
	}

	// 用一个短 ctx 做 hash 计算（DB 读）。失败不致命。
	calcCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	expected, err := ComputeNodeConfigHash(calcCtx, nodeID, s.UserStore, s.InboundStore, s.OutboundStore)
	cancel()
	if err != nil {
		logger.Printf("selfsync: node %s compute expected hash failed: %v", nodeID, err)
		return
	}

	if expected == hello.ConfigHash {
		logger.Printf("selfsync: node %s reconnected, hash match (version=%s)", nodeID, hello.Version)
		return
	}

	s.mismatchCount.Add(1)
	logger.Printf("selfsync: node %s reconnected, hash mismatch (version=%s expected=%s reported=%s); applying config",
		nodeID, hello.Version, short(expected), short(hello.ConfigHash))

	go s.applyAsync(nodeID)
}

func (s *SelfSyncHandler) applyAsync(nodeID string) {
	timeout := s.ApplyTimeout
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	err := s.applyNode(ctx, nodeID)
	if err != nil {
		s.applyErrCount.Add(1)
		s.logger().Printf("selfsync: node %s apply failed: %v (will retry on next hello)", nodeID, err)
	} else {
		s.applyOKCount.Add(1)
		s.logger().Printf("selfsync: node %s apply ok", nodeID)
	}
	if s.OnApplyDone != nil {
		s.OnApplyDone(nodeID, err)
	}
}

// applyNode 通过 HubCaller（而非直接 *nodes.Client）下发配置。
//
// 当前实现：构造一个走 HubCaller 的 NodeDialer，复用现有 ApplyNode 函数。
// nodes.Client 内部 RPC 路径是基于本地 dial 的；要走 hub 需要扩展 client 适配 hub。
// client-short todo 会做这个适配；本 todo 在该适配落地之前，回退为：
//   - 通过 HubCaller 直接调用 "ApplyNodeConfig" method（如未来定义），或
//   - 找不到 hub-aware client 适配时，记录降级日志并返回 ErrHubClientNotWired。
//
// 为不阻塞本 todo，这里采用最简方案：要求调用方在初始化 SelfSyncHandler 时
// 注入 HubCaller 适配过的 NodeDialer（通过 Wire），在缺失时返回错误。
func (s *SelfSyncHandler) applyNode(ctx context.Context, nodeID string) error {
	if s.HubCaller == nil {
		return errors.New("selfsync: HubCaller not configured")
	}
	dial := hubDialer(s.HubCaller)
	return ApplyNode(ctx, nodeID, s.NodeStore, s.UserStore, s.InboundStore, s.OutboundStore, dial, s.ApplyOpts)
}

func (s *SelfSyncHandler) logger() *log.Logger {
	if s.Logger != nil {
		return s.Logger
	}
	return log.Default()
}

func short(h string) string {
	if len(h) <= 12 {
		return h
	}
	return h[:12]
}

// hubDialer 构造一个走 HubCaller 的 NodeDialer。
//
// 当前 nodes.Client 仍使用本地 dial 的实现，client-short todo 会将其改为 hub-aware。
// 在该改动落地前，此 dialer 仍调用 nodes.NewHubClient（若存在）；不存在则返回错误。
// 为最小可用，此处委托给 nodes 包导出的工厂函数 NewHubClient（约定）。
//
// 如果 nodes 包尚未提供 NewHubClient，调用方可在 wiring 阶段提供自定义
// NodeDialer 替代（见 SelfSyncHandler 的扩展字段 CustomDialer）。
func hubDialer(hc HubCaller) NodeDialer {
	return func(nodeID string) (*nodes.Client, error) {
		if f := nodesNewHubClient; f != nil {
			return f(nodeID, hc), nil
		}
		return nil, errors.New("selfsync: nodes.NewHubClient not wired; cannot dial via hub")
	}
}

// nodesNewHubClient 是一个可注入的工厂；server.go 在 wire-up 时通过
// SetNodesHubClientFactory 注入，避免 jobs 强依赖 nodes 包内未来才会提供的 API。
var nodesNewHubClient func(nodeID string, hc HubCaller) *nodes.Client

// SetNodesHubClientFactory 注入 hub-aware client 工厂。
//
// 由 client-short todo 提供的 nodes.NewHubClient 可直接传入：
//
//	jobs.SetNodesHubClientFactory(func(id string, hc jobs.HubCaller) *nodes.Client {
//	    return nodes.NewHubClient(id, hubCallerAdapter{hc})
//	})
func SetNodesHubClientFactory(f func(nodeID string, hc HubCaller) *nodes.Client) {
	nodesNewHubClient = f
}
