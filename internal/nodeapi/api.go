package nodeapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"time"

	"pulse/internal/buildinfo"
	"pulse/internal/coremanager"
	"pulse/internal/sniproxy"
)

type API struct {
	xrManager  coremanager.Manager // xray Manager
	sniManager *sniproxy.Manager   // 可选：若节点启用了内置 SNI 代理则非空
}

type configRequest struct {
	Config string `json:"config"`
	Core   string `json:"core,omitempty"` // 保留兼容，当前只使用 "xray"
}

func New(xrManager coremanager.Manager) *API {
	return &API{xrManager: xrManager}
}

// WithSNIManager 挂入 SNI 代理 Manager，使节点支持 /v1/node/sniproxy/* 接口。
// 传 nil 则相关接口返回 503。
func (a *API) WithSNIManager(m *sniproxy.Manager) *API {
	a.sniManager = m
	return a
}

// managerFor 返回 xray Manager（当前唯一支持的核心）。
func (a *API) managerFor(_ string) coremanager.Manager {
	return a.xrManager
}

// activeManager 返回 xray Manager。
func (a *API) activeManager() coremanager.Manager {
	return a.xrManager
}

func (a *API) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/node/check", a.handleCheck)
	mux.HandleFunc("GET /v1/node/speedtest", a.handleSpeedTest)
	mux.HandleFunc("GET /v1/node/traceroute", a.handleTraceroute)
	mux.HandleFunc("GET /v1/node/latency/probe", a.handleLatencyProbe)
	mux.HandleFunc("POST /v1/node/sniproxy/sync", a.handleSNIProxySync)
	mux.HandleFunc("GET /v1/node/sniproxy/status", a.handleSNIProxyStatus)
	mux.HandleFunc("POST /v1/node/cert/ensure", a.handleCertEnsure)
	mux.HandleFunc("GET /v1/node/runtime", a.handleRuntime)
	mux.HandleFunc("GET /v1/node/runtime/status", a.handleStatus)
	mux.HandleFunc("GET /v1/node/runtime/usage", a.handleUsage)
	mux.HandleFunc("GET /v1/node/runtime/version", a.handleVersion)
	mux.HandleFunc("GET /v1/node/runtime/config", a.handleConfig)
	mux.HandleFunc("GET /v1/node/runtime/logs", a.handleLogs)
	mux.HandleFunc("GET /v1/node/runtime/logs/stream", a.handleLogsStream)
	mux.HandleFunc("GET /v1/node/runtime/accesslogs", a.handleAccessLogs)
	mux.HandleFunc("POST /v1/node/runtime/start", a.handleStart)
	mux.HandleFunc("POST /v1/node/runtime/stop", a.handleStop)
	mux.HandleFunc("POST /v1/node/runtime/restart", a.handleRestart)
	mux.HandleFunc("POST /v1/node/runtime/users/add", a.handleAddUser)
	mux.HandleFunc("POST /v1/node/runtime/users/remove", a.handleRemoveUser)
	mux.HandleFunc("POST /v1/node/update", a.handleUpdate)
}

func (a *API) handleUpdate(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "节点更新已开始，将在数秒后重启"})
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	go func() {
		time.Sleep(500 * time.Millisecond)
		cmd := exec.Command("sh", "-c",
			`curl -fsSL https://raw.githubusercontent.com/0xUnixIO/pulse/main/scripts/install.sh | sh -s -- node`,
		)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			log.Printf("node update: install script failed: %v", err)
		}
	}()
}

func (a *API) handleRuntime(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	mgr := a.activeManager()
	info := mgr.RuntimeInfo(ctx)
	writeJSON(w, http.StatusOK, map[string]any{
		"available":    info.Available,
		"module":       info.Module,
		"version":      info.Version,
		"last_error":   info.LastError,
		"node_version": buildinfo.Version,
	})
}

func (a *API) handleStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, a.activeManager().Status())
}

func (a *API) handleUsage(w http.ResponseWriter, r *http.Request) {
	reset := r.URL.Query().Get("reset") == "true"
	writeJSON(w, http.StatusOK, a.activeManager().Usage(reset))
}

func (a *API) handleVersion(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	version, err := a.activeManager().Version(ctx)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"version": version})
}

func (a *API) handleConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"config": a.activeManager().Config()})
}

func (a *API) handleLogs(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"logs": a.activeManager().Logs()})
}

func (a *API) handleAccessLogs(w http.ResponseWriter, r *http.Request) {
	drainer, ok := a.activeManager().(coremanager.AccessLogDrainer)
	if !ok {
		writeJSON(w, http.StatusOK, map[string]any{"entries": []any{}})
		return
	}
	entries := drainer.DrainAccessLogs()
	if entries == nil {
		entries = []coremanager.AccessLogEntry{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": entries})
}

func (a *API) handleLogsStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")

	mgr := a.activeManager()

	// 先把缓冲区里的历史日志全部发送
	for _, line := range mgr.Logs() {
		fmt.Fprintf(w, "data: %s\n\n", line)
	}
	flusher.Flush()

	// 订阅后续新日志
	id, ch := mgr.Subscribe()
	defer mgr.Unsubscribe(id)

	for {
		select {
		case <-r.Context().Done():
			return
		case line, ok := <-ch:
			if !ok {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", line)
			flusher.Flush()
		}
	}
}

func (a *API) handleStart(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeConfigRequest(w, r)
	if !ok {
		return
	}

	mgr := a.managerFor(req.Core)
	if err := mgr.Start(req.Config); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, mgr.Status())
}

func (a *API) handleStop(w http.ResponseWriter, r *http.Request) {
	xrErr := a.xrManager.Stop()
	if xrErr != nil && !errors.Is(xrErr, coremanager.ErrNotRunning) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": xrErr.Error()})
		return
	}
	writeJSON(w, http.StatusOK, a.xrManager.Status())
}

func (a *API) handleRestart(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeConfigRequest(w, r)
	if !ok {
		return
	}

	mgr := a.managerFor(req.Core)
	if err := mgr.Restart(req.Config); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, mgr.Status())
}

// handleAddUser 向正在运行的 inbound 热增单个用户。
func (a *API) handleAddUser(w http.ResponseWriter, r *http.Request) {
	var req coremanager.UserConfig
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	if req.InboundTag == "" || req.Email == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "inbound_tag and email are required"})
		return
	}
	mgr := a.activeManager()
	if err := mgr.AddUser(r.Context(), req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleRemoveUser 从正在运行的 inbound 热删单个用户。
func (a *API) handleRemoveUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		InboundTag string `json:"inbound_tag"`
		Email      string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	if req.InboundTag == "" || req.Email == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "inbound_tag and email are required"})
		return
	}
	mgr := a.activeManager()
	if err := mgr.RemoveUser(r.Context(), req.InboundTag, req.Email); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *API) handleCertEnsure(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}


func decodeConfigRequest(w http.ResponseWriter, r *http.Request) (configRequest, bool) {
	var req configRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json body"})
		return configRequest{}, false
	}
	if req.Config == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "config is required"})
		return configRequest{}, false
	}
	return req, true
}

func writeMethodNotAllowed(w http.ResponseWriter, allow string) {
	w.Header().Set("Allow", allow)
	writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
