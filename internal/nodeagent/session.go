package nodeagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	nodev1 "pulse/internal/pb/nodev1"
)

// runSession 建立一次连接，发 hello 帧，进入收发循环。任何错误（包括正常 EOF）
// 都返回 error，由上层 Run 触发重连。
func runSession(ctx context.Context, cfg Config, creds credentials.TransportCredentials) error {
	dialOpts := make([]grpc.DialOption, 0, 4)
	if len(cfg.GRPCDialOpts) > 0 {
		dialOpts = append(dialOpts, cfg.GRPCDialOpts...)
	} else {
		if creds == nil {
			return errors.New("nodeagent: nil credentials")
		}
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(creds))
	}
	dialOpts = append(dialOpts, keepaliveParams(cfg))

	conn, err := grpc.NewClient(cfg.ServerAddr, dialOpts...)
	if err != nil {
		return fmt.Errorf("grpc.NewClient: %w", err)
	}
	defer conn.Close()

	// 整个 session 范围 ctx，stream 出错或 ctx 取消时统一收尾。
	sessionCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	client := nodev1.NewNodeAgentClient(conn)
	stream, err := client.Session(sessionCtx)
	if err != nil {
		return fmt.Errorf("open session stream: %w", err)
	}

	s := newSession(sessionCtx, cancel, cfg, stream)

	// 1) hello 帧
	helloBody, err := cfg.HelloProvider()
	if err != nil {
		return fmt.Errorf("HelloProvider: %w", err)
	}
	if err := s.sendRaw(&nodev1.NodeMessage{Event: "hello", Body: helloBody}); err != nil {
		return fmt.Errorf("send hello: %w", err)
	}

	if cfg.OnConnected != nil {
		cfg.OnConnected(sessionCtx, s)
	}

	// Dispatcher 选配 SetSender(Sender)：用于流式 method 内部主动 push 帧。
	if setter, ok := cfg.Dispatcher.(interface{ SetSender(Sender) }); ok {
		setter.SetSender(s)
	}

	// 2) recv 循环（在当前 goroutine 跑），dispatch 派发到独立 goroutine。
	recvErr := s.recvLoop()
	s.shutdown()
	if recvErr != nil && !errors.Is(recvErr, io.EOF) && !errors.Is(recvErr, context.Canceled) {
		return recvErr
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if recvErr == nil {
		// 不应发生（recvLoop 只在 Recv 出错或 ctx 取消时返回 nil 是 EOF 情形）
		return errors.New("session ended")
	}
	return recvErr
}

// session 是单次 Session 的状态：
//   - inflight：reqID -> cancelFunc，用于响应 cancel_id；
//   - acks：seq -> chan struct{}，用于 WaitAck；
//   - sendMu：保护 stream.Send，多 goroutine 串行写。
type session struct {
	ctx    context.Context
	cancel context.CancelFunc
	cfg    Config
	stream nodev1.NodeAgent_SessionClient

	sendMu sync.Mutex

	mu       sync.Mutex
	inflight map[string]context.CancelFunc
	acks     map[uint64]chan struct{}

	wg sync.WaitGroup
}

func newSession(ctx context.Context, cancel context.CancelFunc, cfg Config, stream nodev1.NodeAgent_SessionClient) *session {
	return &session{
		ctx:      ctx,
		cancel:   cancel,
		cfg:      cfg,
		stream:   stream,
		inflight: make(map[string]context.CancelFunc),
		acks:     make(map[uint64]chan struct{}),
	}
}

func (s *session) sendRaw(msg *nodev1.NodeMessage) error {
	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	return s.stream.Send(msg)
}

// PushEvent 实现 Sender。
func (s *session) PushEvent(reqID, event string, body []byte, seq uint64) error {
	if event == "" {
		return errors.New("nodeagent: PushEvent event is empty")
	}
	// 对 usage_push，预先注册 ack 等待槽（即使调用方不 WaitAck 也无副作用）。
	if event == "usage_push" && seq != 0 {
		s.mu.Lock()
		if _, ok := s.acks[seq]; !ok {
			s.acks[seq] = make(chan struct{})
		}
		s.mu.Unlock()
	}
	return s.sendRaw(&nodev1.NodeMessage{
		Id:    reqID,
		Event: event,
		Body:  body,
		Seq:   seq,
	})
}

// WaitAck 实现 Sender。
func (s *session) WaitAck(ctx context.Context, seq uint64) error {
	s.mu.Lock()
	ch, ok := s.acks[seq]
	if !ok {
		ch = make(chan struct{})
		s.acks[seq] = ch
	}
	s.mu.Unlock()

	select {
	case <-ch:
		s.mu.Lock()
		delete(s.acks, seq)
		s.mu.Unlock()
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-s.ctx.Done():
		return s.ctx.Err()
	}
}

func (s *session) recvLoop() error {
	for {
		msg, err := s.stream.Recv()
		if err != nil {
			return err
		}
		s.dispatch(msg)
	}
}

func (s *session) dispatch(msg *nodev1.ServerMessage) {
	// 1) cancel 帧：取消 inflight。
	if cid := msg.GetCancelId(); cid != "" {
		s.mu.Lock()
		cancel, ok := s.inflight[cid]
		s.mu.Unlock()
		if ok {
			cancel()
		}
		return
	}

	method := msg.GetMethod()

	// 2) ack 帧（method=="ack"）：唤醒等待。
	if method == "ack" {
		s.handleAck(msg.GetBody())
		return
	}

	if method == "" {
		// 既无 method 也无 cancel_id，忽略。
		return
	}

	// 3) 普通 method 调用：开 goroutine 处理，注册 cancel。
	reqID := msg.GetId()
	reqCtx, reqCancel := context.WithCancel(s.ctx)
	if reqID != "" {
		reqCtx = context.WithValue(reqCtx, reqIDCtxKey{}, reqID)
		s.mu.Lock()
		s.inflight[reqID] = reqCancel
		s.mu.Unlock()
	}

	body := msg.GetBody()
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer reqCancel()
		defer func() {
			if reqID != "" {
				s.mu.Lock()
				delete(s.inflight, reqID)
				s.mu.Unlock()
			}
		}()

		respBody, err := s.cfg.Dispatcher.Handle(reqCtx, method, body)
		if reqID == "" {
			// 一次性、无需响应的调用。
			return
		}
		resp := &nodev1.NodeMessage{Id: reqID}
		if err != nil {
			resp.Ok = false
			resp.Error = err.Error()
		} else {
			resp.Ok = true
			resp.Body = respBody
		}
		if sendErr := s.sendRaw(resp); sendErr != nil {
			s.cfg.Logger.Warn("nodeagent: send response failed",
				"req_id", reqID, "method", method, "err", sendErr)
		}
	}()
}

func (s *session) handleAck(body []byte) {
	var payload struct {
		Seq uint64 `json:"seq"`
	}
	if len(body) == 0 {
		return
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		s.cfg.Logger.Warn("nodeagent: bad ack body", "err", err)
		return
	}
	s.mu.Lock()
	ch, ok := s.acks[payload.Seq]
	s.mu.Unlock()
	if !ok {
		return
	}
	// 用一次性关闭通知所有 WaitAck（理论只一个）。
	select {
	case <-ch:
		// already closed
	default:
		close(ch)
	}
}

// shutdown 取消所有 inflight，并等待 dispatch goroutine 退出。
func (s *session) shutdown() {
	s.cancel()
	s.mu.Lock()
	for id, c := range s.inflight {
		c()
		delete(s.inflight, id)
	}
	// 关闭所有未触发的 ack 等待，让 WaitAck 通过 ctx.Done 返回。
	s.mu.Unlock()
	s.wg.Wait()
}

// senderFromCtx 等接口未来由 Dispatcher 使用：当前 Sender 对外暴露通过
// 在 dispatch 包后续 todo 中显式注入；这里保留 interface 静态绑定。
var _ Sender = (*session)(nil)

// reqIDCtxKey 是把 server 下发的 reqID 通过 ctx 传给 Dispatcher.Handle 的 key。
// 流式 method 实现需要拿到这个 reqID 才能用 Sender.PushEvent 推回帧。
type reqIDCtxKey struct{}

// ReqIDFromContext 从 dispatch ctx 中取出当前请求的 reqID（若无返回空串）。
func ReqIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	v, _ := ctx.Value(reqIDCtxKey{}).(string)
	return v
}
