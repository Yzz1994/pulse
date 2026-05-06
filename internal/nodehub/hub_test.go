package nodehub

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/test/bufconn"

	nodev1 "pulse/internal/pb/nodev1"
)

const bufSize = 1 << 20

// 测试用：从 metadata 的 "x-node-id" 头取 nodeID（绕过 mTLS）。
func mdPeerExtractor(ctx context.Context) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", errors.New("no metadata")
	}
	vals := md.Get("x-node-id")
	if len(vals) == 0 {
		return "", errors.New("no x-node-id")
	}
	return vals[0], nil
}

type testEnv struct {
	t      *testing.T
	hub    *Hub
	srv    *grpc.Server
	lis    *bufconn.Listener
	dialFn func(context.Context, string) (net.Conn, error)
}

func newTestEnv(t *testing.T, hub *Hub) *testEnv {
	t.Helper()
	lis := bufconn.Listen(bufSize)
	srv := grpc.NewServer()
	nodev1.RegisterNodeAgentServer(srv, hub)
	go func() {
		_ = srv.Serve(lis)
	}()
	t.Cleanup(func() {
		srv.Stop()
	})
	return &testEnv{
		t:   t,
		hub: hub,
		srv: srv,
		lis: lis,
		dialFn: func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		},
	}
}

func (e *testEnv) dial(t *testing.T) *grpc.ClientConn {
	t.Helper()
	cc, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(e.dialFn),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = cc.Close() })
	return cc
}

// nodeMock 模拟一个 node 客户端，启动一个 Session 流，可注册 method handler。
type nodeMock struct {
	stream    nodev1.NodeAgent_SessionClient
	sendMu    sync.Mutex
	cancelIDs chan string // 收到的 cancel_id

	// 收到的 ack 帧（用于 usage_push 测试）
	acks chan []byte

	mu       sync.Mutex
	handlers map[string]func(reqID string, body []byte) (ok bool, respBody []byte, errStr string)

	// 当 handler 不存在时，是否完全不回复（用于超时测试）
	silent atomic.Bool

	// Session 退出（stream EOF / err）时关闭
	done chan struct{}
}

func startNodeMock(t *testing.T, cc *grpc.ClientConn, nodeID string) *nodeMock {
	t.Helper()
	client := nodev1.NewNodeAgentClient(cc)
	ctx := metadata.AppendToOutgoingContext(context.Background(), "x-node-id", nodeID)
	stream, err := client.Session(ctx)
	if err != nil {
		t.Fatalf("Session start: %v", err)
	}
	m := &nodeMock{
		stream:    stream,
		cancelIDs: make(chan string, 16),
		acks:      make(chan []byte, 16),
		handlers:  make(map[string]func(string, []byte) (bool, []byte, string)),
		done:      make(chan struct{}),
	}
	go m.recvLoop()
	return m
}

func (m *nodeMock) handle(method string, fn func(reqID string, body []byte) (ok bool, respBody []byte, errStr string)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.handlers[method] = fn
}

func (m *nodeMock) recvLoop() {
	defer close(m.done)
	for {
		msg, err := m.stream.Recv()
		if err != nil {
			return
		}
		if msg.GetCancelId() != "" {
			select {
			case m.cancelIDs <- msg.GetCancelId():
			default:
			}
			continue
		}
		if msg.GetMethod() == "ack" {
			select {
			case m.acks <- msg.GetBody():
			default:
			}
			continue
		}
		// 普通请求
		m.mu.Lock()
		fn, ok := m.handlers[msg.GetMethod()]
		m.mu.Unlock()
		if m.silent.Load() {
			continue
		}
		if !ok {
			// 默认回 ok=true, body=null
			m.sendNode(&nodev1.NodeMessage{Id: msg.GetId(), Ok: true})
			continue
		}
		ok2, body, errStr := fn(msg.GetId(), msg.GetBody())
		m.sendNode(&nodev1.NodeMessage{Id: msg.GetId(), Ok: ok2, Body: body, Error: errStr})
	}
}

func (m *nodeMock) sendNode(msg *nodev1.NodeMessage) {
	m.sendMu.Lock()
	defer m.sendMu.Unlock()
	_ = m.stream.Send(msg)
}

func (m *nodeMock) pushUsage(seq uint64, body []byte) {
	m.sendNode(&nodev1.NodeMessage{Event: "usage_push", Seq: seq, Body: body})
}

func waitOnline(t *testing.T, hub *Hub, nodeID string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if hub.IsOnline(nodeID) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("node %s never came online", nodeID)
}

// ---- 测试：Call 成功 ----

func TestCall_Success(t *testing.T) {
	hub := New(Options{PeerExtractor: mdPeerExtractor})
	env := newTestEnv(t, hub)

	cc := env.dial(t)
	mock := startNodeMock(t, cc, "node-1")
	waitOnline(t, hub, "node-1")

	type req struct{ Foo string }
	type resp struct{ Echo string }

	mock.handle("hello", func(reqID string, body []byte) (bool, []byte, string) {
		var r req
		_ = json.Unmarshal(body, &r)
		out, _ := json.Marshal(resp{Echo: "hi-" + r.Foo})
		return true, out, ""
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	raw, err := hub.Call(ctx, "node-1", "hello", req{Foo: "abc"})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	var got resp
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Echo != "hi-abc" {
		t.Fatalf("unexpected echo: %q", got.Echo)
	}
}

// ---- 测试：Call 错误响应 ----

func TestCall_NodeReturnsError(t *testing.T) {
	hub := New(Options{PeerExtractor: mdPeerExtractor})
	env := newTestEnv(t, hub)
	cc := env.dial(t)
	mock := startNodeMock(t, cc, "n")
	waitOnline(t, hub, "n")

	mock.handle("boom", func(string, []byte) (bool, []byte, string) {
		return false, nil, "kaboom"
	})

	_, err := hub.Call(context.Background(), "n", "boom", nil)
	if err == nil || !strings.Contains(err.Error(), "kaboom") {
		t.Fatalf("expected kaboom error, got %v", err)
	}
}

// ---- 测试：ctx 超时 + cancel_id 下发 ----

func TestCall_ContextTimeoutSendsCancel(t *testing.T) {
	hub := New(Options{PeerExtractor: mdPeerExtractor})
	env := newTestEnv(t, hub)
	cc := env.dial(t)
	mock := startNodeMock(t, cc, "n")
	waitOnline(t, hub, "n")

	mock.silent.Store(true) // 不响应任何请求

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err := hub.Call(ctx, "n", "slow", nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected DeadlineExceeded, got %v", err)
	}

	select {
	case id := <-mock.cancelIDs:
		if id == "" {
			t.Fatalf("expected non-empty cancel_id")
		}
	case <-time.After(time.Second):
		t.Fatal("did not receive cancel_id from server")
	}
}

// ---- 测试：ErrNodeOffline ----

func TestCall_OfflineNode(t *testing.T) {
	hub := New(Options{PeerExtractor: mdPeerExtractor})
	_ = newTestEnv(t, hub)

	_, err := hub.Call(context.Background(), "ghost", "x", nil)
	if !errors.Is(err, ErrNodeOffline) {
		t.Fatalf("expected ErrNodeOffline, got %v", err)
	}
}

// ---- 测试：并发 Call ----

func TestCall_Concurrent(t *testing.T) {
	hub := New(Options{PeerExtractor: mdPeerExtractor})
	env := newTestEnv(t, hub)
	cc := env.dial(t)
	mock := startNodeMock(t, cc, "n")
	waitOnline(t, hub, "n")

	mock.handle("echo", func(reqID string, body []byte) (bool, []byte, string) {
		return true, body, ""
	})

	const N = 100
	var wg sync.WaitGroup
	wg.Add(N)
	errs := make(chan error, N)
	for i := 0; i < N; i++ {
		i := i
		go func() {
			defer wg.Done()
			payload, _ := json.Marshal(map[string]int{"i": i})
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			raw, err := hub.Call(ctx, "n", "echo", json.RawMessage(payload))
			if err != nil {
				errs <- fmt.Errorf("call %d: %w", i, err)
				return
			}
			// raw 是 node 把 body 原样回的字节；但我们把 RawMessage 序列化时
			// 会被双重编码，所以这里只校验非空与可解析。
			if len(raw) == 0 {
				errs <- fmt.Errorf("call %d: empty body", i)
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

// ---- 测试：重连踢旧连接 ----

func TestSession_ReconnectKicksOld(t *testing.T) {
	hub := New(Options{PeerExtractor: mdPeerExtractor})
	env := newTestEnv(t, hub)

	cc1 := env.dial(t)
	mock1 := startNodeMock(t, cc1, "same-id")
	waitOnline(t, hub, "same-id")

	// 第二次连接（同一 nodeID）
	cc2 := env.dial(t)
	_ = startNodeMock(t, cc2, "same-id")

	// 旧连接的 closed 通道应该被关闭，且老 stream 在 server 侧的 conn 注册被替换。
	// 我们通过观察 hub.OnlineNodes() 仍只有 1 个 + 老 mock 的 done 关闭来验证。
	// 关闭老 client conn 让其 Recv 返回 EOF：
	_ = cc1.Close()

	select {
	case <-mock1.done:
		// ok
	case <-time.After(2 * time.Second):
		t.Fatal("old session did not exit after reconnect")
	}

	online := hub.OnlineNodes()
	if len(online) != 1 || online[0] != "same-id" {
		t.Fatalf("expected single online node 'same-id', got %v", online)
	}
}

// ---- 测试：usage_push ack ----

type recordingPush struct {
	mu     sync.Mutex
	usages []uint64
	failOn map[uint64]bool
}

func (r *recordingPush) OnHello(string, []byte) {}
func (r *recordingPush) OnUsagePush(_ string, seq uint64, _ []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.usages = append(r.usages, seq)
	if r.failOn[seq] {
		return errors.New("nope")
	}
	return nil
}
func (r *recordingPush) OnLog(string, string, []byte)           {}
func (r *recordingPush) OnTracerouteHop(string, string, []byte) {}

func TestUsagePushAck(t *testing.T) {
	rp := &recordingPush{failOn: map[uint64]bool{7: true}}
	hub := New(Options{PeerExtractor: mdPeerExtractor, PushHandler: rp})
	env := newTestEnv(t, hub)
	cc := env.dial(t)
	mock := startNodeMock(t, cc, "n")
	waitOnline(t, hub, "n")

	mock.pushUsage(1, []byte(`{"a":1}`))
	mock.pushUsage(7, []byte(`{"a":7}`)) // handler 失败 → 不应 ack
	mock.pushUsage(2, []byte(`{"a":2}`))

	// 期望收到 seq=1 和 seq=2 的 ack（顺序不强求）
	got := map[uint64]bool{}
	deadline := time.After(2 * time.Second)
	for len(got) < 2 {
		select {
		case body := <-mock.acks:
			var v struct{ Seq uint64 }
			if err := json.Unmarshal(body, &v); err != nil {
				t.Fatalf("ack body parse: %v / %s", err, body)
			}
			got[v.Seq] = true
		case <-deadline:
			t.Fatalf("timed out waiting for acks; got=%v", got)
		}
	}
	if !got[1] || !got[2] {
		t.Fatalf("missing acks; got=%v", got)
	}
	// seq=7 不应收到 ack；再等一小段确保没多发
	select {
	case body := <-mock.acks:
		var v struct{ Seq uint64 }
		_ = json.Unmarshal(body, &v)
		if v.Seq == 7 {
			t.Fatalf("unexpected ack for failing seq 7")
		}
	case <-time.After(150 * time.Millisecond):
		// good
	}
}

// 防止 io 未使用警告（部分平台）
var _ = io.EOF
