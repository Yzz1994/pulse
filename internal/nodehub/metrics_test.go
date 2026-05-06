package nodehub

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestSnapshot_BasicCounts(t *testing.T) {
	hub := New(Options{PeerExtractor: mdPeerExtractor})
	env := newTestEnv(t, hub)

	// Offline call → ErrNodeOffline + 计数
	if _, err := hub.Call(context.Background(), "ghost", "x", nil); !errors.Is(err, ErrNodeOffline) {
		t.Fatalf("expected ErrNodeOffline, got %v", err)
	}
	snap := hub.Snapshot()
	if snap.CallsTotal != 1 || snap.CallsErrTotal != 1 || snap.CallsOfflineTotal != 1 {
		t.Fatalf("unexpected counters after offline call: %+v", snap)
	}
	if snap.OnlineCount != 0 || len(snap.OnlineNodeIDs) != 0 {
		t.Fatalf("expected zero online: %+v", snap)
	}

	// 上线一个节点
	cc := env.dial(t)
	mock := startNodeMock(t, cc, "n1")
	waitOnline(t, hub, "n1")

	mock.handle("ping", func(string, []byte) (bool, []byte, string) { return true, []byte("{}"), "" })

	if _, err := hub.Call(context.Background(), "n1", "ping", nil); err != nil {
		t.Fatalf("call: %v", err)
	}

	snap = hub.Snapshot()
	if snap.OnlineCount != 1 {
		t.Fatalf("OnlineCount=%d, want 1", snap.OnlineCount)
	}
	if len(snap.OnlineNodeIDs) != 1 || snap.OnlineNodeIDs[0] != "n1" {
		t.Fatalf("OnlineNodeIDs=%v", snap.OnlineNodeIDs)
	}
	if snap.CallsTotal != 2 {
		t.Fatalf("CallsTotal=%d, want 2", snap.CallsTotal)
	}
	if snap.CallsErrTotal != 1 {
		t.Fatalf("CallsErrTotal=%d, want 1", snap.CallsErrTotal)
	}
	if snap.CallLatencyAvgNs == 0 {
		t.Fatalf("expected non-zero latency avg")
	}
	if ts, ok := snap.LastSeen["n1"]; !ok || ts == 0 {
		t.Fatalf("LastSeen missing for n1: %+v", snap.LastSeen)
	}
}

func TestSnapshot_PushUsageAck(t *testing.T) {
	rp := &recordingPush{failOn: map[uint64]bool{2: true}}
	hub := New(Options{PeerExtractor: mdPeerExtractor, PushHandler: rp})
	env := newTestEnv(t, hub)
	cc := env.dial(t)
	mock := startNodeMock(t, cc, "u")
	waitOnline(t, hub, "u")

	mock.pushUsage(1, []byte(`{}`))
	mock.pushUsage(2, []byte(`{}`)) // 失败 → 不 ack
	mock.pushUsage(3, []byte(`{}`))

	// 等 acks
	got := 0
	deadline := time.After(2 * time.Second)
	for got < 2 {
		select {
		case <-mock.acks:
			got++
		case <-deadline:
			t.Fatal("timed out waiting for acks")
		}
	}

	// 给 server 一点时间更新计数（pushUsageTotal 在 dispatch 内更新，acks 已经收到，
	// 但 pushUsageAckTotal 在 send 后立即 +1，可能稍滞后）
	time.Sleep(50 * time.Millisecond)

	snap := hub.Snapshot()
	if snap.PushUsageTotal != 3 {
		t.Fatalf("PushUsageTotal=%d, want 3", snap.PushUsageTotal)
	}
	if snap.PushUsageAckTotal != 2 {
		t.Fatalf("PushUsageAckTotal=%d, want 2", snap.PushUsageAckTotal)
	}
}

func TestSnapshot_ReconnectAndDisconnect(t *testing.T) {
	hub := New(Options{PeerExtractor: mdPeerExtractor})
	env := newTestEnv(t, hub)

	cc1 := env.dial(t)
	mock1 := startNodeMock(t, cc1, "node-x")
	waitOnline(t, hub, "node-x")

	cc2 := env.dial(t)
	_ = startNodeMock(t, cc2, "node-x")

	// 旧 session 退出
	_ = cc1.Close()
	select {
	case <-mock1.done:
	case <-time.After(2 * time.Second):
		t.Fatal("old session did not exit")
	}

	// 等到 onlineGauge 稳定为 1
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if hub.Snapshot().OnlineCount == 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	snap := hub.Snapshot()
	if snap.OnlineCount != 1 {
		t.Fatalf("OnlineCount=%d, want 1", snap.OnlineCount)
	}
	if snap.ReconnectTotal != 1 {
		t.Fatalf("ReconnectTotal=%d, want 1", snap.ReconnectTotal)
	}

	// Disconnect
	hub.Disconnect("node-x")
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if hub.Snapshot().OnlineCount == 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	snap = hub.Snapshot()
	if snap.OnlineCount != 0 {
		t.Fatalf("OnlineCount after disconnect=%d, want 0", snap.OnlineCount)
	}
	if _, ok := snap.LastSeen["node-x"]; ok {
		t.Fatalf("LastSeen should be cleared after disconnect")
	}
}
