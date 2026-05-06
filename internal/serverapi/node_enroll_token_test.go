package serverapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"pulse/internal/enrolltokens"
	"pulse/internal/nodes"
)

func newEnrollTokenTestMux(t *testing.T, store *enrolltokens.MemoryStore, nodeIDs ...string) *http.ServeMux {
	t.Helper()
	ns := nodes.NewMemoryStore()
	for _, id := range nodeIDs {
		if _, err := ns.Upsert(nodes.Node{ID: id, Name: id}); err != nil {
			t.Fatalf("seed node %q: %v", id, err)
		}
	}
	mux := http.NewServeMux()
	RegisterNodeEnrollTokenEndpoint(mux, ns, store, func(*http.Request) string {
		return "https://panel.example.com"
	})
	return mux
}

func doEnrollToken(t *testing.T, mux *http.ServeMux, nodeID string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	path := "/v1/nodes/" + nodeID + "/enroll-token"
	if body == nil {
		r = httptest.NewRequest(http.MethodPost, path, nil)
	} else {
		buf, _ := json.Marshal(body)
		r = httptest.NewRequest(http.MethodPost, path, bytes.NewReader(buf))
		r.Header.Set("Content-Type", "application/json")
	}
	r.Host = "panel.example.com"
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	return w
}

func TestNodeEnrollToken_DefaultTTL(t *testing.T) {
	store := enrolltokens.NewMemoryStore()
	mux := newEnrollTokenTestMux(t, store, "node-1")

	w := doEnrollToken(t, mux, "node-1", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var resp enrollTokenResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Token) != 64 {
		t.Errorf("token length=%d, want 64 hex chars", len(resp.Token))
	}
	delta := time.Until(resp.ExpiresAt)
	if delta < 55*time.Minute || delta > 65*time.Minute {
		t.Errorf("expires_at delta=%s, want ~1h", delta)
	}
	if !strings.Contains(resp.InstallCommand, resp.Token) || !strings.Contains(resp.InstallCommand, "node-1") {
		t.Errorf("install_command missing fields: %s", resp.InstallCommand)
	}
	if !strings.Contains(resp.InstallCommand, "https://panel.example.com") {
		t.Errorf("install_command missing server url: %s", resp.InstallCommand)
	}
	if !strings.Contains(resp.ManualCommand, "pulse-node enroll") || !strings.Contains(resp.ManualCommand, resp.Token) {
		t.Errorf("manual_command malformed: %s", resp.ManualCommand)
	}
	// Token must be persisted and usable.
	got, err := store.Get(context.Background(), resp.Token)
	if err != nil {
		t.Fatalf("token not stored: %v", err)
	}
	if got.NodeID != "node-1" {
		t.Errorf("stored node_id=%q", got.NodeID)
	}
}

func TestNodeEnrollToken_CustomTTL(t *testing.T) {
	store := enrolltokens.NewMemoryStore()
	mux := newEnrollTokenTestMux(t, store, "node-1")

	w := doEnrollToken(t, mux, "node-1", enrollTokenRequest{TTLSeconds: 120})
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var resp enrollTokenResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	delta := time.Until(resp.ExpiresAt)
	if delta < 110*time.Second || delta > 130*time.Second {
		t.Errorf("expires_at delta=%s, want ~120s", delta)
	}
}

func TestNodeEnrollToken_TTLClamped(t *testing.T) {
	store := enrolltokens.NewMemoryStore()
	mux := newEnrollTokenTestMux(t, store, "node-1")

	// 7 day request → clamped to maxEnrollTokenTTL (24h).
	w := doEnrollToken(t, mux, "node-1", enrollTokenRequest{TTLSeconds: 7 * 86400})
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d", w.Code)
	}
	var resp enrollTokenResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	delta := time.Until(resp.ExpiresAt)
	if delta > maxEnrollTokenTTL+time.Minute || delta < maxEnrollTokenTTL-time.Minute {
		t.Errorf("expires_at delta=%s, want ~%s", delta, maxEnrollTokenTTL)
	}
}

func TestNodeEnrollToken_NotFound(t *testing.T) {
	store := enrolltokens.NewMemoryStore()
	mux := newEnrollTokenTestMux(t, store) // no nodes seeded

	w := doEnrollToken(t, mux, "ghost", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestNodeEnrollToken_InvalidJSON(t *testing.T) {
	store := enrolltokens.NewMemoryStore()
	mux := newEnrollTokenTestMux(t, store, "node-1")

	r := httptest.NewRequest(http.MethodPost, "/v1/nodes/node-1/enroll-token", strings.NewReader("{not json"))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", w.Code)
	}
}

func TestNodeEnrollToken_ServerOverrideQuery(t *testing.T) {
	store := enrolltokens.NewMemoryStore()
	mux := newEnrollTokenTestMux(t, store, "node-1")

	r := httptest.NewRequest(http.MethodPost, "/v1/nodes/node-1/enroll-token?server=https://override.test", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var resp enrollTokenResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.ServerURL != "https://override.test" {
		t.Errorf("server_url=%q", resp.ServerURL)
	}
	if !strings.Contains(resp.InstallCommand, "https://override.test") {
		t.Errorf("install_command does not honor query override: %s", resp.InstallCommand)
	}
}

func TestNodeEnrollToken_IssuesFreshTokenEachCall(t *testing.T) {
	store := enrolltokens.NewMemoryStore()
	mux := newEnrollTokenTestMux(t, store, "node-1")

	w1 := doEnrollToken(t, mux, "node-1", nil)
	w2 := doEnrollToken(t, mux, "node-1", nil)
	if w1.Code != http.StatusOK || w2.Code != http.StatusOK {
		t.Fatalf("statuses %d,%d", w1.Code, w2.Code)
	}
	var r1, r2 enrollTokenResponse
	_ = json.Unmarshal(w1.Body.Bytes(), &r1)
	_ = json.Unmarshal(w2.Body.Bytes(), &r2)
	if r1.Token == r2.Token {
		t.Fatalf("expected distinct tokens, got %q twice", r1.Token)
	}
	// Old token remains valid until expiry — task spec ("发新的，旧的留过期").
	if _, err := store.Get(context.Background(), r1.Token); err != nil {
		t.Errorf("first token should still be retrievable: %v", err)
	}
}
