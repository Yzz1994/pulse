// Package node 是 pulse-node 进程入口。Run() 加载配置、初始化 xray manager
// 与 sniproxy manager，构造 nodeagent.APIDispatcher，然后阻塞运行 gRPC 客户端：
// 节点主动连接控制面 nodehub，处理下发的 method 调用并主动推送 usage / log /
// traceroute_hop 等事件。节点不再监听任何 HTTP 端口。
package node

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"pulse/internal/config"
	"pulse/internal/ipsentinel"
	"pulse/internal/nodeagent"
	"pulse/internal/nodeapi"
	"pulse/internal/sniproxy"
	"pulse/internal/xray"
)

// xrayConfigPath 根据配置计算 xray 快照文件路径。
func xrayConfigPath(cfg config.Config) string {
	return filepath.Join(stateDir(cfg), "xray_last.json")
}

// sniproxyStatePath 根据配置计算 sniproxy 持久化文件路径。
func sniproxyStatePath(cfg config.Config) string {
	return filepath.Join(stateDir(cfg), "sniproxy_state.json")
}

// stateDir 返回节点持久化目录：优先使用 enroll 写入的 cert 所在目录，否则
// 退回到 DataDir，再退回到当前目录。
func stateDir(cfg config.Config) string {
	if cfg.NodeClientKey != "" {
		return filepath.Dir(cfg.NodeClientKey)
	}
	if cfg.DataDir != "" {
		return cfg.DataDir
	}
	return "."
}

func Run() error {
	cfg := config.Load()

	if cfg.NodeID == "" {
		return errors.New("PULSE_NODE_ID is required (run `pulse-node enroll ...` first)")
	}
	serverAddr := cfg.NodeServerAddr
	if serverAddr == "" {
		return errors.New("PULSE_NODE_SERVER_ADDR is required (host:port of control-plane gRPC)")
	}

	if err := os.MkdirAll(stateDir(cfg), 0o700); err != nil {
		return err
	}

	xrManager := xray.NewManager(xrayConfigPath(cfg))

	xrInfo := xrManager.RuntimeInfo(context.Background())
	if xrInfo.Available {
		log.Printf("pulse-node xray: %s %s", xrInfo.Module, xrInfo.Version)
	}

	sniMgr := sniproxy.NewManager(sniproxyStatePath(cfg))
	if err := sniMgr.Restore(); err != nil {
		log.Printf("pulse-node sniproxy restore: %v", err)
	} else if restored := sniMgr.Config(); restored.Listen != "" {
		log.Printf("pulse-node sniproxy restored: listen=%s routes=%d", restored.Listen, len(restored.Routes))
	}

	api := nodeapi.New(xrManager).WithSNIManager(sniMgr)

	ipsentinelHandler := ipsentinel.NewNodeHandler(cfg.DataDir)

	dispatcher := nodeagent.NewAPIDispatcher(api, ipsentinelHandler)

	if saved := xrManager.SavedConfig(); saved != "" {
		log.Printf("pulse-node restoring xray from saved config")
		if err := xrManager.Start(saved); err != nil {
			log.Printf("pulse-node restore xray failed: %v", err)
		} else {
			log.Printf("pulse-node xray auto-started")
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	usagePusher := nodeagent.NewUsagePusher(api, 30*time.Second)
	var pusherStart sync.Once

	agentCfg := nodeagent.Config{
		NodeID:        cfg.NodeID,
		ServerAddr:    serverAddr,
		CertFile:      cfg.NodeClientCert,
		KeyFile:       cfg.NodeClientKey,
		CAFile:        cfg.NodeServerCAFile,
		ServerName:    cfg.NodeServerName,
		Dispatcher:    dispatcher,
		HelloProvider: nodeagent.DefaultHelloProvider(cfg.NodeID, nodeagent.ConfigHasher(api)),
		OnConnected: func(ctx context.Context, sender nodeagent.Sender) {
			dispatcher.SetSender(sender)
			usagePusher.SetSender(sender)
			pusherStart.Do(func() {
				go func() {
					if err := usagePusher.Run(context.Background()); err != nil && !errors.Is(err, context.Canceled) {
						log.Printf("pulse-node usage pusher exited: %v", err)
					}
				}()
			})
			// 重连后立即触发一次 push，避免等下一个 ticker（最多 30s）才把
			// 离线期间累积的 delta 上报。首次连接命中 baseline 分支无副作用。
			go usagePusher.Tick(ctx)
		},
	}

	log.Printf("pulse-node connecting to grpc hub: node_id=%s server=%s", cfg.NodeID, serverAddr)

	if err := nodeagent.Run(ctx, agentCfg); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}
