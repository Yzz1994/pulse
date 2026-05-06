package sniproxy

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// TerminatingRoute 把一个 SNI 映射到本地 xray 的明文监听地址。
// 客户端 TLS 被 TerminatingProxy 终止后，明文流量转发到 Backend。
type TerminatingRoute struct {
	SNI     string // 客户端 TLS ClientHello 中的 server_name
	Backend string // xray 明文监听地址，如 "127.0.0.1:20149"
}

// TerminatingProxy 是 TLS 终止 SNI 代理，用于落地节点多 inbound 共用单端口。
// TLS 证书由调用方通过 TLSConfig 提供（通常来自 certmgr.Manager）。
type TerminatingProxy struct {
	// Addr 监听地址，如 ":443"
	Addr string
	// TLSConfig 必须配置好 GetCertificate（一般用 certmgr.Manager.TLSConfig()）
	TLSConfig *tls.Config
	// DialTimeout 连接后端的超时，0 表示 5 秒
	DialTimeout time.Duration

	routes atomic.Pointer[map[string]string] // sni → backend
	ln     net.Listener
	wg     sync.WaitGroup
	closed atomic.Bool
}

// SetRoutes 热更新路由表。
func (p *TerminatingProxy) SetRoutes(rs []TerminatingRoute) {
	m := make(map[string]string, len(rs))
	for _, r := range rs {
		if r.SNI != "" && r.Backend != "" {
			m[r.SNI] = r.Backend
		}
	}
	p.routes.Store(&m)
}

// Serve 启动监听循环，阻塞直到 ctx 取消或 listener 被关闭。
func (p *TerminatingProxy) Serve(ctx context.Context) error {
	if p.TLSConfig == nil {
		return fmt.Errorf("sniproxy: TLSConfig is required")
	}
	if p.TLSConfig.GetCertificate == nil && len(p.TLSConfig.Certificates) == 0 {
		return fmt.Errorf("sniproxy: TLSConfig needs GetCertificate or Certificates")
	}
	if p.routes.Load() == nil {
		return fmt.Errorf("sniproxy: routes not configured")
	}

	ln, err := tls.Listen("tcp", p.Addr, p.TLSConfig)
	if err != nil {
		return fmt.Errorf("tls.Listen %s: %w", p.Addr, err)
	}
	p.ln = ln

	go func() {
		<-ctx.Done()
		p.closed.Store(true)
		_ = ln.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			if p.closed.Load() {
				p.wg.Wait()
				return nil
			}
			return fmt.Errorf("accept: %w", err)
		}
		p.wg.Add(1)
		go func(c net.Conn) {
			defer p.wg.Done()
			p.handle(c)
		}(conn)
	}
}

func (p *TerminatingProxy) handle(client net.Conn) {
	defer client.Close()

	tlsConn, ok := client.(*tls.Conn)
	if !ok {
		return
	}

	// 强制完成 TLS 握手，之后 ConnectionState().ServerName 才有值。
	_ = tlsConn.SetDeadline(time.Now().Add(10 * time.Second))
	if err := tlsConn.Handshake(); err != nil {
		return
	}
	_ = tlsConn.SetDeadline(time.Time{})

	sni := tlsConn.ConnectionState().ServerName
	if sni == "" {
		return
	}

	routes := p.routes.Load()
	if routes == nil {
		return
	}
	backend, ok := (*routes)[sni]
	if !ok {
		return
	}

	dialTimeout := p.DialTimeout
	if dialTimeout == 0 {
		dialTimeout = 5 * time.Second
	}
	upstream, err := net.DialTimeout("tcp", backend, dialTimeout)
	if err != nil {
		log.Printf("sniproxy: dial %s (sni=%s): %v", backend, sni, err)
		return
	}
	defer upstream.Close()

	pipe(client, upstream)
}
