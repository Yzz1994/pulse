package ipsentinel

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
)

type NodeHandler struct {
	runner   *Runner
	dataDir  string
	cfgMu    sync.RWMutex
	cfgCache *Config
	ctx      context.Context
	cancel   context.CancelFunc
}

func NewNodeHandler(dataDir string) *NodeHandler {
	ctx, cancel := context.WithCancel(context.Background())
	return &NodeHandler{
		runner:  NewRunner(),
		dataDir: dataDir,
		ctx:     ctx,
		cancel:  cancel,
	}
}

func (h *NodeHandler) configFilePath() string {
	return filepath.Join(h.dataDir, "ip_sentinel_config.json")
}

func (h *NodeHandler) loadConfig() (Config, error) {
	h.cfgMu.RLock()
	if h.cfgCache != nil {
		cfg := *h.cfgCache
		h.cfgMu.RUnlock()
		return cfg, nil
	}
	h.cfgMu.RUnlock()

	h.cfgMu.Lock()
	defer h.cfgMu.Unlock()

	// 二次检查，防止并发写入
	if h.cfgCache != nil {
		return *h.cfgCache, nil
	}

	data, err := os.ReadFile(h.configFilePath())
	if os.IsNotExist(err) {
		return Config{
			LangParams:     "hl=en&gl=US",
			ValidURLSuffix: "com",
		}, nil
	}
	if err != nil {
		return Config{}, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	cfgCopy := cfg
	h.cfgCache = &cfgCopy
	return cfg, nil
}

func (h *NodeHandler) saveConfig(cfg Config) error {
	if err := os.MkdirAll(h.dataDir, 0o750); err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(h.configFilePath(), data, 0o600); err != nil {
		return err
	}

	h.cfgMu.Lock()
	cfgCopy := cfg
	h.cfgCache = &cfgCopy
	h.cfgMu.Unlock()
	return nil
}

func (h *NodeHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/node/ip-sentinel/detect", h.handleDetect)
	mux.HandleFunc("POST /v1/node/ip-sentinel/detect-google", h.handleDetectGoogle)
	mux.HandleFunc("POST /v1/node/ip-sentinel/run", h.handleRun)
	mux.HandleFunc("GET /v1/node/ip-sentinel/status", h.handleStatus)
	mux.HandleFunc("GET /v1/node/ip-sentinel/config", h.handleGetConfig)
	mux.HandleFunc("PUT /v1/node/ip-sentinel/config", h.handleSetConfig)
}

// Dispatch 提供给 nodeagent.APIDispatcher 复用的非 HTTP 入口。
// method 取值：IPSentinelDetect / IPSentinelDetectGoogle / IPSentinelRun /
// IPSentinelStatus / IPSentinelGetConfig / IPSentinelSetConfig。
// 未识别 method 返回 (nil, error)。
func (h *NodeHandler) Dispatch(ctx context.Context, method string, body []byte) (any, error) {
	switch method {
	case "IPSentinelDetect":
		return Detect(ctx)
	case "IPSentinelDetectGoogle":
		cfg, err := h.loadConfig()
		if err != nil {
			cfg = Config{ValidURLSuffix: "com"}
		}
		return DetectGoogle(ctx, cfg)
	case "IPSentinelRun":
		var req struct {
			Type     string `json:"type"`
			TaskType string `json:"task_type"`
		}
		if len(body) > 0 {
			_ = json.Unmarshal(body, &req)
		}
		taskType := req.Type
		if taskType == "" {
			taskType = req.TaskType
		}
		if taskType == "" {
			taskType = "auto"
		}
		cfg, err := h.loadConfig()
		if err != nil {
			return nil, err
		}
		if h.runner.IsRunning() {
			return map[string]any{"ok": false, "running": true, "message": "已有任务正在运行"}, nil
		}
		go h.runner.Run(h.ctx, cfg, taskType)
		return map[string]any{"ok": true, "running": true}, nil
	case "IPSentinelStatus":
		return NodeStatus{Running: h.runner.IsRunning(), Last: h.runner.GetLastResult()}, nil
	case "IPSentinelGetConfig":
		return h.loadConfig()
	case "IPSentinelSetConfig":
		var cfg Config
		if err := json.Unmarshal(body, &cfg); err != nil {
			return nil, err
		}
		if err := h.saveConfig(cfg); err != nil {
			return nil, err
		}
		return map[string]any{"ok": true}, nil
	}
	return nil, fmt.Errorf("ipsentinel: unknown method %q", method)
}

func (h *NodeHandler) handleDetect(w http.ResponseWriter, r *http.Request) {
	result, err := Detect(r.Context())
	if err != nil {
		writeNodeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeNodeJSON(w, http.StatusOK, result)
}

func (h *NodeHandler) handleDetectGoogle(w http.ResponseWriter, r *http.Request) {
	cfg, err := h.loadConfig()
	if err != nil {
		cfg = Config{ValidURLSuffix: "com"}
	}
	result, err := DetectGoogle(r.Context(), cfg)
	if err != nil {
		writeNodeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeNodeJSON(w, http.StatusOK, result)
}

func (h *NodeHandler) handleRun(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Type string `json:"type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Type == "" {
		body.Type = "auto"
	}

	cfg, err := h.loadConfig()
	if err != nil {
		writeNodeJSON(w, http.StatusInternalServerError, map[string]any{"error": "读取配置失败: " + err.Error()})
		return
	}

	if h.runner.IsRunning() {
		writeNodeJSON(w, http.StatusOK, map[string]any{"ok": false, "running": true, "message": "已有任务正在运行"})
		return
	}

	taskType := body.Type
	go h.runner.Run(h.ctx, cfg, taskType)

	writeNodeJSON(w, http.StatusOK, map[string]any{"ok": true, "running": true})
}

func (h *NodeHandler) handleStatus(w http.ResponseWriter, r *http.Request) {
	status := NodeStatus{
		Running: h.runner.IsRunning(),
		Last:    h.runner.GetLastResult(),
	}
	writeNodeJSON(w, http.StatusOK, status)
}

func (h *NodeHandler) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := h.loadConfig()
	if err != nil {
		writeNodeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeNodeJSON(w, http.StatusOK, cfg)
}

func (h *NodeHandler) handleSetConfig(w http.ResponseWriter, r *http.Request) {
	var cfg Config
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeNodeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	if err := h.saveConfig(cfg); err != nil {
		writeNodeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeNodeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func writeNodeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(payload) //nolint:errcheck
}
