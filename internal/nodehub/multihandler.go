package nodehub

// MultiPushHandler 是一个把 PushHandler 各事件分发到多个独立闭包的复合实现。
//
// nodehub 的 PushHandler 是单一接口，但实际有多个业务模块需要消费不同事件
// （self-sync 关心 OnHello、SSE bridge 关心 OnLog/OnTracerouteHop、usage
// 接收器关心 OnUsagePush）。MultiPushHandler 提供一个零依赖的扇出装配点：
//
//	hub := nodehub.New(nodehub.Options{
//	    PushHandler: &nodehub.MultiPushHandler{
//	        HelloHandler:         selfSync.OnHello,
//	        UsagePushHandler:     usageRecv.OnUsagePush,
//	        LogHandler:           sseBridge.OnLog,
//	        TracerouteHopHandler: sseBridge.OnTracerouteHop,
//	    },
//	})
//
// 任意字段为 nil 时该事件 no-op（OnUsagePush 返回 nil 触发 ack）。
type MultiPushHandler struct {
	HelloHandler         func(nodeID string, body []byte)
	UsagePushHandler     func(nodeID string, seq uint64, body []byte) error
	LogHandler           func(nodeID, reqID string, body []byte)
	TracerouteHopHandler func(nodeID, reqID string, body []byte)
}

var _ PushHandler = (*MultiPushHandler)(nil)

// OnHello 实现 PushHandler。
func (m *MultiPushHandler) OnHello(nodeID string, body []byte) {
	if m == nil || m.HelloHandler == nil {
		return
	}
	m.HelloHandler(nodeID, body)
}

// OnUsagePush 实现 PushHandler。零值字段返回 nil（hub 会立即 ack）。
func (m *MultiPushHandler) OnUsagePush(nodeID string, seq uint64, body []byte) error {
	if m == nil || m.UsagePushHandler == nil {
		return nil
	}
	return m.UsagePushHandler(nodeID, seq, body)
}

// OnLog 实现 PushHandler。
func (m *MultiPushHandler) OnLog(nodeID, reqID string, body []byte) {
	if m == nil || m.LogHandler == nil {
		return
	}
	m.LogHandler(nodeID, reqID, body)
}

// OnTracerouteHop 实现 PushHandler。
func (m *MultiPushHandler) OnTracerouteHop(nodeID, reqID string, body []byte) {
	if m == nil || m.TracerouteHopHandler == nil {
		return
	}
	m.TracerouteHopHandler(nodeID, reqID, body)
}
