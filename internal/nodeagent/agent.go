// Package nodeagent 实现 node 侧的 gRPC 客户端：主动连 server，建立双向
// Session 流，处理 server 下发的 method 调用，提供主动事件推送（usage_push、
// log、traceroute_hop 等）的接口，并在断线后按指数退避重连。
//
// 本包不实现具体业务 method 的 dispatcher（runtime/status/usage 等业务由
// 上层 dispatch 包注入 Dispatcher）。
package nodeagent

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"

	"pulse/internal/buildinfo"
)

// Dispatcher 处理 server 下发的 method 调用。Handle 必须支持 ctx 取消：
// 当 server 发送对应的 cancel_id 帧时，agent 会取消传入的 ctx。
type Dispatcher interface {
	Handle(ctx context.Context, method string, body json.RawMessage) (json.RawMessage, error)
}

// NoopDispatcher 是用于测试或过渡期的空实现。
type NoopDispatcher struct{}

// Handle 总是返回 method not implemented。
func (NoopDispatcher) Handle(ctx context.Context, method string, body json.RawMessage) (json.RawMessage, error) {
	return nil, fmt.Errorf("nodeagent: method %q not implemented", method)
}

// Sender 提供 Dispatcher 或外部 caller 主动向 server 推送事件的能力。
// 实现内部对 grpc stream Send 加锁，调用是线程安全的。
type Sender interface {
	// PushEvent 推送一条非请求/响应的事件帧。reqID 通常留空（对 usage_push、
	// hello 而言），log/traceroute_hop 应携带触发它们的 server 请求 id。
	PushEvent(reqID, event string, body []byte, seq uint64) error
	// WaitAck 阻塞等待对应 seq 的 ack（usage_push 用），ctx 控制超时。
	// 在 WaitAck 之前调用 PushEvent；同一个 seq 仅会被 Wait 一次。
	WaitAck(ctx context.Context, seq uint64) error
}

// Config 是 Run 的输入。
type Config struct {
	NodeID     string
	ServerAddr string // gRPC 地址，如 "controlplane.example.com:8082"

	// mTLS 证书路径（生产路径）。若 InsecureSkipTLS 为 true，可留空。
	CertFile string
	KeyFile  string
	CAFile   string

	// ServerName 用于校验 server 证书。留空时从 ServerAddr 解析 host。
	ServerName string

	Dispatcher Dispatcher

	// HelloProvider 在每次（含重连）Session 建立时被调用，返回 hello 帧 body。
	HelloProvider func() (json.RawMessage, error)

	// ReconnectBackoff 在重连失败时按 attempt 索引取等待时长，超过尾部时
	// 沿用最后一项。默认 [2s, 5s, 15s, 60s]。
	ReconnectBackoff []time.Duration

	// gRPC keepalive 参数，先放进 Config 供 keepalive-tune todo 调整。
	// 默认 KeepaliveTime=30s, KeepaliveTimeout=10s。
	KeepaliveTime    time.Duration
	KeepaliveTimeout time.Duration

	Logger *slog.Logger

	// OnConnected 在每次 Session 成功发出 hello 帧后被调用一次，传入当前
	// session 的 Sender。回调应快速返回（启动需要的后台 goroutine 即可）；
	// 当 session 结束时，传入的 Sender 上的调用会返回错误，调用方需配合
	// ctx 退出后台 goroutine。本 todo 用此机制把 Sender 暴露给上层。
	OnConnected func(ctx context.Context, sender Sender)

	// Dialer 自定义 gRPC 客户端连接构造方式。零值使用 ServerAddr + 加载的
	// mTLS 凭证 + keepalive 默认参数构建（生产路径）。允许调用方注入自定义
	// 连接逻辑（service mesh 集成、bufconn 等）。
	Dialer func(ctx context.Context) (*grpc.ClientConn, error)
}

// DefaultHelloProvider 构造一个常见的 hello 帧 provider：
// 返回 JSON {"node_id":..., "config_hash":..., "version":...}。
// configHasher 由调用方注入，常见做法是返回 xray 配置 hash。
func DefaultHelloProvider(nodeID string, configHasher func() string) func() (json.RawMessage, error) {
	if configHasher == nil {
		configHasher = func() string { return "" }
	}
	return func() (json.RawMessage, error) {
		payload := struct {
			NodeID     string `json:"node_id"`
			ConfigHash string `json:"config_hash"`
			Version    string `json:"version"`
		}{
			NodeID:     nodeID,
			ConfigHash: configHasher(),
			Version:    buildinfo.Version,
		}
		return json.Marshal(payload)
	}
}

// Run 阻塞运行 agent，直到 ctx 取消。期间任何错误（连接失败、stream 出错、
// Recv EOF 等）都会触发指数退避重连。
func Run(ctx context.Context, cfg Config) error {
	if cfg.Dispatcher == nil {
		return errors.New("nodeagent: Config.Dispatcher is required")
	}
	if cfg.HelloProvider == nil {
		return errors.New("nodeagent: Config.HelloProvider is required")
	}
	if cfg.ServerAddr == "" && cfg.Dialer == nil {
		return errors.New("nodeagent: Config.ServerAddr or Config.Dialer is required")
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if len(cfg.ReconnectBackoff) == 0 {
		cfg.ReconnectBackoff = []time.Duration{2 * time.Second, 5 * time.Second, 15 * time.Second, 60 * time.Second}
	}
	if cfg.KeepaliveTime == 0 {
		cfg.KeepaliveTime = 30 * time.Second
	}
	if cfg.KeepaliveTimeout == 0 {
		cfg.KeepaliveTimeout = 10 * time.Second
	}

	// 默认 Dialer：从 Cert/Key/CA 加载 mTLS，按 ServerAddr 建立 gRPC 连接。
	if cfg.Dialer == nil {
		creds, err := loadTLSCreds(cfg)
		if err != nil {
			return fmt.Errorf("nodeagent: load TLS: %w", err)
		}
		cfg.Dialer = func(_ context.Context) (*grpc.ClientConn, error) {
			return grpc.NewClient(cfg.ServerAddr,
				grpc.WithTransportCredentials(creds),
				keepaliveParams(cfg),
			)
		}
	}

	attempts := 0
	for {
		err := runSession(ctx, cfg)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		// 选取等待时长：min(attempts, len-1)。
		idx := attempts
		if idx >= len(cfg.ReconnectBackoff) {
			idx = len(cfg.ReconnectBackoff) - 1
		}
		wait := cfg.ReconnectBackoff[idx]
		cfg.Logger.Warn("nodeagent: session ended, reconnecting",
			"node_id", cfg.NodeID, "wait", wait, "attempt", attempts, "err", err)
		select {
		case <-time.After(wait):
			attempts++
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func loadTLSCreds(cfg Config) (credentials.TransportCredentials, error) {
	if cfg.CertFile == "" || cfg.KeyFile == "" || cfg.CAFile == "" {
		return nil, errors.New("CertFile/KeyFile/CAFile required when Dialer is not set")
	}
	clientCert, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("load client keypair: %w", err)
	}
	caPEM, err := os.ReadFile(cfg.CAFile)
	if err != nil {
		return nil, fmt.Errorf("read CA: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, errors.New("invalid CA PEM")
	}
	// 只校验 server 证书是否由受信任的 NodeCA 签发（CA pinning），
	// 不校验 hostname/IP SAN，使控制面 IP 变动后无需重签证书。
	return credentials.NewTLS(&tls.Config{
		InsecureSkipVerify: true, //nolint:gosec // 见下方 VerifyPeerCertificate
		Certificates:       []tls.Certificate{clientCert},
		MinVersion:         tls.VersionTLS12,
		VerifyPeerCertificate: func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			certs := make([]*x509.Certificate, 0, len(rawCerts))
			for _, raw := range rawCerts {
				c, err := x509.ParseCertificate(raw)
				if err != nil {
					return err
				}
				certs = append(certs, c)
			}
			if len(certs) == 0 {
				return errors.New("server sent no certificates")
			}
			intermediates := x509.NewCertPool()
			for _, c := range certs[1:] {
				intermediates.AddCert(c)
			}
			_, err := certs[0].Verify(x509.VerifyOptions{
				Roots:         pool,
				Intermediates: intermediates,
			})
			return err
		},
	}), nil
}

// keepaliveParams 返回 grpc.WithKeepaliveParams 选项。
// PermitWithoutStream=true 让 client 在没有 active stream 时也能维持 ping，
// 与 server 的 EnforcementPolicy.PermitWithoutStream=true 配合，配合 reaper
// 实现死连接快速发现。
func keepaliveParams(cfg Config) grpc.DialOption {
	return grpc.WithKeepaliveParams(keepalive.ClientParameters{
		Time:                cfg.KeepaliveTime,
		Timeout:             cfg.KeepaliveTimeout,
		PermitWithoutStream: true,
	})
}
