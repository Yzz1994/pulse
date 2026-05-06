package sniproxy

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestUnified_HTTPReverse_InjectsHeaders 验证命中 http-reverse 路由时：
//   - TLS 正确终止
//   - HTTP 请求被反代到本地后端
//   - X-Forwarded-For / X-Forwarded-Proto / X-Real-IP / X-Forwarded-Host 正确注入
//   - Host 头改写为 SNI，不是 127.0.0.1
func TestUnified_HTTPReverse_InjectsHeaders(t *testing.T) {
	var gotHeaders http.Header
	var gotHost string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeaders = r.Header.Clone()
		gotHost = r.Host
		fmt.Fprint(w, "ok")
	}))
	defer backend.Close()

	// backend.URL 形如 http://127.0.0.1:xxxxx，剥掉 scheme
	backendAddr := strings.TrimPrefix(backend.URL, "http://")

	cert := genSelfSigned(t)
	proxy := &UnifiedProxy{Addr: "127.0.0.1:0"}
	proxy.SetTLSConfig(&tls.Config{Certificates: []tls.Certificate{cert}})
	proxy.SetRoutes([]Route{
		{SNI: "panel.example.com", Backend: backendAddr, Mode: ModeHTTPReverse},
	})

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

	// 模拟客户端用 SNI=panel.example.com 连 sniproxy
	// 测试里禁掉客户端 keepalive：Get 返回时立刻关 conn，sniproxy 端 ConnState
	// 回调触发 → listener 关闭 → Serve 退出 → handler goroutine 干净结束。
	// 生产代码仍保持 keepalive 开启。
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				ServerName:         "panel.example.com",
				InsecureSkipVerify: true,
			},
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("tcp", addr)
			},
			DisableKeepAlives: true,
		},
		Timeout: 3 * time.Second,
	}
	resp, err := client.Get("https://panel.example.com/")
	if err != nil {
		t.Fatalf("client.Get: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "ok" {
		t.Errorf("body = %q, want 'ok'", body)
	}
	if resp.StatusCode != 200 {
		t.Errorf("status = %d", resp.StatusCode)
	}

	// 断言 header 注入
	if gotHost != "panel.example.com" {
		t.Errorf("backend Host = %q, want panel.example.com", gotHost)
	}
	if gotHeaders.Get("X-Forwarded-Proto") != "https" {
		t.Errorf("X-Forwarded-Proto = %q, want https", gotHeaders.Get("X-Forwarded-Proto"))
	}
	if gotHeaders.Get("X-Forwarded-Host") != "panel.example.com" {
		t.Errorf("X-Forwarded-Host = %q", gotHeaders.Get("X-Forwarded-Host"))
	}
	xff := gotHeaders.Get("X-Forwarded-For")
	if xff == "" || !strings.HasPrefix(xff, "127.0.0.1") {
		t.Errorf("X-Forwarded-For = %q, want 127.0.0.1...", xff)
	}
	xri := gotHeaders.Get("X-Real-IP")
	if xri == "" {
		t.Errorf("X-Real-IP empty")
	}
}

// TestUnified_HTTPReverse_Streaming 验证 SSE 流式响应不被缓冲。
func TestUnified_HTTPReverse_Streaming(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		f, _ := w.(http.Flusher)
		for i := 0; i < 3; i++ {
			fmt.Fprintf(w, "data: %d\n\n", i)
			if f != nil {
				f.Flush()
			}
			time.Sleep(30 * time.Millisecond)
		}
	}))
	defer backend.Close()

	cert := genSelfSigned(t)
	proxy := &UnifiedProxy{Addr: "127.0.0.1:0"}
	proxy.SetTLSConfig(&tls.Config{Certificates: []tls.Certificate{cert}})
	proxy.SetRoutes([]Route{
		{SNI: "x.example.com", Backend: strings.TrimPrefix(backend.URL, "http://"), Mode: ModeHTTPReverse},
	})

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

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{ServerName: "x.example.com", InsecureSkipVerify: true},
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("tcp", addr)
			},
			DisableKeepAlives: true,
		},
		Timeout: 5 * time.Second,
	}
	resp, err := client.Get("https://x.example.com/events")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "data: 0") || !strings.Contains(string(body), "data: 2") {
		t.Errorf("expected all events, got: %q", body)
	}
}
