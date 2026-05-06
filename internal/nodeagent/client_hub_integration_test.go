package nodeagent

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"pulse/internal/nodes"
)

// TestIntegration_NodesClient_HubPath 验证：
//
//	server.Side: nodes.NewClientWithHub(...).Status(ctx)
//	  → hub.Call("n1", "Status", nil)
//	  → bufconn → node-side nodeagent.Run → APIDispatcher.Handle("Status", ...)
//	  → nodeapi.API.DoStatus()
//
// 端到端走通 client-short todo 的核心切换路径。
func TestIntegration_NodesClient_HubPath(t *testing.T) {
	t.Parallel()
	ph := &integrationPushHandler{}
	hub, dialer, stop := startHub(t, ph)
	defer stop()

	api := newTestAPI()
	disp := NewAPIDispatcher(api, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := Config{
		NodeID:        "n1",
		ServerAddr:    "passthrough:///bufnet",
		Dispatcher:    disp,
		HelloProvider: DefaultHelloProvider("n1", nil),
		GRPCDialOpts: []grpc.DialOption{
			grpc.WithContextDialer(dialer),
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		},
		ReconnectBackoff: []time.Duration{20 * time.Millisecond},
	}
	done := make(chan error, 1)
	go func() { done <- Run(ctx, cfg) }()
	defer func() { cancel(); <-done }()

	// 等 node 上线
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && !hub.IsOnline("n1") {
		time.Sleep(10 * time.Millisecond)
	}
	if !hub.IsOnline("n1") {
		t.Fatal("node n1 never came online")
	}

	// 通过 nodes.Client 调用（走 hub 分支）
	client := nodes.NewClientWithHub("n1", hub)
	callCtx, cc := context.WithTimeout(ctx, 2*time.Second)
	defer cc()
	st, err := client.Status(callCtx)
	if err != nil {
		t.Fatalf("client.Status: %v", err)
	}
	// xray 没启动 → Running=false
	if st.Running {
		t.Fatalf("unexpected running=true: %+v", st)
	}

	// 离线 nodeID → ErrNodeOffline
	offClient := nodes.NewClientWithHub("missing-node", hub)
	if _, err := offClient.Status(callCtx); err == nil {
		t.Fatal("expected error for offline node")
	} else if err != nodes.ErrNodeOffline {
		// errors.Is 兼容性也要满足
		if err.Error() != nodes.ErrNodeOffline.Error() {
			t.Fatalf("expected ErrNodeOffline, got %v", err)
		}
	}
}
