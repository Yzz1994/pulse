package nodehub

import (
	"context"
	"encoding/json"
	"sync"

	nodev1 "pulse/internal/pb/nodev1"
)

// StreamFrame 是流式调用中收到的一帧 push（log 行 / traceroute hop 等）。
type StreamFrame struct {
	Event string
	Body  json.RawMessage
}

// Stream 由 Hub.CallStream 返回，封装一次流式调用的生命周期。
//
// 使用模式：
//
//	stream, err := hub.CallStream(ctx, nodeID, "LogsStream", nil)
//	if err != nil { return err }
//	defer stream.Close()
//	for {
//	    select {
//	    case f := <-stream.Frames():
//	        ...
//	    case <-stream.Done():
//	        return stream.Err()
//	    }
//	}
type Stream struct {
	conn  *conn
	reqID string

	frames chan StreamFrame
	done   chan struct{}

	mu        sync.Mutex
	err       error
	closed    bool
	closeOnce sync.Once
}

// Frames 返回流帧通道。Frames 通道在 Stream 终结后会被一并关闭，调用方
// 通常配合 Done() 判断终态。
func (s *Stream) Frames() <-chan StreamFrame { return s.frames }

// Done 在 Stream 终结（node 端正常结束 / node 端报错 / caller Close /
// 节点连接断开）时被关闭。
func (s *Stream) Done() <-chan struct{} { return s.done }

// Err 返回流终态错误（node 端 Ok=false 或连接断开）。Stream 仍在进行中时返回 nil。
func (s *Stream) Err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

// Close 由调用方主动取消流：会向 node 发送 cancel_id（best-effort），并立刻关闭
// 本地 Done。多次调用安全。
func (s *Stream) Close() {
	s.closeOnce.Do(func() {
		s.conn.streamSubs.Delete(s.reqID)
		// best-effort cancel 下发；忽略错误（连接可能已断）
		_ = s.conn.send(&nodev1.ServerMessage{CancelId: s.reqID})
		s.mu.Lock()
		s.closed = true
		s.mu.Unlock()
		close(s.done)
		// 安全关闭 frames：deliver 在加锁后检查 closed，不会再写。
		close(s.frames)
	})
}

// closeFromHub 由 hub 内部触发的终结（node 回了终态帧 / 连接断开），不发 cancel。
func (s *Stream) closeFromHub(endErr error) {
	s.closeOnce.Do(func() {
		s.conn.streamSubs.Delete(s.reqID)
		s.mu.Lock()
		s.err = endErr
		s.closed = true
		s.mu.Unlock()
		close(s.done)
		close(s.frames)
	})
}

// deliver 把一帧投递到 frames 通道。投递在 mu 保护下检查 closed，避免向已关闭通道写入。
// frames 缓冲不足时阻塞等待消费方，但若同时 Close 触发则立即放弃（done 兜底）。
func (s *Stream) deliver(f StreamFrame) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	// 持锁期间 Close/closeFromHub 不会进入 closeOnce 内部（互斥锁同源），
	// 但为避免阻塞 dispatch goroutine 导致死锁，发送时释放锁，转而用 done 兜底。
	frames := s.frames
	done := s.done
	s.mu.Unlock()
	select {
	case frames <- f:
	case <-done:
	}
}

// streamFramesBuffer 是 Stream.frames 的缓冲容量。日志/traceroute hop 突发可
// 容忍少量积压；超出时 deliver 会阻塞 hub 的 recv goroutine 等待消费方。
const streamFramesBuffer = 64

// CallStream 发起一次流式调用：发送 ServerMessage{Method, Body} 后，把后续 node
// 推回的同 reqID push 帧（event="log" / "traceroute_hop"）通过 Stream.Frames()
// 暴露。node 端在流自然结束时回一条普通响应（Ok=true 或 Ok=false+Error），
// 触发 Stream.Done()；caller 也可通过 Stream.Close() 主动取消（向 node 发送
// cancel_id 帧）。
//
// reqBody 用 json 编码（nil 表示空 body）。
func (h *Hub) CallStream(ctx context.Context, nodeID, method string, reqBody any) (*Stream, error) {
	h.mu.RLock()
	c, ok := h.conns[nodeID]
	h.mu.RUnlock()
	if !ok {
		return nil, ErrNodeOffline
	}

	var bodyBytes []byte
	if reqBody != nil {
		b, err := json.Marshal(reqBody)
		if err != nil {
			return nil, err
		}
		bodyBytes = b
	}

	reqID, err := newReqID()
	if err != nil {
		return nil, err
	}

	s := &Stream{
		conn:   c,
		reqID:  reqID,
		frames: make(chan StreamFrame, streamFramesBuffer),
		done:   make(chan struct{}),
	}

	c.streamSubs.Store(reqID, s)

	if err := c.send(&nodev1.ServerMessage{
		Id:     reqID,
		Method: method,
		Body:   bodyBytes,
	}); err != nil {
		c.streamSubs.Delete(reqID)
		return nil, err
	}

	// ctx 取消 → 主动 Close（best-effort 通知 node）
	go func() {
		select {
		case <-ctx.Done():
			// 仅在尚未结束时 Close
			s.mu.Lock()
			closed := s.closed
			s.mu.Unlock()
			if !closed {
				// 标记 ctx 错误，再 Close
				s.mu.Lock()
				if s.err == nil {
					s.err = ctx.Err()
				}
				s.mu.Unlock()
				s.Close()
			}
		case <-s.done:
		}
	}()

	return s, nil
}