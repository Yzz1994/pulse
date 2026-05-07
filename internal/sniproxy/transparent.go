package sniproxy

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// TransparentRoute 定义一条 SNI → 后端的透明转发规则。
// 连接到达时根据客户端 TLS SNI 选择对应 Backend，原样 TCP 转发，不终止 TLS。
type TransparentRoute struct {
	SNI     string // 客户端 TLS ClientHello 中的 server_name
	Backend string // 目标地址，如 "203.0.113.20:20148"
}

// TransparentProxy 是透明 SNI 代理，用于前置节点转发 TLS 到落地节点。
// 并发安全：Routes 通过 atomic.Pointer 支持运行时热更新。
type TransparentProxy struct {
	// Addr 监听地址，如 ":443"
	Addr string
	// HandshakeTimeout 读 ClientHello 的超时，0 表示 10 秒
	HandshakeTimeout time.Duration
	// DialTimeout 连接后端的超时，0 表示 5 秒
	DialTimeout time.Duration

	routes atomic.Pointer[map[string]string] // sni → backend
	ln     net.Listener
	wg     sync.WaitGroup
	closed atomic.Bool
}

// SetRoutes 热更新路由表。可以在 Serve 运行期间调用。
func (p *TransparentProxy) SetRoutes(rs []TransparentRoute) {
	m := make(map[string]string, len(rs))
	for _, r := range rs {
		if r.SNI != "" && r.Backend != "" {
			m[r.SNI] = r.Backend
		}
	}
	p.routes.Store(&m)
}

// Serve 启动监听循环，阻塞直到 ctx 取消或 listener 被关闭。
// 路由表必须在调用前通过 SetRoutes 初始化。
func (p *TransparentProxy) Serve(ctx context.Context) error {
	if p.routes.Load() == nil {
		return fmt.Errorf("sniproxy: routes not configured")
	}
	ln, err := net.Listen("tcp", p.Addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", p.Addr, err)
	}
	p.ln = ln

	// ctx 取消时主动关闭 listener 使 Accept 返回
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

func (p *TransparentProxy) handle(client net.Conn) {
	defer client.Close()

	hsTimeout := p.HandshakeTimeout
	if hsTimeout == 0 {
		hsTimeout = 10 * time.Second
	}
	_ = client.SetReadDeadline(time.Now().Add(hsTimeout))

	sni, peeked, err := PeekSNI(client)
	if err != nil {
		return
	}
	_ = client.SetReadDeadline(time.Time{})

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

	// 把解析 ClientHello 时已读的字节回放给后端
	if _, err := upstream.Write(peeked); err != nil {
		return
	}

	pipe(client, upstream)
}

// closeWriter 由任何支持半关写端的 Conn 实现（*net.TCPConn、*tls.Conn 都实现）。
type closeWriter interface {
	CloseWrite() error
}

// pipe 在两个连接间双向拷贝，任一方向结束即返回。
// 转发结束后尝试半关写端，使对端从 Read 得到 EOF 而不是等 Close 超时。
func pipe(a, b net.Conn) {
	done := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(b, a)
		if cw, ok := b.(closeWriter); ok {
			_ = cw.CloseWrite()
		}
		done <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(a, b)
		if cw, ok := a.(closeWriter); ok {
			_ = cw.CloseWrite()
		}
		done <- struct{}{}
	}()
	<-done
}
