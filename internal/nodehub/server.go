package nodehub

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"

	"pulse/internal/cert"
	nodev1 "pulse/internal/pb/nodev1"
)

// ServerOptions 配置 nodehub gRPC 监听。
type ServerOptions struct {
	Addr       string // 默认 ":8082"
	CA         *cert.NodeCA
	ServerCert tls.Certificate
	Hub        *Hub

	// gRPC keepalive 参数。零值时使用默认值（见下方注释），不退化为 grpc 默认行为。
	KeepaliveTime         time.Duration // 服务端主动 ping 间隔，默认 30s
	KeepaliveTimeout      time.Duration // ping 等待 ack 超时，默认 10s
	MaxConnectionIdle     time.Duration // 默认 0（不限）
	MaxConnectionAge      time.Duration // 默认 0（不限；可选限制强制重连周期）
	PermitWithoutStream   bool          // 默认 true（Hub 在长流外仍接受 client ping）
	MinClientPingInterval time.Duration // 默认 25s（防 client 过频 ping 被 grpc 标记为 too_many_pings）

	// permitWithoutStreamSet 内部标记，区分 PermitWithoutStream==false 的"未设"与"显式 false"。
	// 测试可能需要显式 false，生产路径走默认 true。
	permitWithoutStreamSet bool
}

// ListenAndServe 启动 gRPC server（mTLS），阻塞直到 ctx 取消或 listener 出错。
// ctx 取消时执行 GracefulStop，5s 超时后 Stop。
func ListenAndServe(ctx context.Context, opts ServerOptions) error {
	if opts.Hub == nil {
		return errors.New("nodehub: ServerOptions.Hub is required")
	}
	if opts.CA == nil {
		return errors.New("nodehub: ServerOptions.CA is required")
	}
	addr := opts.Addr
	if addr == "" {
		addr = ":8082"
	}

	tlsCfg := &tls.Config{
		Certificates: []tls.Certificate{opts.ServerCert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    opts.CA.ClientCAPool(),
		MinVersion:   tls.VersionTLS12,
	}

	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	if opts.KeepaliveTime == 0 {
		opts.KeepaliveTime = 30 * time.Second
	}
	if opts.KeepaliveTimeout == 0 {
		opts.KeepaliveTimeout = 10 * time.Second
	}
	if opts.MinClientPingInterval == 0 {
		opts.MinClientPingInterval = 25 * time.Second
	}
	if !opts.permitWithoutStreamSet {
		opts.PermitWithoutStream = true
	}

	srv := grpc.NewServer(
		grpc.Creds(credentials.NewTLS(tlsCfg)),
		grpc.KeepaliveParams(keepalive.ServerParameters{
			Time:              opts.KeepaliveTime,
			Timeout:           opts.KeepaliveTimeout,
			MaxConnectionIdle: opts.MaxConnectionIdle,
			MaxConnectionAge:  opts.MaxConnectionAge,
		}),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             opts.MinClientPingInterval,
			PermitWithoutStream: opts.PermitWithoutStream,
		}),
	)
	nodev1.RegisterNodeAgentServer(srv, opts.Hub)

	// 启动 reaper（关闭超过 DeadConnectionTimeout 没收到任何帧的连接）。
	go opts.Hub.RunReaper(ctx)

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Serve(lis)
	}()

	select {
	case <-ctx.Done():
		stopped := make(chan struct{})
		go func() {
			srv.GracefulStop()
			close(stopped)
		}()
		select {
		case <-stopped:
		case <-time.After(5 * time.Second):
			srv.Stop()
		}
		return nil
	case err := <-errCh:
		return err
	}
}
