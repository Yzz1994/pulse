package nodehub

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	nodev1 "pulse/internal/pb/nodev1"
)

// ErrNodeOffline 表示目标 nodeID 当前没有活跃连接。
var ErrNodeOffline = errors.New("nodehub: node offline")

// Call 向指定 node 发起一次请求-响应调用。
//
// reqBody 会用 encoding/json 编码（nil 表示空 body）。
// 返回值是 node 的 NodeMessage.Body（json 原文）。
//
// 当 ctx 取消时，hub 会 best-effort 向 node 发送 cancel_id 帧，
// 然后返回 ctx.Err()。
func (h *Hub) Call(ctx context.Context, nodeID, method string, reqBody any) (json.RawMessage, error) {
	h.metrics.callsTotal.Add(1)
	start := time.Now()
	raw, err := h.doCall(ctx, nodeID, method, reqBody)
	h.metrics.recordCallLatency(time.Since(start))
	if err != nil {
		h.metrics.callsErrTotal.Add(1)
		if errors.Is(err, ErrNodeOffline) {
			h.metrics.callsOfflineTotal.Add(1)
		}
	}
	return raw, err
}

func (h *Hub) doCall(ctx context.Context, nodeID, method string, reqBody any) (json.RawMessage, error) {
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

	ch := make(chan *nodev1.NodeMessage, 1)
	c.pending.Store(reqID, ch)
	defer c.pending.Delete(reqID)

	if err := c.send(&nodev1.ServerMessage{
		Id:     reqID,
		Method: method,
		Body:   bodyBytes,
	}); err != nil {
		return nil, err
	}

	select {
	case msg := <-ch:
		if !msg.GetOk() {
			errStr := msg.GetError()
			if errStr == "" {
				errStr = "node returned not-ok with empty error"
			}
			return nil, errors.New(errStr)
		}
		return json.RawMessage(msg.GetBody()), nil

	case <-ctx.Done():
		// best-effort 取消下发
		_ = c.send(&nodev1.ServerMessage{CancelId: reqID})
		return nil, ctx.Err()

	case <-c.closed:
		return nil, ErrNodeOffline
	}
}

func newReqID() (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf[:]), nil
}
