package nodehub

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	nodev1 "pulse/internal/pb/nodev1"
)

// TestCallStream_LogFrames：CallStream 发起后，node 推送多条 log 帧 + 终态 ok
// → caller 收到所有帧，Done 关闭，Err()==nil。
func TestCallStream_LogFrames(t *testing.T) {
	hub := New(Options{PeerExtractor: mdPeerExtractor})
	env := newTestEnv(t, hub)

	cc := env.dial(t)
	mock := startNodeMock(t, cc, "node-stream")
	waitOnline(t, hub, "node-stream")

	// node 收到 LogsStream 时推 3 帧 log，再发终态 Ok。
	mock.handle("LogsStream", func(reqID string, _ []byte) (bool, []byte, string) {
		// 在 mock 层 handle 必须返回响应，所以我们这里返回 ok=true 表示流终态；
		// 帧通过 sendNode 单独 push。
		for i := 0; i < 3; i++ {
			body, _ := json.Marshal(map[string]string{"line": "log-" + string(rune('A'+i))})
			mock.sendNode(&nodev1.NodeMessage{Id: reqID, Event: "log", Body: body})
		}
		return true, nil, ""
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	stream, err := hub.CallStream(ctx, "node-stream", "LogsStream", nil)
	if err != nil {
		t.Fatalf("CallStream: %v", err)
	}
	defer stream.Close()

	got := []string{}
	timeout := time.After(3 * time.Second)
loop:
	for {
		select {
		case f, ok := <-stream.Frames():
			if !ok {
				break loop
			}
			if f.Event != "log" {
				t.Fatalf("unexpected event: %q", f.Event)
			}
			var p struct {
				Line string `json:"line"`
			}
			if err := json.Unmarshal(f.Body, &p); err != nil {
				t.Fatalf("decode: %v", err)
			}
			got = append(got, p.Line)
		case <-stream.Done():
			// 排干 frames（终态前缓冲的剩余帧）
			for {
				select {
				case f, ok := <-stream.Frames():
					if !ok {
						break loop
					}
					var p struct {
						Line string `json:"line"`
					}
					if err := json.Unmarshal(f.Body, &p); err != nil {
						t.Fatalf("decode: %v", err)
					}
					got = append(got, p.Line)
				default:
					break loop
				}
			}
		case <-timeout:
			t.Fatalf("timed out, got=%v", got)
		}
	}
	if len(got) != 3 {
		t.Fatalf("want 3 frames, got %d: %v", len(got), got)
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("Err()=%v want nil", err)
	}
}

// TestCallStream_CancelSendsCancelID：caller Close()/ctx 取消时，node 端必须收到
// 同 reqID 的 cancel_id 帧。
func TestCallStream_CancelSendsCancelID(t *testing.T) {
	hub := New(Options{PeerExtractor: mdPeerExtractor})
	env := newTestEnv(t, hub)
	cc := env.dial(t)
	mock := startNodeMock(t, cc, "node-cancel")
	waitOnline(t, hub, "node-cancel")

	// 让 mock 收到任何 method 都"沉默"——不立即回响应，stream 长开。
	mock.silent.Store(true)

	ctx := context.Background()
	stream, err := hub.CallStream(ctx, "node-cancel", "LogsStream", nil)
	if err != nil {
		t.Fatalf("CallStream: %v", err)
	}

	// caller 主动 Close → 应发送 cancel_id
	stream.Close()

	select {
	case cid := <-mock.cancelIDs:
		if cid == "" {
			t.Fatal("got empty cancel_id")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("node did not receive cancel_id")
	}

	// stream.Done() 已关闭
	select {
	case <-stream.Done():
	case <-time.After(time.Second):
		t.Fatal("stream.Done() not closed after Close")
	}
}

// TestCallStream_NodeOffline：未注册 nodeID → ErrNodeOffline。
func TestCallStream_NodeOffline(t *testing.T) {
	hub := New(Options{PeerExtractor: mdPeerExtractor})
	_, err := hub.CallStream(context.Background(), "no-such-node", "LogsStream", nil)
	if !errors.Is(err, ErrNodeOffline) {
		t.Fatalf("want ErrNodeOffline, got %v", err)
	}
}
