package server

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"time"

	"google.golang.org/grpc"

	"pulse/internal/cert"
	"pulse/internal/nodehub"
	"pulse/internal/nodes"
)

// nodeHubResult 包含 startNodeHub 的启动结果。
type nodeHubResult struct {
	Hub        *nodehub.Hub
	GRPCServer *grpc.Server    // nil 表示 gRPC 未启用（证书颁发失败等）
	TLSCert    tls.Certificate // 服务器 TLS 证书，供外层单端口 TLS 使用
}

// startNodeHub 实例化 nodehub.Hub 并构造 gRPC server（单端口 cmux 模式）。
//
// TLS 监听和 cmux 分流由调用方负责；gRPC server 通过 Serve(grpcSubListener)
// 启动，keepalive 参数完全有效。
//
// serverCN/serverSANs 用于自签服务器证书（节点 CA 信任链，节点无需额外配置）。
// pushHandler 为 nil 时使用 NoopPushHandler。
//
// 失败仅打印 log：GRPCServer 为 nil 时不启用 gRPC 功能。
func startNodeHub(ctx context.Context, serverCN string, serverSANs []string, nodeCA *cert.NodeCA, nodeStore nodes.Store, pushHandler nodehub.PushHandler) *nodeHubResult {
	if pushHandler == nil {
		pushHandler = nodehub.NoopPushHandler{}
	}
	hub := nodehub.New(nodehub.Options{
		PushHandler:           pushHandler,
		DeadConnectionTimeout: 60 * time.Second,
		ReaperInterval:        10 * time.Second,
		OnNodeConnected:       onNodeConnected(nodeStore),
	})

	serverTLS, err := nodeCA.IssueServerCert(serverCN, serverSANs, 365*24*time.Hour)
	if err != nil {
		log.Printf("nodehub: issue server cert failed: %v; gRPC disabled", err)
		return &nodeHubResult{Hub: hub}
	}

	grpcSrv, err := nodehub.NewGRPCServer(ctx, nodehub.ServerOptions{
		Hub:                 hub,
		KeepaliveTime:       30 * time.Second,
		KeepaliveTimeout:    10 * time.Second,
		PermitWithoutStream: true,
	})
	if err != nil {
		log.Printf("nodehub: create gRPC server failed: %v; gRPC disabled", err)
		return &nodeHubResult{Hub: hub}
	}

	return &nodeHubResult{Hub: hub, GRPCServer: grpcSrv, TLSCert: serverTLS}
}

// onNodeConnected 返回节点建连回调：用 gRPC 对端 IP 更新 node.BaseURL。
// 仅更新 loopback 或空 BaseURL，保留管理员手动配置的域名。
func onNodeConnected(store nodes.Store) func(nodeID, peerIP string) {
	return func(nodeID, peerIP string) {
		if peerIP == "" {
			return
		}
		node, err := store.Get(nodeID)
		if err != nil {
			return
		}
		existing := node.BaseURL
		if existing != "" && !isLoopbackURL(existing) {
			return
		}
		node.BaseURL = fmt.Sprintf("http://%s", peerIP)
		if _, err := store.Upsert(node); err != nil {
			log.Printf("nodehub: update node %s base_url: %v", nodeID, err)
		}
	}
}

func isLoopbackURL(u string) bool {
	// 支持 http://host:port、http://host、https://[::1]:port 等格式
	for _, scheme := range []string{"https://", "http://"} {
		u = trimPrefixFold(u, scheme)
	}
	host, _, err := net.SplitHostPort(u)
	if err != nil {
		// 没有端口号，整个字符串是 host
		host = u
	}
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

func trimPrefixFold(s, prefix string) string {
	if len(s) >= len(prefix) && s[:len(prefix)] == prefix {
		return s[len(prefix):]
	}
	return s
}

