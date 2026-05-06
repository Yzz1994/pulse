package nodeagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	nodev1 "pulse/internal/pb/nodev1"
)

const bufSize = 1 << 20

// mockHub 是测试用的 NodeAgentServer 实现，只关心：
//   - 接收 hello / 普通响应 / event push（usage_push 等）
//   - 主动下发 ServerMessage（包括 cancel_id、ack）
//   - 模拟服务端断开
type mockHub struct {
	nodev1.UnimplementedNodeAgentServer

	sessions chan *mockSession // 每次 Session 进来都丢进来
}

func newMockHub() *mockHub {
	return &mockHub{sessions: make(chan *mockSession, 16)}
}

type mockSession struct {
	stream nodev1.NodeAgent_SessionServer
	sendMu sync.Mutex

	hello  chan *nodev1.NodeMessage
	resps  chan *nodev1.NodeMessage // Id != "" 且 Event == ""
	pushes chan *nodev1.NodeMessage // Event != ""

	closeReq chan struct{} // 触发 Session handler 返回（server 侧主动断流）
	closed   chan struct{} // Session handler 已返回
}

func (h *mockHub) Session(stream nodev1.NodeAgent_SessionServer) error {
	s := &mockSession{
		stream:   stream,
		hello:    make(chan *nodev1.NodeMessage, 1),
		resps:    make(chan *nodev1.NodeMessage, 256),
		pushes:   make(chan *nodev1.NodeMessage, 256),
		closeReq: make(chan struct{}),
		closed:   make(chan struct{}),
	}
	h.sessions <- s
	defer close(s.closed)

	type recvResult struct {
		msg *nodev1.NodeMessage
		err error
	}
	recvCh := make(chan recvResult, 1)
	go func() {
		for {
			msg, err := stream.Recv()
			recvCh <- recvResult{msg, err}
			if err != nil {
				return
			}
		}
	}()

	for {
		select {
		case <-s.closeReq:
			return nil
		case r := <-recvCh:
			if r.err != nil {
				return r.err
			}
			msg := r.msg
			switch {
			case msg.GetEvent() == "hello":
				select {
				case s.hello <- msg:
				default:
				}
			case msg.GetEvent() != "":
				s.pushes <- msg
			case msg.GetId() != "":
				s.resps <- msg
			}
		}
	}
}

func (s *mockSession) send(msg *nodev1.ServerMessage) error {
	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	return s.stream.Send(msg)
}

// closeStream 让 server 侧主动 return，从而关闭流。
func (s *mockSession) closeStream() {
	select {
	case <-s.closeReq:
	default:
		close(s.closeReq)
	}
	<-s.closed
}

// startMockServer 在 bufconn 上启动 mock，返回 hub + dialer + server stop。
func startMockServer(t *testing.T) (*mockHub, func(context.Context, string) (net.Conn, error), func()) {
	t.Helper()
	lis := bufconn.Listen(bufSize)
	srv := grpc.NewServer()
	hub := newMockHub()
	nodev1.RegisterNodeAgentServer(srv, hub)
	go func() { _ = srv.Serve(lis) }()
	dialer := func(ctx context.Context, _ string) (net.Conn, error) {
		return lis.DialContext(ctx)
	}
	stop := func() { srv.Stop() }
	return hub, dialer, stop
}

// helloProvider 返回一个固定的 hello body。
func helloProvider() (json.RawMessage, error) {
	return json.RawMessage(`{"node_id":"test","config_hash":"deadbeef","version":"test"}`), nil
}

func dialOpts(dialer func(context.Context, string) (net.Conn, error)) []grpc.DialOption {
	return []grpc.DialOption{
		grpc.WithContextDialer(dialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	}
}

// fnDispatcher 用函数实现 Dispatcher。
type fnDispatcher func(ctx context.Context, method string, body json.RawMessage) (json.RawMessage, error)

func (f fnDispatcher) Handle(ctx context.Context, method string, body json.RawMessage) (json.RawMessage, error) {
	return f(ctx, method, body)
}

func runAgent(t *testing.T, ctx context.Context, cfg Config) <-chan error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- Run(ctx, cfg) }()
	return done
}

// 1) 成功连接：mock server 收到 hello。
func TestAgent_HelloOnConnect(t *testing.T) {
	t.Parallel()
	hub, dialer, stop := startMockServer(t)
	defer stop()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := Config{
		NodeID:        "n1",
		ServerAddr:    "passthrough:///bufnet",
		Dispatcher:    NoopDispatcher{},
		HelloProvider: helloProvider,
		GRPCDialOpts:  dialOpts(dialer),
	}
	done := runAgent(t, ctx, cfg)

	select {
	case s := <-hub.sessions:
		select {
		case h := <-s.hello:
			if got := string(h.GetBody()); got == "" {
				t.Fatalf("empty hello body")
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("did not receive hello")
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("no session opened")
	}

	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Run err = %v, want Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("Run did not return on ctx cancel")
	}
}

// 2) Dispatcher 调用 + 响应。
func TestAgent_DispatcherRoundTrip(t *testing.T) {
	t.Parallel()
	hub, dialer, stop := startMockServer(t)
	defer stop()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := Config{
		NodeID:     "n1",
		ServerAddr: "passthrough:///bufnet",
		Dispatcher: fnDispatcher(func(ctx context.Context, method string, body json.RawMessage) (json.RawMessage, error) {
			if method != "ping" {
				return nil, fmt.Errorf("unexpected method %s", method)
			}
			return json.RawMessage(`{"pong":true}`), nil
		}),
		HelloProvider: helloProvider,
		GRPCDialOpts:  dialOpts(dialer),
	}
	done := runAgent(t, ctx, cfg)
	defer func() { cancel(); <-done }()

	s := <-hub.sessions
	<-s.hello

	if err := s.send(&nodev1.ServerMessage{Id: "r1", Method: "ping", Body: []byte(`{}`)}); err != nil {
		t.Fatalf("send method: %v", err)
	}
	select {
	case resp := <-s.resps:
		if resp.GetId() != "r1" || !resp.GetOk() || string(resp.GetBody()) != `{"pong":true}` {
			t.Fatalf("bad response: %+v", resp)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("no response from agent")
	}
}

// 3) cancel_id 取消 dispatcher。
func TestAgent_CancelInflight(t *testing.T) {
	t.Parallel()
	hub, dialer, stop := startMockServer(t)
	defer stop()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	started := make(chan struct{}, 1)
	canceled := make(chan struct{}, 1)
	cfg := Config{
		NodeID:     "n1",
		ServerAddr: "passthrough:///bufnet",
		Dispatcher: fnDispatcher(func(ctx context.Context, method string, body json.RawMessage) (json.RawMessage, error) {
			started <- struct{}{}
			<-ctx.Done()
			canceled <- struct{}{}
			return nil, ctx.Err()
		}),
		HelloProvider: helloProvider,
		GRPCDialOpts:  dialOpts(dialer),
	}
	done := runAgent(t, ctx, cfg)
	defer func() { cancel(); <-done }()

	s := <-hub.sessions
	<-s.hello

	if err := s.send(&nodev1.ServerMessage{Id: "r1", Method: "stream", Body: []byte(`{}`)}); err != nil {
		t.Fatalf("send method: %v", err)
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatalf("dispatcher did not start")
	}
	if err := s.send(&nodev1.ServerMessage{CancelId: "r1"}); err != nil {
		t.Fatalf("send cancel: %v", err)
	}
	select {
	case <-canceled:
	case <-time.After(2 * time.Second):
		t.Fatalf("dispatcher ctx not canceled")
	}
	// 应收到一个错误响应。
	select {
	case resp := <-s.resps:
		if resp.GetOk() {
			t.Fatalf("expected error response, got ok")
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("no response after cancel")
	}
}

// 4) 重连：server 主动断开，agent 用短 backoff 重连。
func TestAgent_Reconnect(t *testing.T) {
	t.Parallel()
	hub, dialer, stop := startMockServer(t)
	defer stop()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := Config{
		NodeID:           "n1",
		ServerAddr:       "passthrough:///bufnet",
		Dispatcher:       NoopDispatcher{},
		HelloProvider:    helloProvider,
		GRPCDialOpts:     dialOpts(dialer),
		ReconnectBackoff: []time.Duration{10 * time.Millisecond},
	}
	done := runAgent(t, ctx, cfg)
	defer func() { cancel(); <-done }()

	// 第一次 session：收 hello 后让 server 端主动关闭流。
	s1 := <-hub.sessions
	<-s1.hello
	s1.closeStream()

	select {
	case s2 := <-hub.sessions:
		select {
		case <-s2.hello:
		case <-time.After(2 * time.Second):
			t.Fatalf("no hello on reconnect")
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("no reconnect")
	}
}

// 5) ctx 取消：Run 应及时返回。
func TestAgent_CtxCancel(t *testing.T) {
	t.Parallel()
	hub, dialer, stop := startMockServer(t)
	defer stop()

	ctx, cancel := context.WithCancel(context.Background())

	cfg := Config{
		NodeID:        "n1",
		ServerAddr:    "passthrough:///bufnet",
		Dispatcher:    NoopDispatcher{},
		HelloProvider: helloProvider,
		GRPCDialOpts:  dialOpts(dialer),
	}
	done := runAgent(t, ctx, cfg)

	s := <-hub.sessions
	<-s.hello

	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v want Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("Run did not return")
	}
}

// 6) usage push + ack。
func TestAgent_UsagePushAck(t *testing.T) {
	t.Parallel()
	hub, dialer, stop := startMockServer(t)
	defer stop()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	senderCh := make(chan Sender, 1)
	cfg := Config{
		NodeID:        "n1",
		ServerAddr:    "passthrough:///bufnet",
		Dispatcher:    NoopDispatcher{},
		HelloProvider: helloProvider,
		GRPCDialOpts:  dialOpts(dialer),
		OnConnected: func(ctx context.Context, sender Sender) {
			senderCh <- sender
		},
	}
	done := runAgent(t, ctx, cfg)
	defer func() { cancel(); <-done }()

	s := <-hub.sessions
	<-s.hello
	sender := <-senderCh

	// 启动 server-side: 收到 usage_push 后回 ack。
	go func() {
		for {
			select {
			case <-s.closed:
				return
			case p := <-s.pushes:
				if p.GetEvent() != "usage_push" {
					continue
				}
				body := []byte(fmt.Sprintf(`{"seq":%d}`, p.GetSeq()))
				_ = s.send(&nodev1.ServerMessage{Method: "ack", Body: body})
			}
		}
	}()

	if err := sender.PushEvent("", "usage_push", []byte(`{"users":[]}`), 5); err != nil {
		t.Fatalf("PushEvent: %v", err)
	}
	waitCtx, waitCancel := context.WithTimeout(ctx, 2*time.Second)
	defer waitCancel()
	if err := sender.WaitAck(waitCtx, 5); err != nil {
		t.Fatalf("WaitAck: %v", err)
	}
}

// 7) 并发 dispatch 100 个请求。
func TestAgent_ConcurrentDispatch(t *testing.T) {
	t.Parallel()
	hub, dialer, stop := startMockServer(t)
	defer stop()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var calls atomic.Int64
	cfg := Config{
		NodeID:     "n1",
		ServerAddr: "passthrough:///bufnet",
		Dispatcher: fnDispatcher(func(ctx context.Context, method string, body json.RawMessage) (json.RawMessage, error) {
			calls.Add(1)
			return body, nil
		}),
		HelloProvider: helloProvider,
		GRPCDialOpts:  dialOpts(dialer),
	}
	done := runAgent(t, ctx, cfg)
	defer func() { cancel(); <-done }()

	s := <-hub.sessions
	<-s.hello

	const N = 100
	for i := 0; i < N; i++ {
		body := []byte(fmt.Sprintf(`{"i":%d}`, i))
		if err := s.send(&nodev1.ServerMessage{
			Id:     fmt.Sprintf("r%d", i),
			Method: "echo",
			Body:   body,
		}); err != nil {
			t.Fatalf("send %d: %v", i, err)
		}
	}
	got := make(map[string]string, N)
	deadline := time.After(5 * time.Second)
	for len(got) < N {
		select {
		case resp := <-s.resps:
			if !resp.GetOk() {
				t.Fatalf("resp %s not ok: %s", resp.GetId(), resp.GetError())
			}
			got[resp.GetId()] = string(resp.GetBody())
		case <-deadline:
			t.Fatalf("only got %d/%d responses", len(got), N)
		}
	}
	if int(calls.Load()) != N {
		t.Fatalf("dispatcher calls = %d want %d", calls.Load(), N)
	}
	for i := 0; i < N; i++ {
		want := fmt.Sprintf(`{"i":%d}`, i)
		if got[fmt.Sprintf("r%d", i)] != want {
			t.Fatalf("r%d body mismatch: %q", i, got[fmt.Sprintf("r%d", i)])
		}
	}
}

// 8) DefaultHelloProvider 行为。
func TestDefaultHelloProvider(t *testing.T) {
	t.Parallel()
	hp := DefaultHelloProvider("nodeA", func() string { return "abc" })
	body, err := hp()
	if err != nil {
		t.Fatalf("hp err: %v", err)
	}
	var out struct {
		NodeID, ConfigHash, Version string `json:"-"`
	}
	out.NodeID = "x"
	if err := json.Unmarshal(body, &struct {
		NodeID     *string `json:"node_id"`
		ConfigHash *string `json:"config_hash"`
		Version    *string `json:"version"`
	}{&out.NodeID, &out.ConfigHash, &out.Version}); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.NodeID != "nodeA" || out.ConfigHash != "abc" {
		t.Fatalf("unexpected hello body: %+v", out)
	}
}

// 让 mockSession 提供主动关闭流的能力。已在上方实现（closeStream）。
var _ = io.EOF
