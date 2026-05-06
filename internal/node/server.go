package node

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"pulse/internal/cert"
	"pulse/internal/config"
	"pulse/internal/ipsentinel"
	"pulse/internal/nodeapi"
	"pulse/internal/sniproxy"
	"pulse/internal/xray"
)

// xrayConfigPath 根据配置计算 xray 快照文件路径。
func xrayConfigPath(cfg config.Config) string {
	if cfg.NodeTLSKeyFile != "" {
		return filepath.Join(filepath.Dir(cfg.NodeTLSKeyFile), "xray_last.json")
	}
	return "./xray_last.json"
}

// sniproxyStatePath 根据配置计算 sniproxy 持久化文件路径。
func sniproxyStatePath(cfg config.Config) string {
	if cfg.NodeTLSKeyFile != "" {
		return filepath.Join(filepath.Dir(cfg.NodeTLSKeyFile), "sniproxy_state.json")
	}
	return "./sniproxy_state.json"
}

func Run() error {
	cfg := config.Load()

	xrManager := xray.NewManager(xrayConfigPath(cfg))

	xrInfo := xrManager.RuntimeInfo(context.Background())

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		xi := xrManager.RuntimeInfo(r.Context())
		status := "ok"
		if !xi.Available {
			status = "degraded"
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"service": "pulse-node",
			"status":  status,
			"role":    "node-plane",
		})
	})
	mux.HandleFunc("/v1/node/info", func(w http.ResponseWriter, r *http.Request) {
		xi := xrManager.RuntimeInfo(r.Context())
		writeJSON(w, http.StatusOK, map[string]any{
			"name":        "pulse-node",
			"description": "pulse node runtime",
			"addr":        cfg.NodeAddr,
			"xray":        xi,
		})
	})

	sniMgr := sniproxy.NewManager(sniproxyStatePath(cfg))
	if err := sniMgr.Restore(); err != nil {
		log.Printf("pulse-node sniproxy restore: %v", err)
	} else if restored := sniMgr.Config(); restored.Listen != "" {
		log.Printf("pulse-node sniproxy restored: listen=%s routes=%d", restored.Listen, len(restored.Routes))
	}

	nodeapi.New(xrManager).WithSNIManager(sniMgr).Register(mux)
	ipsentinel.NewNodeHandler(cfg.DataDir).Register(mux)

	tlsConfig, err := buildTLSConfig(cfg)
	if err != nil {
		return err
	}

	srv := &http.Server{
		Addr:              cfg.NodeAddr,
		Handler:           mux,
		TLSConfig:         tlsConfig,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      30 * time.Second, // 节点无 SSE，限制慢客户端占用
		IdleTimeout:       120 * time.Second,
	}

	if xrInfo.Available {
		log.Printf("pulse-node xray: %s %s", xrInfo.Module, xrInfo.Version)
	}

	// 进程重启后自动恢复：若磁盘上有上次的配置则直接启动
	if saved := xrManager.SavedConfig(); saved != "" {
		log.Printf("pulse-node restoring xray from saved config")
		if err := xrManager.Start(saved); err != nil {
			log.Printf("pulse-node restore xray failed: %v", err)
		} else {
			log.Printf("pulse-node xray auto-started")
		}
	}

	log.Printf("pulse-node listening on %s", cfg.NodeAddr)
	err = srv.ListenAndServeTLS("", "")
	if err == nil || err == http.ErrServerClosed {
		return shutdown(srv)
	}

	return err
}

func buildTLSConfig(cfg config.Config) (*tls.Config, error) {
	if err := cert.EnsureSelfSignedKeyPair(cfg.NodeTLSCertFile, cfg.NodeTLSKeyFile, "pulse-node"); err != nil {
		return nil, err
	}
	if cfg.NodeTLSClientCertFile == "" {
		return nil, fmt.Errorf("PULSE_NODE_TLS_CLIENT_CERT_FILE is required")
	}

	certPair, err := tls.LoadX509KeyPair(cfg.NodeTLSCertFile, cfg.NodeTLSKeyFile)
	if err != nil {
		return nil, err
	}
	clientPEM, err := os.ReadFile(cfg.NodeTLSClientCertFile)
	if err != nil {
		return nil, err
	}
	clientPool := x509.NewCertPool()
	if !clientPool.AppendCertsFromPEM(clientPEM) {
		return nil, fmt.Errorf("parse client certificate")
	}

	return &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{certPair},
		ClientCAs:    clientPool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
	}, nil
}

func shutdown(srv *http.Server) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return srv.Shutdown(ctx)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
