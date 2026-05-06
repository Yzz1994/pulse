// Package server 中本文件提供 self-sync 子系统的 wire-up：在 hub 启动时注入
// 一个 nodehub.MultiPushHandler，其中 HelloHandler 由 jobs.SelfSyncHandler
// 提供，从而在 node 重连 hello 帧到达时按需触发 ApplyNode。
//
// 当前文件仅暴露 SetupSelfSync / SelfSyncDeps 给 server.go 的主流程使用，
// 不直接修改 server.go（避免叠加在 pre-existing 编译错误之上）。
//
// 典型用法（server.go 启动顺序）：
//
//	hello, multi := server.SetupSelfSync(ctx, server.SelfSyncDeps{
//	    UserStore:     userStore,
//	    InboundStore:  ibStore,
//	    OutboundStore: outboundStore,
//	    NodeStore:     nodeStore,
//	    ApplyOpts:     applyOpts,
//	})
//	hub := nodehub.New(nodehub.Options{PushHandler: multi, ...})
//	server.SetupSelfSyncHubCaller(hello, hub)  // hub 创建好后回填 HubCaller
package server

import (
	"context"
	"encoding/json"
	"log/slog"

	"pulse/internal/inbounds"
	"pulse/internal/jobs"
	"pulse/internal/nodehub"
	"pulse/internal/nodes"
	"pulse/internal/outbounds"
	"pulse/internal/users"
)

// SelfSyncDeps 是 SetupSelfSync 的入参集合。
type SelfSyncDeps struct {
	UserStore     users.Store
	InboundStore  inbounds.InboundStore
	OutboundStore outbounds.Store
	NodeStore     nodes.Store
	ApplyOpts     jobs.ApplyOptions

	// 可选的 hop 处理器，零值时该事件 noop。
	UsagePushHandler     func(nodeID string, seq uint64, body []byte) error
	LogHandler           func(nodeID, reqID string, body []byte)
	TracerouteHopHandler func(nodeID, reqID string, body []byte)

	Logger *slog.Logger
}

// SetupSelfSync 构造 SelfSyncHandler 并把它的 OnHello 装到一个新的
// MultiPushHandler 中。其他事件（usage/log/traceroute）按 deps 中的可选闭包
// 装载，未提供则保持 noop。
//
// 返回 (helloHandler, multi)：
//   - helloHandler 用于后续注入 HubCaller（见 SetupSelfSyncHubCaller）；
//   - multi 直接传给 nodehub.New 的 PushHandler。
//
// ctx 仅用于将来扩展（例如订阅 store 变更主动重算 hash），当前未使用。
func SetupSelfSync(ctx context.Context, deps SelfSyncDeps) (*jobs.SelfSyncHandler, *nodehub.MultiPushHandler) {
	_ = ctx

	hello := &jobs.SelfSyncHandler{
		UserStore:     deps.UserStore,
		InboundStore:  deps.InboundStore,
		OutboundStore: deps.OutboundStore,
		NodeStore:     deps.NodeStore,
		ApplyOpts:     deps.ApplyOpts,
	}
	_ = deps.Logger // 预留：jobs.SelfSyncHandler 当前用 log 包默认 logger，未来对齐 slog 时使用

	multi := &nodehub.MultiPushHandler{
		HelloHandler:         hello.OnHello,
		UsagePushHandler:     deps.UsagePushHandler,
		LogHandler:           deps.LogHandler,
		TracerouteHopHandler: deps.TracerouteHopHandler,
	}

	return hello, multi
}

// SetupSelfSyncHubCaller 在 hub 实例化之后回填 HubCaller，使 self-sync
// 触发 ApplyNode 时能通过 hub 调用 node。
//
// 拆成两步是因为 hub 与 SelfSyncHandler 互相需要对方：hub 启动时需要
// PushHandler，而 PushHandler 触发的 ApplyNode 又需要 hub 来下发指令。
func SetupSelfSyncHubCaller(h *jobs.SelfSyncHandler, hub *nodehub.Hub) {
	if h == nil || hub == nil {
		return
	}
	h.HubCaller = hubCallerAdapter{hub: hub}
}

// hubCallerAdapter 把 *nodehub.Hub 适配为 jobs.HubCaller。
type hubCallerAdapter struct {
	hub *nodehub.Hub
}

func (a hubCallerAdapter) Call(ctx context.Context, nodeID, method string, body any) (json.RawMessage, error) {
	return a.hub.Call(ctx, nodeID, method, body)
}
