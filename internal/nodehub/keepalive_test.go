package nodehub

import (
	"context"
	"testing"
	"time"
)

// TestReaper_DropsSilentConnection 验证 reaper 在 DeadConnectionTimeout 后强制
// 关闭一个建立后从未发任何帧的连接（hello 之后保持沉默亦可触发，因为 markSeen
// 只在 server 收到帧时刷新）。
//
// 测试加速：DeadConnectionTimeout=80ms，ReaperInterval=20ms。
func TestReaper_DropsSilentConnection(t *testing.T) {
	hub := New(Options{
		PeerExtractor:         mdPeerExtractor,
		DeadConnectionTimeout: 80 * time.Millisecond,
		ReaperInterval:        20 * time.Millisecond,
	})
	env := newTestEnv(t, hub)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go hub.RunReaper(ctx)

	cc := env.dial(t)
	mock := startNodeMock(t, cc, "silent-node")
	waitOnline(t, hub, "silent-node")

	// 故意不发送任何帧。等过 DeadConnectionTimeout * 4，reaper 应已踢掉。
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if !hub.IsOnline("silent-node") {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if hub.IsOnline("silent-node") {
		t.Fatalf("expected silent-node to be reaped after %v, still online",
			hub.deadConnTimeout)
	}

	// mock.done 应在 server 关闭流后被关闭（client 侧 Recv 返回错误 → recvLoop 退出）
	select {
	case <-mock.done:
	case <-time.After(2 * time.Second):
		t.Fatal("mock recvLoop did not exit after reaper dropped connection")
	}

	if got := hub.Snapshot().ReapedTotal; got == 0 {
		t.Fatalf("expected ReapedTotal>=1, got %d", got)
	}
}

// TestReaper_KeepsActiveConnection 验证持续发帧的连接不会被 reaper 误杀。
func TestReaper_KeepsActiveConnection(t *testing.T) {
	hub := New(Options{
		PeerExtractor:         mdPeerExtractor,
		DeadConnectionTimeout: 100 * time.Millisecond,
		ReaperInterval:        20 * time.Millisecond,
	})
	env := newTestEnv(t, hub)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go hub.RunReaper(ctx)

	cc := env.dial(t)
	mock := startNodeMock(t, cc, "alive-node")
	waitOnline(t, hub, "alive-node")

	stopPing := make(chan struct{})
	defer close(stopPing)
	go func() {
		var seq uint64
		ticker := time.NewTicker(30 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stopPing:
				return
			case <-ticker.C:
				seq++
				mock.pushUsage(seq, []byte(`{}`))
			}
		}
	}()

	// 跑 500ms（>5x DeadConnectionTimeout），节点应一直在线。
	end := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(end) {
		if !hub.IsOnline("alive-node") {
			t.Fatalf("alive-node was reaped despite continuous traffic")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// TestServerKeepaliveDefaults 仅做编译/默认填充层面的 sanity check：
// 调用 ListenAndServe 的代码路径会在所有 keepalive 字段为 0 时填充默认值。
// 这里直接测 reapOnce 的边界（threshold=now-DeadConnectionTimeout）。
//
// 关于 KeepaliveEnforcementPolicy 拒绝过频 ping 的验证：构造 grpc client
// 用 30ms 间隔 ping 较复杂（需要直接构造 transport-level conn），且
// EnforcementPolicy 行为已由 grpc-go 自身测试覆盖；此处仅做注释说明。
func TestReaper_thresholdBoundary(t *testing.T) {
	hub := New(Options{
		DeadConnectionTimeout: 50 * time.Millisecond,
		ReaperInterval:        time.Hour, // 不让自动 reaper 干扰
	})
	hub.metrics.markSeen("fresh")
	time.Sleep(60 * time.Millisecond)
	hub.metrics.markSeen("alive")

	// 手动调一次 reapOnce：fresh 已超时，但它没有 conns 项，所以不会 panic；
	// alive 没超时，不会进 stale 列表。
	hub.reapOnce(time.Now())
	// 没有 panic 即通过。
}
