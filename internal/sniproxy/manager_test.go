package sniproxy

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// TestManagerConfig_JSONWireFormat 保护 Route.Mode 字段的字符串枚举格式。
// 这条测试存在的原因：早期版本 RouteMode 是 int，而服务端发 "terminating" 字符串，
// 导致 Decode 失败整个 sync 被拒。若有人把 RouteMode 改回 int 此测试立刻失败。
func TestManagerConfig_JSONWireFormat(t *testing.T) {
	wire := `{
		"listen": ":443",
		"routes": [
			{"sni": "a.example.com", "backend": "127.0.0.1:20149", "mode": "terminating"},
			{"sni": "b.example.com", "backend": "1.2.3.4:443", "mode": "transparent"}
		]
	}`
	var cfg ManagerConfig
	if err := json.Unmarshal([]byte(wire), &cfg); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(cfg.Routes) != 2 {
		t.Fatalf("got %d routes, want 2", len(cfg.Routes))
	}
	if cfg.Routes[0].Mode != ModeTerminating {
		t.Errorf("route[0].Mode = %q, want %q", cfg.Routes[0].Mode, ModeTerminating)
	}
	if cfg.Routes[1].Mode != ModeTransparent {
		t.Errorf("route[1].Mode = %q, want %q", cfg.Routes[1].Mode, ModeTransparent)
	}

	// 反向：Route 写出的 JSON 应包含小写字符串
	out, err := json.Marshal(Route{SNI: "x", Backend: "y", Mode: ModeTerminating})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(out, []byte(`"mode":"terminating"`)) {
		t.Errorf("serialized route missing string mode: %s", out)
	}
}

func TestManager_RestoreMissing(t *testing.T) {
	m := NewManager(filepath.Join(t.TempDir(), "state.json"))
	if err := m.Restore(); err != nil {
		t.Errorf("Restore on missing file: %v", err)
	}
}

func TestManager_ApplyEmpty(t *testing.T) {
	m := NewManager(filepath.Join(t.TempDir(), "state.json"))
	defer m.Close()
	if err := m.Apply(ManagerConfig{}); err != nil {
		t.Errorf("Apply empty: %v", err)
	}
}

func TestManager_TransparentOnly(t *testing.T) {
	backend := startEchoTLS(t, "x.example.com")

	// 提前探测一个空闲端口
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := ln.Addr().String()
	_ = ln.Close()

	statePath := filepath.Join(t.TempDir(), "state.json")
	m := NewManager(statePath)
	defer m.Close()

	cfg := ManagerConfig{
		Listen: addr,
		Routes: []Route{
			{SNI: "x.example.com", Backend: backend, Mode: ModeTransparent},
		},
	}
	if err := m.Apply(cfg); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	waitForListen(t, addr)

	// 校验持久化
	data, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("state not written: %v", err)
	}
	var got ManagerConfig
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("state not valid JSON: %v", err)
	}
	if got.Listen != addr {
		t.Errorf("persisted Listen = %q, want %q", got.Listen, addr)
	}

	// 功能验证：一次 TLS 连接应经过透明代理到 backend
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	tc := tls.Client(conn, &tls.Config{ServerName: "x.example.com", InsecureSkipVerify: true})
	_ = tc.SetDeadline(time.Now().Add(3 * time.Second))
	if err := tc.Handshake(); err != nil {
		t.Fatalf("handshake: %v", err)
	}
	if _, err := tc.Write([]byte("hi")); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 2)
	if _, err := io.ReadFull(tc, buf); err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(buf) != "hi" {
		t.Errorf("echo = %q, want hi", buf)
	}
}

func TestManager_HotReloadRoutes(t *testing.T) {
	backendA := startEchoTLS(t, "a.example.com")
	backendB := startEchoTLS(t, "b.example.com")

	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := ln.Addr().String()
	_ = ln.Close()

	m := NewManager(filepath.Join(t.TempDir(), "state.json"))
	defer m.Close()

	_ = m.Apply(ManagerConfig{
		Listen: addr,
		Routes: []Route{{SNI: "a.example.com", Backend: backendA, Mode: ModeTransparent}},
	})
	waitForListen(t, addr)

	// 热更新加一条新路由，同一监听端口不应重启
	if err := m.Apply(ManagerConfig{
		Listen: addr,
		Routes: []Route{
			{SNI: "a.example.com", Backend: backendA, Mode: ModeTransparent},
			{SNI: "b.example.com", Backend: backendB, Mode: ModeTransparent},
		},
	}); err != nil {
		t.Fatal(err)
	}

	// 新增的 b 路由应可用
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	tc := tls.Client(conn, &tls.Config{ServerName: "b.example.com", InsecureSkipVerify: true})
	_ = tc.SetDeadline(time.Now().Add(3 * time.Second))
	if err := tc.Handshake(); err != nil {
		t.Fatalf("b handshake: %v", err)
	}
}

func TestManager_RestoreFromDisk(t *testing.T) {
	backend := startEchoTLS(t, "x.example.com")

	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := ln.Addr().String()
	_ = ln.Close()

	statePath := filepath.Join(t.TempDir(), "state.json")

	// 先用一个 Manager Apply 配置
	m1 := NewManager(statePath)
	_ = m1.Apply(ManagerConfig{
		Listen: addr,
		Routes: []Route{{SNI: "x.example.com", Backend: backend, Mode: ModeTransparent}},
	})
	waitForListen(t, addr)
	m1.Close()

	// 等端口释放（Linux 上 TCP TIME_WAIT 可能需要 SO_REUSEADDR，测试里等一下就好）
	time.Sleep(100 * time.Millisecond)

	// 新 Manager 从磁盘恢复，应该能监听同一端口
	m2 := NewManager(statePath)
	defer m2.Close()
	if err := m2.Restore(); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	waitForListen(t, addr)

	cfg := m2.Config()
	if len(cfg.Routes) != 1 || cfg.Routes[0].SNI != "x.example.com" {
		t.Errorf("restored cfg unexpected: %+v", cfg)
	}
}

// TestManager_ListenFailurePropagates 验证端口被占时 Apply 立刻返回错误，
// 且 Status 能反映失败原因；之后切换到新端口 Apply 必须能成功（Manager 不该卡死）。
func TestManager_ListenFailurePropagates(t *testing.T) {
	backend := startEchoTLS(t, "x.example.com")

	// 占住一个端口
	squatter, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	squattedAddr := squatter.Addr().String()
	defer squatter.Close()

	m := NewManager(filepath.Join(t.TempDir(), "state.json"))
	defer m.Close()

	err = m.Apply(ManagerConfig{
		Listen: squattedAddr,
		Routes: []Route{{SNI: "x.example.com", Backend: backend, Mode: ModeTransparent}},
	})
	if err == nil {
		t.Fatal("expected Apply to fail when port is bound")
	}

	st := m.Status()
	if st.Listen != "" {
		t.Errorf("Status.Listen = %q after failed Apply, want empty", st.Listen)
	}
	if st.LastError == "" {
		t.Error("Status.LastError empty after failed Apply")
	}

	// 换一个空闲端口再 Apply 应该成功
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	freshAddr := ln.Addr().String()
	_ = ln.Close()
	if err := m.Apply(ManagerConfig{
		Listen: freshAddr,
		Routes: []Route{{SNI: "x.example.com", Backend: backend, Mode: ModeTransparent}},
	}); err != nil {
		t.Fatalf("Apply on fresh port: %v", err)
	}
	waitForListen(t, freshAddr)

	st = m.Status()
	if st.Listen != freshAddr {
		t.Errorf("Status.Listen = %q, want %q", st.Listen, freshAddr)
	}
	if st.LastError != "" {
		t.Errorf("Status.LastError = %q, want empty after successful Apply", st.LastError)
	}
}

// TestManager_ApplyAfterFailedServe 验证 Serve 失败后 Manager 状态残留被清理，
// 下一次 Apply 走完整重启分支而不是对僵尸 proxy SetRoutes。
func TestManager_ApplyAfterFailedServe(t *testing.T) {
	backend := startEchoTLS(t, "x.example.com")

	squatter, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := squatter.Addr().String()
	defer squatter.Close()

	m := NewManager(filepath.Join(t.TempDir(), "state.json"))
	defer m.Close()

	// 第一次 Apply：必须失败（端口被占）
	if err := m.Apply(ManagerConfig{
		Listen: addr,
		Routes: []Route{{SNI: "x.example.com", Backend: backend, Mode: ModeTransparent}},
	}); err == nil {
		t.Fatal("first Apply should have failed (port squatted)")
	}

	// 释放端口
	_ = squatter.Close()
	time.Sleep(50 * time.Millisecond)

	// 第二次 Apply：相同配置，应该重启成功（而不是走热更新路径往僵尸 proxy SetRoutes）
	if err := m.Apply(ManagerConfig{
		Listen: addr,
		Routes: []Route{{SNI: "x.example.com", Backend: backend, Mode: ModeTransparent}},
	}); err != nil {
		t.Fatalf("retry Apply should succeed after port freed: %v", err)
	}
	waitForListen(t, addr)
}

func TestManager_TerminatingNeedsCFToken(t *testing.T) {
	m := NewManager("")
	defer m.Close()
	err := m.Apply(ManagerConfig{
		Listen: ":0",
		Routes: []Route{{SNI: "x", Backend: "127.0.0.1:1", Mode: ModeTerminating}},
	})
	if err == nil {
		t.Error("expected error when terminating route present but no CF token")
	}
}

// TestManager_ConcurrentApply 用几个 goroutine 同时 Apply，确保内部锁没问题。
func TestManager_ConcurrentApply(t *testing.T) {
	backend := startEchoTLS(t, "x.example.com")
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := ln.Addr().String()
	_ = ln.Close()

	m := NewManager(filepath.Join(t.TempDir(), "state.json"))
	defer m.Close()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = m.Apply(ManagerConfig{
				Listen: addr,
				Routes: []Route{{SNI: "x.example.com", Backend: backend, Mode: ModeTransparent}},
			})
		}()
	}
	wg.Wait()

	// 收尾：Listen 应可用
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	for {
		select {
		case <-ctx.Done():
			t.Fatal("proxy never came up")
		default:
		}
		c, err := net.Dial("tcp", addr)
		if err == nil {
			_ = c.Close()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
}
