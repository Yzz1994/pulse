package serverapi

import (
	"io"
	"net/http"
)

// bridgeSSE forwards an upstream byte stream to an SSE response writer.
// It guarantees that when the HTTP client disconnects (r.Context() done),
// the upstream ReadCloser is closed promptly so any backing resources
// (e.g. nodehub streams) can release their goroutines.
func bridgeSSE(w http.ResponseWriter, r *http.Request, upstream io.ReadCloser) {
	defer upstream.Close()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	flusher, _ := w.(http.Flusher)
	if flusher != nil {
		flusher.Flush()
	}

	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-r.Context().Done():
			// Triggers stream.Close on the underlying hub stream,
			// which sends cancel_id upstream and unblocks Read below.
			_ = upstream.Close()
		case <-done:
		}
	}()

	buf := make([]byte, 4096)
	for {
		n, readErr := upstream.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
		if readErr != nil {
			return
		}
	}
}
