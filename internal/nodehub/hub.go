// Package nodehub 实现控制面侧的 gRPC node 长连接管理：
//
//   - 接受 node 发起的双向流（NodeAgent.Session）
//   - 维护 nodeID → conn 的注册表
//   - 提供 Call(nodeID, method, body) 的请求-响应封装（在 ServerMessage/NodeMessage
//     上叠加 reqID 关联）
//   - 通过 PushHandler 把 node 主动推送（hello / usage_push / log / traceroute_hop）
//     交给业务层处理
//
// 报文约定：
//
//   - 请求-响应：server 发 ServerMessage{Id: reqID, Method, Body}，node 必须以
//     NodeMessage{Id: reqID, Ok, Body|Error} 回复。Id 为空且 Event 为空的 NodeMessage
//     会被丢弃。
//   - usage_push ack：node 推送 NodeMessage{Event:"usage_push", Seq:N, Body}, Id 留空。
//     server 在 PushHandler.OnUsagePush 返回 nil 后，主动发送
//     ServerMessage{Method:"ack", Body: {"seq":N}} 作为 ack。Id 字段对 ack 不使用。
//     （node 端只需根据 Method=="ack" 来识别 ack 帧，并解析 Body 中的 seq。）
//   - 其他 event（hello / log / traceroute_hop）由 PushHandler 自行处理，hub 不主动 ack。
//   - 取消：调用方 ctx 取消时，hub 会 best-effort 发送 ServerMessage{CancelId: reqID}，
//     node 端可据此中断流式调用。
package nodehub

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	nodev1 "pulse/internal/pb/nodev1"
)

// PushHandler 处理 node 主动推送的事件。
// 实现可注入到 Hub.Options，用于解耦 hub 与业务逻辑（usage 入库、SSE 转发等）。
type PushHandler interface {
	OnHello(nodeID string, body []byte)
	// OnUsagePush 返回 nil 时，hub 会向 node ack。
	OnUsagePush(nodeID string, seq uint64, body []byte) error
	OnLog(nodeID, reqID string, body []byte)
	OnTracerouteHop(nodeID, reqID string, body []byte)
}

// NoopPushHandler 是 PushHandler 的空实现：OnUsagePush 立即 ack，其他方法 no-op。
// 用于尚未接入业务的过渡期或测试。
type NoopPushHandler struct{}

func (NoopPushHandler) OnHello(string, []byte)                   {}
func (NoopPushHandler) OnUsagePush(string, uint64, []byte) error { return nil }
func (NoopPushHandler) OnLog(string, string, []byte)             {}
func (NoopPushHandler) OnTracerouteHop(string, string, []byte)   {}

// PeerExtractor 从 stream context 中提取 nodeID（生产环境从 client TLS cert CN 取）。
// 抽象出来主要是为了测试可绕过 mTLS。
type PeerExtractor func(ctx context.Context) (nodeID string, err error)

func extractCNFromState(state tls.ConnectionState) (string, error) {
	if len(state.PeerCertificates) == 0 {
		return "", errors.New("nodehub: no peer certificates")
	}
	cn := state.PeerCertificates[0].Subject.CommonName
	if cn == "" {
		return "", errors.New("nodehub: peer cert has empty CN")
	}
	return cn, nil
}

// Options 配置 Hub。
type Options struct {
	Logger        *slog.Logger
	PushHandler   PushHandler
	PeerExtractor PeerExtractor // 默认基于 mTLS client cert CN（见 peer.go）

	// DeadConnectionTimeout 是 reaper 判定一个 node 连接已死的最长无帧时间。
	// 超过此时长（自上次收到任意帧起）未收到帧的连接会被强制关闭。
	// 默认 60s；ReaperInterval 默认 10s（不可外部调）；测试通过 ReaperInterval 字段加速。
	DeadConnectionTimeout time.Duration
	// ReaperInterval 是 reaper 扫描间隔。零值时使用默认 10s。仅供测试或精细调优使用。
	ReaperInterval time.Duration
}

// Hub 维护 nodeID → 连接 的注册表，并实现 NodeAgentServer。
type Hub struct {
	nodev1.UnimplementedNodeAgentServer

	logger        *slog.Logger
	pushHandler   PushHandler
	peerExtractor PeerExtractor

	deadConnTimeout time.Duration
	reaperInterval  time.Duration

	mu    sync.RWMutex
	conns map[string]*conn

	metrics *metrics
}

type conn struct {
	hub        *Hub
	nodeID     string
	stream     nodev1.NodeAgent_SessionServer
	sendMu     sync.Mutex // grpc stream Send 不能并发
	pending    sync.Map   // reqID(string) → chan *nodev1.NodeMessage
	streamSubs sync.Map   // reqID(string) → *Stream（流式调用订阅）
	closed     chan struct{}
	closeOnce  sync.Once
}

// New 构造 Hub。
func New(opts Options) *Hub {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	ph := opts.PushHandler
	if ph == nil {
		ph = NoopPushHandler{}
	}
	pe := opts.PeerExtractor
	if pe == nil {
		pe = mtlsPeerExtractor
	}
	dct := opts.DeadConnectionTimeout
	if dct <= 0 {
		dct = 60 * time.Second
	}
	ri := opts.ReaperInterval
	if ri <= 0 {
		ri = 10 * time.Second
	}
	return &Hub{
		logger:          logger,
		pushHandler:     ph,
		peerExtractor:   pe,
		deadConnTimeout: dct,
		reaperInterval:  ri,
		conns:           make(map[string]*conn),
		metrics:         newMetrics(),
	}
}

// RunReaper 阻塞运行死连接清理：每 ReaperInterval 扫描一次注册表，
// 关闭超过 DeadConnectionTimeout 没收到任何帧的连接。ctx 取消时返回。
// 一般由 ListenAndServe 在 goroutine 中调用；外部直接持有 Hub 时也可手动调。
func (h *Hub) RunReaper(ctx context.Context) {
	t := time.NewTicker(h.reaperInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			h.reapOnce(now)
		}
	}
}

func (h *Hub) reapOnce(now time.Time) {
	threshold := now.Add(-h.deadConnTimeout)
	h.metrics.mu.RLock()
	stale := make([]string, 0)
	for id, ts := range h.metrics.perNodeLastSeen {
		if ts.Before(threshold) {
			stale = append(stale, id)
		}
	}
	h.metrics.mu.RUnlock()
	if len(stale) == 0 {
		return
	}
	for _, id := range stale {
		h.mu.RLock()
		c, ok := h.conns[id]
		h.mu.RUnlock()
		if !ok {
			continue
		}
		h.logger.Warn("nodehub: reaping dead connection",
			"node_id", id, "timeout", h.deadConnTimeout)
		h.metrics.reapedTotal.Add(1)
		// close() 仅关闭 closed 通道；Session goroutine select 到后会返回，
		// gRPC server 自动断开底层流。
		c.close()
	}
}

// Session 实现 NodeAgentServer：处理 node 的双向流。
func (h *Hub) Session(stream nodev1.NodeAgent_SessionServer) error {
	ctx := stream.Context()
	nodeID, err := h.peerExtractor(ctx)
	if err != nil {
		h.logger.Warn("nodehub: reject session, peer extract failed", "err", err)
		return err
	}
	if nodeID == "" {
		return errors.New("nodehub: empty nodeID")
	}

	c := &conn{
		hub:    h,
		nodeID: nodeID,
		stream: stream,
		closed: make(chan struct{}),
	}

	// 注册（如有旧连接，关闭旧的）
	h.mu.Lock()
	if old, ok := h.conns[nodeID]; ok {
		h.logger.Info("nodehub: node reconnected, closing old session", "node_id", nodeID)
		old.close()
		h.metrics.reconnectTotal.Add(1)
		// 旧连接占用的 onlineGauge 会在其 Session goroutine 退出时减一；
		// 这里不重复增减，新连接下面统一 +1。
		h.metrics.onlineGauge.Add(-1)
	}
	h.conns[nodeID] = c
	h.mu.Unlock()
	h.metrics.onlineGauge.Add(1)
	h.metrics.markSeen(nodeID)

	defer func() {
		h.mu.Lock()
		// 仅当当前注册的还是自己时才删除（避免被新连接挤掉时误删）
		stillOurs := h.conns[nodeID] == c
		if stillOurs {
			delete(h.conns, nodeID)
		}
		h.mu.Unlock()
		if stillOurs {
			h.metrics.onlineGauge.Add(-1)
			h.metrics.forgetNode(nodeID)
		}
		c.close()
	}()

	// 阻塞读 NodeMessage：用独立 goroutine pump，主循环可同时 select c.closed
	// （reaper 强制关闭时主动返回，不等待下次 Recv 自然失败）。
	type recvResult struct {
		msg *nodev1.NodeMessage
		err error
	}
	recvCh := make(chan recvResult, 1)
	go func() {
		for {
			msg, err := stream.Recv()
			select {
			case recvCh <- recvResult{msg: msg, err: err}:
			case <-c.closed:
				return
			}
			if err != nil {
				return
			}
		}
	}()

	for {
		select {
		case r := <-recvCh:
			if r.err != nil {
				h.logger.Debug("nodehub: stream recv ended", "node_id", nodeID, "err", r.err)
				return nil
			}
			h.dispatch(c, r.msg)
		case <-c.closed:
			h.logger.Debug("nodehub: session closed by hub", "node_id", nodeID)
			return nil
		case <-ctx.Done():
			return nil
		}
	}
}

func (h *Hub) dispatch(c *conn, msg *nodev1.NodeMessage) {
	h.metrics.markSeen(c.nodeID)
	event := msg.GetEvent()
	if event != "" {
		// 优先派发到流订阅；命中则不再走 PushHandler。
		if id := msg.GetId(); id != "" {
			if v, ok := c.streamSubs.Load(id); ok {
				s := v.(*Stream)
				s.deliver(StreamFrame{Event: event, Body: append([]byte(nil), msg.GetBody()...)})
				return
			}
		}
		h.handleEvent(c, msg)
		return
	}
	id := msg.GetId()
	if id == "" {
		// 既无 id 也无 event → 丢弃
		return
	}
	// 流终态：node 在流结束时回一条普通响应（Ok=true 或 Ok=false+Error）
	if v, ok := c.streamSubs.Load(id); ok {
		s := v.(*Stream)
		var endErr error
		if !msg.GetOk() {
			es := msg.GetError()
			if es == "" {
				es = "node returned not-ok with empty error"
			}
			endErr = errors.New(es)
		}
		s.closeFromHub(endErr)
		return
	}
	if v, ok := c.pending.Load(id); ok {
		ch := v.(chan *nodev1.NodeMessage)
		select {
		case ch <- msg:
		default:
			// 通道已满（不应发生，buffer=1 且只投递一次）
		}
	}
}

func (h *Hub) handleEvent(c *conn, msg *nodev1.NodeMessage) {
	body := msg.GetBody()
	switch msg.GetEvent() {
	case "hello":
		h.pushHandler.OnHello(c.nodeID, body)
	case "usage_push":
		seq := msg.GetSeq()
		h.metrics.pushUsageTotal.Add(1)
		if err := h.pushHandler.OnUsagePush(c.nodeID, seq, body); err != nil {
			h.logger.Warn("nodehub: usage_push handler failed; not acking",
				"node_id", c.nodeID, "seq", seq, "err", err)
			return
		}
		ackBody := []byte(fmt.Sprintf(`{"seq":%d}`, seq))
		if err := c.send(&nodev1.ServerMessage{
			Method: "ack",
			Body:   ackBody,
		}); err != nil {
			h.logger.Warn("nodehub: send usage ack failed",
				"node_id", c.nodeID, "seq", seq, "err", err)
			return
		}
		h.metrics.pushUsageAckTotal.Add(1)
	case "log":
		h.pushHandler.OnLog(c.nodeID, msg.GetId(), body)
	case "traceroute_hop":
		h.pushHandler.OnTracerouteHop(c.nodeID, msg.GetId(), body)
	default:
		h.logger.Debug("nodehub: unknown event", "node_id", c.nodeID, "event", msg.GetEvent())
	}
}

func (c *conn) send(msg *nodev1.ServerMessage) error {
	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	return c.stream.Send(msg)
}

func (c *conn) close() {
	c.closeOnce.Do(func() {
		close(c.closed)
	})
	// 关闭所有未结束的流订阅，避免 caller 永久阻塞。
	// 放在 closeOnce 之外是为了允许 close() 多次调用都执行（虽然实际通常只一次）。
	c.streamSubs.Range(func(key, value any) bool {
		s := value.(*Stream)
		s.closeFromHub(ErrNodeOffline)
		return true
	})
}

// IsOnline 返回指定 nodeID 是否当前在线。
func (h *Hub) IsOnline(nodeID string) bool {
	h.mu.RLock()
	_, ok := h.conns[nodeID]
	h.mu.RUnlock()
	return ok
}

// OnlineNodes 返回当前在线的所有 nodeID。
func (h *Hub) OnlineNodes() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]string, 0, len(h.conns))
	for id := range h.conns {
		out = append(out, id)
	}
	return out
}

// Disconnect 主动踢掉某节点。
// 内部：从注册表删除并关闭 closed 通道；当前 Session goroutine 仍阻塞在 Recv 上，
// 实际断开依赖 stream context 取消，由调用方根据需要再调用 grpc 层取消（生产场景下
// 通常是配合 GracefulStop 或注销节点元数据后由 client 端自行重连）。
func (h *Hub) Disconnect(nodeID string) {
	h.mu.Lock()
	c, ok := h.conns[nodeID]
	if ok {
		delete(h.conns, nodeID)
	}
	h.mu.Unlock()
	if ok {
		h.metrics.onlineGauge.Add(-1)
		h.metrics.forgetNode(nodeID)
		c.close()
	}
}
