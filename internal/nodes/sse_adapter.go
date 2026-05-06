package nodes

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// sseAdapter 把 hub 流式调用返回的 HubStreamFrame 序列转成 SSE 格式（text/event-stream）
// 字节流，对外实现 io.ReadCloser，让 nodes.Client.LogsStream / TracerouteStream 在
// hub 模式下保持与 HTTP 模式一致的返回类型。
//
// 编码约定（与 nodeapi 的 SSE handler 对齐）：
//   - "log" 帧：body 形如 {"line":"..."} → 写出 `data: <line>\n\n`
//   - "traceroute_hop" 帧：body 已经是 hop json → 写出 `data: <body>\n\n`
//   - 其他/未知 event：兜底写出 `data: <body>\n\n`
//   - 流终态：HubStream.Done() 关闭后，若 Err() 非 nil 写出 `event: error\ndata: {"error":"..."}\n\n`
//     否则写出 `event: done\ndata: {}\n\n`，最后再返回 io.EOF。
type sseAdapter struct {
	stream       HubStream
	buf          bytes.Buffer
	closed       bool
	doneRendered bool
}

func newSSEAdapter(s HubStream) *sseAdapter {
	return &sseAdapter{stream: s}
}

func (a *sseAdapter) Read(p []byte) (int, error) {
	if a.closed {
		return 0, io.ErrClosedPipe
	}
	for a.buf.Len() == 0 {
		if !a.fetchOne() {
			return 0, io.EOF
		}
	}
	return a.buf.Read(p)
}

// fetchOne 拉取一帧到 buf；返回 false 表示流已结束（且终态已渲染过）。
func (a *sseAdapter) fetchOne() bool {
	select {
	case f, ok := <-a.stream.Frames():
		if !ok {
			return a.renderTerminal()
		}
		a.renderFrame(f)
		return true
	case <-a.stream.Done():
		// Done 关闭后，frames 也已关闭；先 drain 残留帧再渲染终态。
		select {
		case f, ok := <-a.stream.Frames():
			if ok {
				a.renderFrame(f)
				return true
			}
		default:
		}
		return a.renderTerminal()
	}
}

func (a *sseAdapter) renderFrame(f HubStreamFrame) {
	switch f.Event {
	case "log":
		var payload struct {
			Line string `json:"line"`
		}
		if err := json.Unmarshal(f.Body, &payload); err == nil {
			fmt.Fprintf(&a.buf, "data: %s\n\n", payload.Line)
			return
		}
		fmt.Fprintf(&a.buf, "data: %s\n\n", f.Body)
	default:
		fmt.Fprintf(&a.buf, "data: %s\n\n", f.Body)
	}
}

func (a *sseAdapter) renderTerminal() bool {
	if a.doneRendered {
		return false
	}
	a.doneRendered = true
	if err := a.stream.Err(); err != nil && !errors.Is(err, io.EOF) {
		fmt.Fprintf(&a.buf, "event: error\ndata: {\"error\":%q}\n\n", err.Error())
		return true
	}
	fmt.Fprintf(&a.buf, "event: done\ndata: {}\n\n")
	return true
}

func (a *sseAdapter) Close() error {
	if a.closed {
		return nil
	}
	a.closed = true
	a.stream.Close()
	return nil
}
