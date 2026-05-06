package serverapi

import (
	"context"
	"encoding/json"

	"pulse/internal/nodes"
)

// fakeHub 是单测用的 hub 适配器：把 nodes.Client 的 RPC 方法名映射到
// 内联 handler，返回预置 JSON 响应。模拟节点行为而无需启动真实节点。
//
// 用法：
//
//	hub := &fakeHub{
//	    handlers: map[string]func(body any) (json.RawMessage, error){
//	        "Usage":   func(any) (json.RawMessage, error) { return json.RawMessage(`{"running":true}`), nil },
//	        "Restart": func(b any) (json.RawMessage, error) { capture(b); return json.RawMessage(`{}`), nil },
//	    },
//	}
//	client := nodes.NewClientWithHub("n1", hub)
type fakeHub struct {
	handlers map[string]func(body any) (json.RawMessage, error)
}

func (f *fakeHub) Call(_ context.Context, _ string, method string, body any) (json.RawMessage, error) {
	if h, ok := f.handlers[method]; ok {
		return h(body)
	}
	return json.RawMessage(`{}`), nil
}

// fakeHubClientFactory 返回一个 clientFactory，所有节点共用同一个 fakeHub。
func fakeHubClientFactory(hub *fakeHub) func(node nodes.Node) *nodes.Client {
	return func(node nodes.Node) *nodes.Client {
		return nodes.NewClientWithHub(node.ID, hub)
	}
}
