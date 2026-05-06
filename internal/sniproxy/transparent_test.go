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

// startEchoTLS 启动一个真正的 TLS server，收到的明文原样 echo 回去。
// 返回监听地址和 cleanup 函数。
func startEchoTLS(t *testing.T, sni string) string {
	t.Helper()
	cert := genSelfSigned(t)
	cfg := &tls.Config{Certificates: []tls.Certificate{cert}, ServerName: sni}
	l, err := tls.Listen("tcp", "127.0.0.1:0", cfg)
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

func TestTransparent_RoutesBySNI(t *testing.T) {
	// 两个不同 SNI 对应两个不同后端
	backendA := startEchoTLS(t, "a.example.com")
	backendB := startEchoTLS(t, "b.example.com")

	proxy := &TransparentProxy{Addr: "127.0.0.1:0"}
	proxy.SetRoutes([]TransparentRoute{
		{SNI: "a.example.com", Backend: backendA},
		{SNI: "b.example.com", Backend: backendB},
	})

	// 用一个预先创建的 listener 来获取实际端口
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_ = ln.Close()
	proxy.Addr = ln.Addr().String()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = proxy.Serve(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		wg.Wait()
	})

	waitForListen(t, proxy.Addr)

	for _, sni := range []string{"a.example.com", "b.example.com"} {
		t.Run(sni, func(t *testing.T) {
			conn, err := net.Dial("tcp", proxy.Addr)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			tc := tls.Client(conn, &tls.Config{ServerName: sni, InsecureSkipVerify: true})
			_ = tc.SetDeadline(time.Now().Add(3 * time.Second))
			if err := tc.Handshake(); err != nil {
				t.Fatalf("handshake: %v", err)
			}
			want := []byte("ping-" + sni)
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

func TestTransparent_UnknownSNIDropped(t *testing.T) {
	proxy := &TransparentProxy{Addr: "127.0.0.1:0"}
	proxy.SetRoutes([]TransparentRoute{{SNI: "known.example.com", Backend: "127.0.0.1:1"}})

	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	_ = ln.Close()
	proxy.Addr = ln.Addr().String()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); _ = proxy.Serve(ctx) }()
	t.Cleanup(func() { cancel(); wg.Wait() })

	waitForListen(t, proxy.Addr)

	conn, err := net.Dial("tcp", proxy.Addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	tc := tls.Client(conn, &tls.Config{ServerName: "unknown.example.com", InsecureSkipVerify: true})
	_ = tc.SetDeadline(time.Now().Add(2 * time.Second))
	// 未命中路由应该被直接关闭，握手会失败
	if err := tc.Handshake(); err == nil {
		t.Error("expected handshake to fail for unknown SNI")
	}
}

func TestTransparent_HotReload(t *testing.T) {
	backendA := startEchoTLS(t, "a.example.com")
	backendB := startEchoTLS(t, "b.example.com")

	proxy := &TransparentProxy{Addr: "127.0.0.1:0"}
	proxy.SetRoutes([]TransparentRoute{{SNI: "a.example.com", Backend: backendA}})

	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	_ = ln.Close()
	proxy.Addr = ln.Addr().String()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); _ = proxy.Serve(ctx) }()
	t.Cleanup(func() { cancel(); wg.Wait() })

	waitForListen(t, proxy.Addr)

	// 热更新路由，加入 b.example.com
	proxy.SetRoutes([]TransparentRoute{
		{SNI: "a.example.com", Backend: backendA},
		{SNI: "b.example.com", Backend: backendB},
	})

	conn, err := net.Dial("tcp", proxy.Addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	tc := tls.Client(conn, &tls.Config{ServerName: "b.example.com", InsecureSkipVerify: true})
	_ = tc.SetDeadline(time.Now().Add(3 * time.Second))
	if err := tc.Handshake(); err != nil {
		t.Fatalf("b should work after hot reload: %v", err)
	}
}

func waitForListen(t *testing.T, addr string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		c, err := net.Dial("tcp", addr)
		if err == nil {
			_ = c.Close()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("proxy never came up on %s", addr)
}
