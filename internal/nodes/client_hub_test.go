package nodes

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

type mockHub struct {
	lastNodeID string
	lastMethod string
	lastBody   any
	resp       json.RawMessage
	err        error
}

func (m *mockHub) Call(ctx context.Context, nodeID, method string, body any) (json.RawMessage, error) {
	m.lastNodeID = nodeID
	m.lastMethod = method
	m.lastBody = body
	return m.resp, m.err
}

func TestClient_Status_HubSuccess(t *testing.T) {
	mh := &mockHub{resp: json.RawMessage(`{"running":true}`)}
	c := NewClientWithHub("node-1", mh)

	st, err := c.Status(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !st.Running {
		t.Fatalf("expected running=true, got %+v", st)
	}
	if mh.lastNodeID != "node-1" || mh.lastMethod != "Status" {
		t.Fatalf("unexpected hub call: id=%q method=%q", mh.lastNodeID, mh.lastMethod)
	}
}

func TestClient_HubOffline_UsesSentinel(t *testing.T) {
	// 模拟一个未注册的 hub-离线错误：使用错误消息匹配兜底路径。
	mh := &mockHub{err: errors.New("nodehub: node offline")}
	c := NewClientWithHub("node-x", mh)

	_, err := c.Status(context.Background())
	if !errors.Is(err, ErrNodeOffline) {
		t.Fatalf("expected ErrNodeOffline, got %v", err)
	}
}

func TestClient_HubOffline_RegisteredSentinel(t *testing.T) {
	// 注册一个自定义 sentinel，验证 errors.Is 路径生效。
	custom := errors.New("custom-offline")
	RegisterHubOfflineError(custom)
	t.Cleanup(func() {
		// 复原：从全局 slice 中移除（保持其他测试隔离）。
		out := hubOfflineErrSentinels[:0]
		for _, e := range hubOfflineErrSentinels {
			if e != custom {
				out = append(out, e)
			}
		}
		hubOfflineErrSentinels = out
	})

	mh := &mockHub{err: custom}
	c := NewClientWithHub("node-y", mh)
	_, err := c.Status(context.Background())
	if !errors.Is(err, ErrNodeOffline) {
		t.Fatalf("expected ErrNodeOffline via sentinel, got %v", err)
	}
}

func TestClient_HubBadJSON_ReturnsDecodeError(t *testing.T) {
	mh := &mockHub{resp: json.RawMessage(`not-json`)}
	c := NewClientWithHub("n", mh)
	_, err := c.Status(context.Background())
	if err == nil {
		t.Fatal("expected decode error")
	}
}

func TestClient_HubGenericError_PassThrough(t *testing.T) {
	mh := &mockHub{err: errors.New("boom")}
	c := NewClientWithHub("n", mh)
	err := c.AddUser(context.Background(), UserChangeRequest{InboundTag: "t"})
	if err == nil || err.Error() != "boom" {
		t.Fatalf("expected pass-through 'boom', got %v", err)
	}
}

func TestClient_RemoveUser_HubBody(t *testing.T) {
	mh := &mockHub{resp: json.RawMessage(`{}`)}
	c := NewClientWithHub("n", mh)
	if err := c.RemoveUser(context.Background(), "tag-1", "user@@@tag-1"); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	got, ok := mh.lastBody.(map[string]string)
	if !ok {
		t.Fatalf("expected map body, got %T", mh.lastBody)
	}
	if got["inbound_tag"] != "tag-1" || got["email"] != "user@@@tag-1" {
		t.Fatalf("unexpected body: %+v", got)
	}
	if mh.lastMethod != "RemoveUser" {
		t.Fatalf("unexpected method: %q", mh.lastMethod)
	}
}

func TestClient_NilHub_ReturnsErrHubNotConfigured(t *testing.T) {
	// hub == nil 是偏执检查路径：所有方法都应返回 ErrHubNotConfigured，
	// 而不是 panic。生产环境 server.Run 始终通过 SetNodeHub 注入 hub。
	c := NewClientWithHub("n", nil)
	if _, err := c.Status(context.Background()); !errors.Is(err, ErrHubNotConfigured) {
		t.Fatalf("expected ErrHubNotConfigured, got %v", err)
	}
	if _, err := c.LogsStream(context.Background()); !errors.Is(err, ErrHubNotConfigured) {
		t.Fatalf("expected ErrHubNotConfigured for stream, got %v", err)
	}
}
