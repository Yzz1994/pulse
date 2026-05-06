package nodehub

import (
	"sync/atomic"
	"testing"
)

func TestMultiPushHandler_NilFieldsAreNoop(t *testing.T) {
	m := &MultiPushHandler{}
	m.OnHello("n1", []byte(`{}`))
	if err := m.OnUsagePush("n1", 1, []byte(`{}`)); err != nil {
		t.Fatalf("nil UsagePushHandler should return nil, got %v", err)
	}
	m.OnLog("n1", "r1", []byte(`{}`))
	m.OnTracerouteHop("n1", "r1", []byte(`{}`))
}

func TestMultiPushHandler_DispatchesEachEvent(t *testing.T) {
	var hello, usage, logs, hops atomic.Int32
	m := &MultiPushHandler{
		HelloHandler:         func(string, []byte) { hello.Add(1) },
		UsagePushHandler:     func(string, uint64, []byte) error { usage.Add(1); return nil },
		LogHandler:           func(string, string, []byte) { logs.Add(1) },
		TracerouteHopHandler: func(string, string, []byte) { hops.Add(1) },
	}

	m.OnHello("n", nil)
	_ = m.OnUsagePush("n", 1, nil)
	m.OnLog("n", "r", nil)
	m.OnTracerouteHop("n", "r", nil)

	if hello.Load() != 1 || usage.Load() != 1 || logs.Load() != 1 || hops.Load() != 1 {
		t.Fatalf("expected each handler called once, got hello=%d usage=%d logs=%d hops=%d",
			hello.Load(), usage.Load(), logs.Load(), hops.Load())
	}
}

func TestMultiPushHandler_NilReceiverSafe(t *testing.T) {
	var m *MultiPushHandler
	m.OnHello("n", nil)
	if err := m.OnUsagePush("n", 0, nil); err != nil {
		t.Fatalf("nil receiver should be safe, got %v", err)
	}
	m.OnLog("n", "r", nil)
	m.OnTracerouteHop("n", "r", nil)
}
