package nodehub

import (
"context"

"pulse/internal/nodes"
)

// 注册 hub 层的离线错误到 nodes 包，让 nodes.Client.callHub 可识别。
// 把 import 方向定为 nodehub → nodes，避免 nodes 反向依赖 nodehub。
func init() {
nodes.RegisterHubOfflineError(ErrNodeOffline)
nodes.RegisterHubStreamCaller(callStreamAdapter)
}

// callStreamAdapter 把 nodes.HubStreamFunc 适配到 *Hub.CallStream。
// hub any 必须是 *Hub 类型；其他类型将返回错误。
func callStreamAdapter(ctx context.Context, hub any, nodeID, method string, body any) (nodes.HubStream, error) {
h, ok := hub.(*Hub)
if !ok {
return nil, ErrNodeOffline
}
s, err := h.CallStream(ctx, nodeID, method, body)
if err != nil {
return nil, err
}
return &nodesStreamAdapter{s: s, frames: convertFrames(s)}, nil
}

// nodesStreamAdapter 把 *Stream 包成 nodes.HubStream（重新打包 frames 通道
// 以适配 nodes 包定义的 HubStreamFrame 类型）。
type nodesStreamAdapter struct {
s      *Stream
frames <-chan nodes.HubStreamFrame
}

func (a *nodesStreamAdapter) Frames() <-chan nodes.HubStreamFrame { return a.frames }
func (a *nodesStreamAdapter) Done() <-chan struct{}               { return a.s.Done() }
func (a *nodesStreamAdapter) Err() error                          { return a.s.Err() }
func (a *nodesStreamAdapter) Close()                              { a.s.Close() }

// convertFrames 把 *Stream.Frames() 转成 nodes.HubStreamFrame 通道。
// 当 stream.Done() 关闭时，输出通道也会被关闭。
func convertFrames(s *Stream) <-chan nodes.HubStreamFrame {
out := make(chan nodes.HubStreamFrame, streamFramesBuffer)
go func() {
defer close(out)
for {
select {
case f, ok := <-s.Frames():
if !ok {
return
}
select {
case out <- nodes.HubStreamFrame{Event: f.Event, Body: f.Body}:
case <-s.Done():
return
}
case <-s.Done():
// drain 残留帧
for {
select {
case f, ok := <-s.Frames():
if !ok {
return
}
select {
case out <- nodes.HubStreamFrame{Event: f.Event, Body: f.Body}:
default:
return
}
default:
return
}
}
}
}
}()
return out
}
