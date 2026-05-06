package nodes

import (
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
)

// fakeStream 实现 HubStream，用于驱动 sseAdapter。
type fakeStream struct {
	frames chan HubStreamFrame
	done   chan struct{}
	err    error
	closed bool
}

func newFakeStream() *fakeStream {
	return &fakeStream{
		frames: make(chan HubStreamFrame, 8),
		done:   make(chan struct{}),
	}
}

func (f *fakeStream) Frames() <-chan HubStreamFrame { return f.frames }
func (f *fakeStream) Done() <-chan struct{}         { return f.done }
func (f *fakeStream) Err() error                    { return f.err }
func (f *fakeStream) Close() {
	if f.closed {
		return
	}
	f.closed = true
	close(f.done)
	close(f.frames)
}
func (f *fakeStream) finish(err error) {
	f.err = err
	if !f.closed {
		f.closed = true
		close(f.done)
		close(f.frames)
	}
}

func TestSSEAdapter_LogFrames(t *testing.T) {
	fs := newFakeStream()
	for _, line := range []string{"hello", "world", "third"} {
		body, _ := json.Marshal(map[string]string{"line": line})
		fs.frames <- HubStreamFrame{Event: "log", Body: body}
	}
	go fs.finish(nil)

	a := newSSEAdapter(fs)
	defer a.Close()

	out, err := io.ReadAll(a)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	got := string(out)
	for _, want := range []string{"data: hello\n\n", "data: world\n\n", "data: third\n\n", "event: done"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in:\n%s", want, got)
		}
	}
}

func TestSSEAdapter_TracerouteHopFrames(t *testing.T) {
	fs := newFakeStream()
	hops := []string{`{"hop":1,"ip":"1.1.1.1"}`, `{"hop":2,"ip":"2.2.2.2"}`}
	for _, h := range hops {
		fs.frames <- HubStreamFrame{Event: "traceroute_hop", Body: json.RawMessage(h)}
	}
	go fs.finish(nil)

	a := newSSEAdapter(fs)
	defer a.Close()

	out, err := io.ReadAll(a)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	got := string(out)
	for _, h := range hops {
		want := "data: " + h + "\n\n"
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in:\n%s", want, got)
		}
	}
}

func TestSSEAdapter_ErrorTerminal(t *testing.T) {
	fs := newFakeStream()
	go fs.finish(errors.New("node failed"))

	a := newSSEAdapter(fs)
	defer a.Close()

	out, err := io.ReadAll(a)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	got := string(out)
	if !strings.Contains(got, "event: error") {
		t.Fatalf("missing error event in:\n%s", got)
	}
	if !strings.Contains(got, "node failed") {
		t.Fatalf("missing error message in:\n%s", got)
	}
}

func TestSSEAdapter_CloseStopsStream(t *testing.T) {
	fs := newFakeStream()
	a := newSSEAdapter(fs)
	if err := a.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !fs.closed {
		t.Fatal("underlying stream not closed")
	}
}
