package sniproxy

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"sync"
	"testing"
	"time"
)

// startEchoPlaintext 启动一个纯 TCP echo server，模拟 xray 明文 inbound。
func startEchoPlaintext(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				_, _ = io.Copy(c, c)
			}(c)
		}
	}()
	return l.Addr().String()
}

func TestTerminating_DecryptsAndForwards(t *testing.T) {
	// 模拟两个 xray 明文后端
	backendA := startEchoPlaintext(t)
	backendB := startEchoPlaintext(t)

	// 一张自签证书服务所有 SNI（测试用）
	cert := genSelfSigned(t)
	tlsCfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
	}

	proxy := &TerminatingProxy{
		Addr:      "127.0.0.1:0",
		TLSConfig: tlsCfg,
	}
	proxy.SetRoutes([]TerminatingRoute{
		{SNI: "a.example.com", Backend: backendA},
		{SNI: "b.example.com", Backend: backendB},
	})

	// 提前分配端口
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := ln.Addr().String()
	_ = ln.Close()
	proxy.Addr = addr

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); _ = proxy.Serve(ctx) }()
	t.Cleanup(func() { cancel(); wg.Wait() })

	waitForListen(t, addr)

	for _, sni := range []string{"a.example.com", "b.example.com"} {
		t.Run(sni, func(t *testing.T) {
			conn, err := net.Dial("tcp", addr)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			tc := tls.Client(conn, &tls.Config{ServerName: sni, InsecureSkipVerify: true})
			_ = tc.SetDeadline(time.Now().Add(3 * time.Second))
			if err := tc.Handshake(); err != nil {
				t.Fatalf("handshake: %v", err)
			}
			want := []byte("hello-" + sni)
			if _, err := tc.Write(want); err != nil {
				t.Fatal(err)
			}
			got := make([]byte, len(want))
			if _, err := io.ReadFull(tc, got); err != nil {
				t.Fatal(err)
			}
			if string(got) != string(want) {
				t.Errorf("echo = %q, want %q", got, want)
			}
		})
	}
}

func TestTerminating_UnknownSNIDropped(t *testing.T) {
	cert := genSelfSigned(t)
	proxy := &TerminatingProxy{
		Addr:      "127.0.0.1:0",
		TLSConfig: &tls.Config{Certificates: []tls.Certificate{cert}},
	}
	proxy.SetRoutes([]TerminatingRoute{{SNI: "known.example.com", Backend: "127.0.0.1:1"}})

	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := ln.Addr().String()
	_ = ln.Close()
	proxy.Addr = addr

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); _ = proxy.Serve(ctx) }()
	t.Cleanup(func() { cancel(); wg.Wait() })

	waitForListen(t, addr)

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	tc := tls.Client(conn, &tls.Config{ServerName: "unknown.example.com", InsecureSkipVerify: true})
	_ = tc.SetDeadline(time.Now().Add(2 * time.Second))
	// 握手可以成功（证书覆盖任意 SNI），但握手后连接应被关闭
	if err := tc.Handshake(); err != nil {
		return // 握手被拒绝也算符合预期
	}
	// 能握手则后续 Read 应立刻返回 EOF
	buf := make([]byte, 1)
	if _, err := tc.Read(buf); err == nil {
		t.Error("unknown SNI connection should be closed, but Read succeeded")
	}
}

func TestTerminating_RequiresCert(t *testing.T) {
	proxy := &TerminatingProxy{
		Addr:      "127.0.0.1:0",
		TLSConfig: &tls.Config{}, // 缺 GetCertificate 和 Certificates
	}
	proxy.SetRoutes([]TerminatingRoute{{SNI: "x", Backend: "127.0.0.1:1"}})
	err := proxy.Serve(context.Background())
	if err == nil {
		t.Error("expected error when neither GetCertificate nor Certificates provided")
	}
}
