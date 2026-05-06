package serverapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"pulse/internal/nodes"
)

func TestNodeLifecycleAndProxyEndpoints(t *testing.T) {
	hub := &fakeHub{handlers: map[string]func(body any) (json.RawMessage, error){
		"Runtime": func(any) (json.RawMessage, error) {
			return json.RawMessage(`{"available":true,"version":"v1.13.3"}`), nil
		},
		"Status": func(any) (json.RawMessage, error) {
			return json.RawMessage(`{"running":true}`), nil
		},
		"Start": func(b any) (json.RawMessage, error) {
			req, ok := b.(nodes.ConfigRequest)
			if !ok || req.Config == "" {
				return nil, &nodeAPIError{"bad request"}
			}
			return json.RawMessage(`{"running":true}`), nil
		},
	}}

	store := nodes.NewMemoryStore()
	api := New(store)
	api.clientFactory = fakeHubClientFactory(hub)
	mux := http.NewServeMux()
	api.Register(mux)

	upsertBody := []byte(`{"id":"node-1","name":"node 1","base_url":"http://node.test"}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/nodes", bytes.NewReader(upsertBody))
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("upsert node status = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/nodes", nil)
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list nodes status = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/nodes/node-1/runtime", nil)
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("runtime status = %d body=%s", rec.Code, rec.Body.String())
	}

	// runtime/usage 在 hub 模式下走按需 hub 拉取（fakeHub 默认返回 {}），应返回 200。
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/nodes/node-1/runtime/usage", nil)
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("usage status = %d body=%s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/nodes/node-1/runtime/start", bytes.NewReader([]byte(`{"config":"{}"}`)))
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("start status = %d body=%s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodDelete, "/v1/nodes/node-1", nil)
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete status = %d", rec.Code)
	}
}

// nodeAPIError 模拟 hub 调用失败的错误（如节点端 4xx 响应）。
type nodeAPIError struct{ msg string }

func (e *nodeAPIError) Error() string { return e.msg }

func TestCreateNodeAutoGeneratesID(t *testing.T) {
	store := nodes.NewMemoryStore()
	api := New(store)
	mux := http.NewServeMux()
	api.Register(mux)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/nodes", bytes.NewReader([]byte(`{"name":"node 1","base_url":"http://node.test"}`)))
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("create node status = %d body=%s", rec.Code, rec.Body.String())
	}

	var out nodes.Node
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode node: %v", err)
	}
	if out.ID == "" {
		t.Fatal("expected generated node id")
	}
}
