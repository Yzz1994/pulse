package server

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"pulse/internal/cert"
	"pulse/internal/nodehub"
	"pulse/internal/nodes"
)

// startNodeHub 实例化 nodehub.Hub，启动 reaper，并在 ctx 后台启动 gRPC 服务。
// 返回的 hub 可注入到 nodes.NewClientWithHub 用作短调用通道。
//
// addr 是 gRPC 监听地址（如 ":8082"）。serverCN/serverSANs 用于自签 server 证书
// （nodes 已通过 NodeCA 信任链验证，因此用同一 CA 签发即可）。
//
// pushHandler 当 nil 时使用 NoopPushHandler；调用方一般传入 MultiPushHandler
// 以扇出 hello/usage_push/log/traceroute_hop 事件到不同业务模块。
//
// 失败仅打印 log，不阻断 server 启动：HTTP API 仍可工作，nodes.Client 会因 hub
// 离线而走 ErrNodeOffline 路径。
func startNodeHub(ctx context.Context, addr, serverCN string, serverSANs []string, nodeCA *cert.NodeCA, nodeStore nodes.Store, pushHandler nodehub.PushHandler) *nodehub.Hub {
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
		return hub
	}

	go func() {
		err := nodehub.ListenAndServe(ctx, nodehub.ServerOptions{
			Addr:                addr,
			Hub:                 hub,
			CA:                  nodeCA,
			ServerCert:          serverTLS,
			KeepaliveTime:       30 * time.Second,
			KeepaliveTimeout:    10 * time.Second,
			PermitWithoutStream: true,
		})
		if err != nil {
			log.Printf("nodehub: gRPC server exited: %v", err)
		}
	}()

	return hub
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
		// 已有非 loopback 地址（管理员手动配置的域名/IP）则不覆盖
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
	trimmed := strings.TrimPrefix(strings.TrimPrefix(u, "https://"), "http://")
	host := strings.SplitN(trimmed, ":", 2)[0]
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}
