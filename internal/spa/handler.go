// Package spa serves a single-page application from an embedded filesystem.
// It serves static files when they exist, and falls back to index.html
// for client-side routing.
package spa

import (
	"compress/gzip"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"path/filepath"
	"strings"
	"sync"
)

// gzip writer pool 复用，避免频繁分配
var gzPool = sync.Pool{
	New: func() any { w, _ := gzip.NewWriterLevel(io.Discard, gzip.BestSpeed); return w },
}

// Handler serves an embedded SPA. Static assets are served directly;
// all other paths receive index.html so the frontend router can handle them.
type Handler struct {
	fsys  fs.FS
	index []byte
}

// New creates a Handler from an fs.FS rooted at the SPA dist directory.
// The FS must contain index.html at its root.
func New(fsys fs.FS) (*Handler, error) {
	index, err := fs.ReadFile(fsys, "index.html")
	if err != nil {
		return nil, err
	}
	return &Handler{fsys: fsys, index: index}, nil
}

// acceptsGzip 判断客户端是否接受 gzip 编码
func acceptsGzip(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept-Encoding"), "gzip")
}

// serve 将 src 写入 w，根据客户端能力决定是否 gzip 压缩
func serve(w http.ResponseWriter, r *http.Request, src io.Reader) {
	if !acceptsGzip(r) {
		io.Copy(w, src)
		return
	}
	w.Header().Set("Content-Encoding", "gzip")
	gz := gzPool.Get().(*gzip.Writer)
	gz.Reset(w)
	defer func() {
		gz.Close()
		gzPool.Put(gz)
	}()
	io.Copy(gz, src)
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Clean URL path
	p := path.Clean(r.URL.Path)
	if p == "." {
		p = ""
	}
	p = strings.TrimPrefix(p, "/")

	// API paths must be handled by registered routes; do not fall through to SPA.
	// If we're here it means no route matched — return 404 instead of serving
	// index.html (which would be misinterpreted as a 200 success by API clients).
	if strings.HasPrefix(r.URL.Path, "/v1/") || strings.HasPrefix(r.URL.Path, "/api/") {
		http.NotFound(w, r)
		return
	}

	// Try to open the file if the path has an extension (static asset)
	if p != "" && filepath.Ext(p) != "" {
		f, err := h.fsys.Open(p)
		if err == nil {
			defer f.Close()
			stat, _ := f.Stat()
			if stat != nil && !stat.IsDir() {
				ct := mime.TypeByExtension(filepath.Ext(p))
				if ct != "" {
					w.Header().Set("Content-Type", ct)
				}
				// Cache static assets aggressively (they have content hashes)
				if p != "index.html" {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				serve(w, r, f)
				return
			}
		}
	}

	// Fallback: serve index.html for SPA client-side routing (GET/HEAD only)
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	serve(w, r, strings.NewReader(string(h.index)))
}
