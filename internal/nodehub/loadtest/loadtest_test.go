//go:build loadtest

// Package loadtest 包含 nodehub 的并发压测，用 build tag 隔离，默认 CI 不跑。
//
// 运行：
//
//	go test -tags loadtest -count=1 -timeout 5m ./internal/nodehub/loadtest/...
//
// 或：
//
//	make loadtest
package loadtest

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/test/bufconn"

	"pulse/internal/nodeagent"
	"pulse/internal/nodehub"
	nodev1 "pulse/internal/pb/nodev1"
)

const (
	bufSize     = 1 << 20
	numNodes    = 1000
	holdSeconds = 30
	callsPerSec = 100
)

// 用 metadata 传递 nodeID（绕过 mTLS）。
func mdPeerExtractor(ctx context.Context) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", fmt.Errorf("loadtest: no metadata")
	}
	v := md.Get("x-node-id")
	if len(v) == 0 {
		return "", fmt.Errorf("loadtest: no x-node-id")
	}
	return v[0], nil
}

// nopDispatcher 立即响应所有 method 请求。
type nopDispatcher struct{}

func (nopDispatcher) Handle(ctx context.Context, method string, body json.RawMessage) (json.RawMessage, error) {
	return []byte(`{"ok":true}`), nil
}

func TestLoad_ConcurrentNodes(t *testing.T) {
	baseGoroutines := runtime.NumGoroutine()
	t.Logf("baseline goroutines before test: %d", baseGoroutines)

	lis := bufconn.Listen(bufSize)
	hub := nodehub.New(nodehub.Options{
		PeerExtractor:         mdPeerExtractor,
		PushHandler:           nodehub.NoopPushHandler{},
		DeadConnectionTimeout: 60 * time.Second,
		ReaperInterval:        10 * time.Second,
	})
	srv := grpc.NewServer()
	nodev1.RegisterNodeAgentServer(srv, hub)
	go func() { _ = srv.Serve(lis) }()
	defer srv.Stop()

	hubCtx, hubCancel := context.WithCancel(context.Background())
	defer hubCancel()
	go hub.RunReaper(hubCtx)

	dialer := func(ctx context.Context, _ string) (net.Conn, error) {
		return lis.DialContext(ctx)
	}

	nodeIDInterceptor := func(nodeID string) grpc.DialOption {
		return grpc.WithStreamInterceptor(func(
			ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn,
			method string, streamer grpc.Streamer, opts ...grpc.CallOption,
		) (grpc.ClientStream, error) {
			ctx = metadata.AppendToOutgoingContext(ctx, "x-node-id", nodeID)
			return streamer(ctx, desc, cc, method, opts...)
		})
	}

	agentCtx, agentCancel := context.WithCancel(context.Background())
	var agentWG sync.WaitGroup

	startErrs := make(chan error, numNodes)
	for i := 0; i < numNodes; i++ {
		nodeID := fmt.Sprintf("node-%d", i)
		agentWG.Add(1)
		go func() {
			defer agentWG.Done()
			cfg := nodeagent.Config{
				NodeID:           nodeID,
				ServerAddr:       "passthrough:///bufnet",
				Dispatcher:       nopDispatcher{},
				HelloProvider:    nodeagent.DefaultHelloProvider(nodeID, nil),
				ReconnectBackoff: []time.Duration{200 * time.Millisecond},
				KeepaliveTime:    30 * time.Second,
				KeepaliveTimeout: 10 * time.Second,
				GRPCDialOpts: []grpc.DialOption{
					grpc.WithContextDialer(dialer),
					grpc.WithTransportCredentials(insecure.NewCredentials()),
					nodeIDInterceptor(nodeID),
				},
			}
			err := nodeagent.Run(agentCtx, cfg)
			if err != nil && err != context.Canceled {
				startErrs <- fmt.Errorf("%s: %w", nodeID, err)
			}
		}()
	}

	// 等所有节点上线
	deadline := time.Now().Add(60 * time.Second)
	for {
		online := len(hub.OnlineNodes())
		if online >= numNodes {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("only %d/%d nodes came online within 60s", online, numNodes)
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Logf("all %d nodes online", numNodes)

	peakGoroutines := runtime.NumGoroutine()
	t.Logf("goroutines after all online: %d", peakGoroutines)

	// 持续发 Call，验证吞吐 + 流稳定性
	var callOK, callErr atomic.Uint64
	stopCallers := make(chan struct{})
	var callerWG sync.WaitGroup
	const numCallers = 8
	perCallerRate := callsPerSec / numCallers
	for c := 0; c < numCallers; c++ {
		callerWG.Add(1)
		go func() {
			defer callerWG.Done()
			r := rand.New(rand.NewSource(time.Now().UnixNano()))
			ticker := time.NewTicker(time.Second / time.Duration(perCallerRate))
			defer ticker.Stop()
			for {
				select {
				case <-stopCallers:
					return
				case <-ticker.C:
					id := fmt.Sprintf("node-%d", r.Intn(numNodes))
					ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
					_, err := hub.Call(ctx, id, "ping", nil)
					cancel()
					if err != nil {
						callErr.Add(1)
					} else {
						callOK.Add(1)
					}
				}
			}
		}()
	}

	holdEnd := time.Now().Add(holdSeconds * time.Second)
	maxGoroutines := peakGoroutines
	for time.Now().Before(holdEnd) {
		time.Sleep(2 * time.Second)
		g := runtime.NumGoroutine()
		if g > maxGoroutines {
			maxGoroutines = g
		}
	}
	close(stopCallers)
	callerWG.Wait()

	t.Logf("Call results: ok=%d err=%d (over %ds, target rate=%d/s)",
		callOK.Load(), callErr.Load(), holdSeconds, callsPerSec)

	var memDuringHold runtime.MemStats
	runtime.ReadMemStats(&memDuringHold)
	t.Logf("HeapAlloc during hold: %.1f MB",
		float64(memDuringHold.HeapAlloc)/1024/1024)
	t.Logf("max goroutines during hold: %d", maxGoroutines)

	// 优雅关停 agents
	agentCancel()
	agentWG.Wait()
	close(startErrs)
	for e := range startErrs {
		// 只是日志：很多节点退出时会报 context.Canceled / EOF
		t.Logf("agent exit: %v", e)
	}

	// 等所有 conn 从 hub 注销
	deadline = time.Now().Add(30 * time.Second)
	for len(hub.OnlineNodes()) > 0 && time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
	}
	if remain := len(hub.OnlineNodes()); remain != 0 {
		t.Fatalf("after shutdown, %d nodes still online", remain)
	}

	// 让 GC 跑一下
	for i := 0; i < 3; i++ {
		runtime.GC()
		time.Sleep(500 * time.Millisecond)
	}

	finalGoroutines := runtime.NumGoroutine()
	var memAfter runtime.MemStats
	runtime.ReadMemStats(&memAfter)

	t.Logf("=== loadtest summary ===")
	t.Logf("nodes: %d, hold: %ds, calls ok/err: %d/%d",
		numNodes, holdSeconds, callOK.Load(), callErr.Load())
	t.Logf("goroutines: baseline=%d peak=%d after-shutdown=%d",
		baseGoroutines, maxGoroutines, finalGoroutines)
	t.Logf("HeapAlloc: peak-during-hold=%.1fMB after-gc=%.1fMB",
		float64(memDuringHold.HeapAlloc)/1024/1024,
		float64(memAfter.HeapAlloc)/1024/1024)

	// 软断言阈值：
	// - 关停后 NumGoroutine 应回到接近基线（核心泄漏指标），余量 200。
	// - 内存峰值阈值非常宽松：bufconn 同进程 1000 grpc client 各自占
	//   per-stream 读写缓冲，2GB 量级是 grpc-go 的固有开销，并非泄漏。
	//   真实部署中 1000 个 node 分布在不同机器，单 server 占用与连接数线性
	//   但远小于此（无 client 侧开销）。这里仅做"未爆炸"的上限保护。
	if finalGoroutines > 200 {
		t.Errorf("after shutdown, NumGoroutine=%d > 200 (leak suspected)", finalGoroutines)
	}
	if memDuringHold.HeapAlloc > 4*1024*1024*1024 {
		t.Errorf("HeapAlloc during hold = %d bytes > 4GB (sanity ceiling)",
			memDuringHold.HeapAlloc)
	}
}
