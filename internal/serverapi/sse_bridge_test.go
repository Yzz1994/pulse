package serverapi

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeUpstream is an io.ReadCloser that yields chunks pushed via push() and
// blocks Read until either a chunk is available, EOF is signalled, or Close
// is called. It records how many times Close is invoked.
type fakeUpstream struct {
	mu        sync.Mutex
	cond      *sync.Cond
	chunks    [][]byte
	eof       bool
	closed    bool
	closeCnt  int32
	closeOnce sync.Once
}

func newFakeUpstream() *fakeUpstream {
	f := &fakeUpstream{}
	f.cond = sync.NewCond(&f.mu)
	return f
}

func (f *fakeUpstream) push(b []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed || f.eof {
		return
	}
	cp := make([]byte, len(b))
	copy(cp, b)
	f.chunks = append(f.chunks, cp)
	f.cond.Broadcast()
}

func (f *fakeUpstream) signalEOF() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.eof = true
	f.cond.Broadcast()
}

func (f *fakeUpstream) Read(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for len(f.chunks) == 0 && !f.closed && !f.eof {
		f.cond.Wait()
	}
	if len(f.chunks) > 0 {
		c := f.chunks[0]
		n := copy(p, c)
		if n < len(c) {
			f.chunks[0] = c[n:]
		} else {
			f.chunks = f.chunks[1:]
		}
		return n, nil
	}
	if f.closed {
		return 0, io.ErrClosedPipe
	}
	return 0, io.EOF
}

func (f *fakeUpstream) Close() error {
	atomic.AddInt32(&f.closeCnt, 1)
	f.closeOnce.Do(func() {
		f.mu.Lock()
		f.closed = true
		f.cond.Broadcast()
		f.mu.Unlock()
	})
	return nil
}

func (f *fakeUpstream) closeCount() int { return int(atomic.LoadInt32(&f.closeCnt)) }

func TestBridgeSSE_ForwardsAndFlushes(t *testing.T) {
	up := newFakeUpstream()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bridgeSSE(w, r, up)
	}))
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()

	if got := resp.Header.Get("Content-Type"); got != "text/event-stream" {
		t.Errorf("content-type = %q", got)
	}
	if got := resp.Header.Get("Cache-Control"); got != "no-cache" {
		t.Errorf("cache-control = %q", got)
	}
	if got := resp.Header.Get("X-Accel-Buffering"); got != "no" {
		t.Errorf("x-accel = %q", got)
	}

	up.push([]byte("data: hello\n\n"))
	up.push([]byte("data: world\n\n"))

	buf := make([]byte, 256)
	var collected strings.Builder
	deadline := time.Now().Add(2 * time.Second)
	for collected.Len() < len("data: hello\n\ndata: world\n\n") && time.Now().Before(deadline) {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			collected.Write(buf[:n])
		}
		if rerr != nil {
			break
		}
	}
	if !strings.Contains(collected.String(), "data: hello") || !strings.Contains(collected.String(), "data: world") {
		t.Errorf("collected = %q", collected.String())
	}

	up.signalEOF()
	// Drain remaining (handler returns, server closes connection).
	io.Copy(io.Discard, resp.Body) //nolint:errcheck

	// Wait briefly for handler defer to run.
	time.Sleep(50 * time.Millisecond)
	if up.closeCount() == 0 {
		t.Errorf("upstream Close not called after EOF")
	}
}

func TestBridgeSSE_ClientDisconnectClosesUpstream(t *testing.T) {
	up := newFakeUpstream()
	handlerDone := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bridgeSSE(w, r, up)
		close(handlerDone)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("get: %v", err)
	}

	// Push a frame to ensure the handler is in the read loop.
	up.push([]byte("data: x\n\n"))
	buf := make([]byte, 64)
	if _, rerr := resp.Body.Read(buf); rerr != nil {
		t.Fatalf("read first frame: %v", rerr)
	}

	// Client disconnects.
	cancel()
	resp.Body.Close()

	select {
	case <-handlerDone:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not return after client disconnect")
	}
	if up.closeCount() == 0 {
		t.Errorf("upstream Close not called after client disconnect")
	}
}

func TestBridgeSSE_UpstreamEOFExitsCleanly(t *testing.T) {
	up := newFakeUpstream()
	handlerDone := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bridgeSSE(w, r, up)
		close(handlerDone)
	}))
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()

	up.push([]byte("data: bye\n\n"))
	up.signalEOF()

	io.Copy(io.Discard, resp.Body) //nolint:errcheck

	select {
	case <-handlerDone:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not return after upstream EOF")
	}
	if up.closeCount() == 0 {
		t.Errorf("upstream Close not called")
	}
}
