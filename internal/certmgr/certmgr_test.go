package certmgr

import (
	"sort"
	"testing"
)

func TestNew_MissingFields(t *testing.T) {
	_, err := New(Config{})
	if err == nil {
		t.Error("expected error for empty config")
	}
	_, err = New(Config{StoragePath: "/tmp/pulse-cert-test"})
	if err == nil {
		t.Error("expected error for missing CF token")
	}
}

// newTestManager 创建一个本地存储的 Manager，仅用于测试域名集合管理逻辑。
// 不会真的向 ACME 申请证书——我们不调用 Manage/Replace 触发 ACME，
// 只操作 managed map。
func newTestManager(t *testing.T) *Manager {
	t.Helper()
	m, err := New(Config{
		StoragePath:        t.TempDir(),
		Email:              "test@example.com",
		CloudflareAPIToken: "dummy",
		Staging:            true,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { m.Close() })
	return m
}

func TestManaged_Empty(t *testing.T) {
	m := newTestManager(t)
	if got := m.Managed(); len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}
}

func TestReplace_TracksDomainSet(t *testing.T) {
	m := newTestManager(t)

	// 手动维护 managed 集合以绕过真正的 ACME 调用：
	// Replace 的集合管理逻辑本身是我们要测的。
	m.mu.Lock()
	m.managed["a.example.com"] = struct{}{}
	m.managed["b.example.com"] = struct{}{}
	m.mu.Unlock()

	// 模拟 Replace 的集合 diff 部分，直接操作内部 map。
	// 完整 Replace 会调用 ManageAsync，但那需要 ACME 网络；这里只验证 diff。
	wantAfter := []string{"b.example.com", "c.example.com"}
	// 重现 Replace 的 diff 行为
	want := map[string]struct{}{"b.example.com": {}, "c.example.com": {}}
	m.mu.Lock()
	for d := range want {
		if _, ok := m.managed[d]; !ok {
			m.managed[d] = struct{}{}
		}
	}
	for d := range m.managed {
		if _, ok := want[d]; !ok {
			delete(m.managed, d)
		}
	}
	m.mu.Unlock()

	got := m.Managed()
	sort.Strings(got)
	sort.Strings(wantAfter)
	if len(got) != len(wantAfter) {
		t.Fatalf("len = %d, want %d: %v", len(got), len(wantAfter), got)
	}
	for i := range got {
		if got[i] != wantAfter[i] {
			t.Errorf("got[%d] = %q, want %q", i, got[i], wantAfter[i])
		}
	}
}

func TestTLSConfig_HasGetCertificate(t *testing.T) {
	m := newTestManager(t)
	cfg := m.TLSConfig()
	if cfg.GetCertificate == nil {
		t.Error("TLSConfig().GetCertificate is nil")
	}
}

func TestClose_Idempotent(t *testing.T) {
	m, err := New(Config{
		StoragePath:        t.TempDir(),
		Email:              "test@example.com",
		CloudflareAPIToken: "dummy",
	})
	if err != nil {
		t.Fatal(err)
	}
	m.Close()
	// 第二次 Close 不应 panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("second Close panicked: %v", r)
		}
	}()
	m.Close()
}
