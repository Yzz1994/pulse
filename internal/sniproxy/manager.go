package sniproxy

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"pulse/internal/certmgr"
)

// ManagerConfig 是 pulse-server 下发给 pulse-node 的完整 SNI 代理配置。
// 它也是磁盘持久化格式，进程重启后节点会加载此文件恢复运行状态。
type ManagerConfig struct {
	// Listen 代理监听地址，如 ":443"。空值表示未启用代理。
	Listen string `json:"listen"`

	// CertStoragePath 证书落盘目录；为空时 Manager.Start 返回错误（只要有 Terminating 路由）。
	CertStoragePath string `json:"cert_storage_path"`

	// ACMEEmail 发给 Let's Encrypt 的账户邮箱（必填，用于证书到期提醒）。
	ACMEEmail string `json:"acme_email"`

	// CloudflareToken Cloudflare API Token，DNS-01 challenge 用。
	CloudflareToken string `json:"cloudflare_token"`

	// ACMEStaging 为 true 时使用 Let's Encrypt staging，用于测试。
	ACMEStaging bool `json:"acme_staging"`

	// Routes SNI → 后端的路由表。
	Routes []Route `json:"routes"`

	// CertDomains 仅用于驱动 certmgr 申请证书但不参与 UnifiedProxy 路由的额外域名。
	// 例如 hy2 inbound 的 SNI：UDP/QUIC 由 Xray 自己持证，但证书仍走节点 ACME 通道。
	CertDomains []string `json:"cert_domains,omitempty"`
}

// Manager 是 pulse-node 上的 SNI 代理运行实例：
// 持有 UnifiedProxy + certmgr.Manager，管理生命周期和磁盘持久化。
//
// 典型使用方式：
//
//	m := sniproxy.NewManager(statePath)
//	m.Restore()               // 进程启动时恢复上次配置
//	m.Apply(newCfg)           // 收到 server 推送时更新
//	m.Close()                 // 退出时清理
type Manager struct {
	statePath string

	mu        sync.Mutex
	cfg       ManagerConfig
	proxy     *UnifiedProxy
	serveErr  chan error // Serve 返回时 goroutine 写入，非阻塞读以探活
	certs     *certmgr.Manager
	ctx       context.Context
	cancel    context.CancelFunc
	lastError string // Serve 最近一次失败原因，用于 /sniproxy/status 暴露
}

// checkLivenessLocked 非阻塞检查 Serve 是否意外退出。
// 调用者必须已持有 m.mu。Serve 仍在运行时无副作用；已退出时清理 proxy 状态。
func (m *Manager) checkLivenessLocked() {
	if m.proxy == nil || m.serveErr == nil {
		return
	}
	select {
	case err, ok := <-m.serveErr:
		// Serve 已退出
		m.proxy = nil
		m.serveErr = nil
		if ok && err != nil {
			m.lastError = err.Error()
		}
	default:
		// 仍在跑
	}
}

// Status 描述 Manager 当前运行状态。
type Status struct {
	Listen      string             `json:"listen"`                 // 当前监听地址，空 = 未运行
	RouteCount  int                `json:"route_count"`            // 路由数
	CertDomains int                `json:"cert_domains"`           // certmgr 管理的域名数
	LastError   string             `json:"last_error,omitempty"`   // Serve 最近一次失败信息（如 bind EADDRINUSE）
	Routes      []Route            `json:"routes,omitempty"`       // 路由明细（当前生效的 SNI → Backend 映射）
	Certs       []certmgr.CertInfo `json:"certs,omitempty"`        // 证书明细（每个域名的 NotAfter 等）
	StoragePath string             `json:"storage_path,omitempty"` // 证书落盘目录，便于运维 ls/openssl 查看
}

// Status 返回当前运行状态（线程安全）。
// 读取前会主动探活一次 Serve goroutine，避免把"已崩溃但状态未清理"的 proxy
// 误报为 running。
func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.checkLivenessLocked()
	s := Status{
		Listen:      m.cfg.Listen,
		RouteCount:  len(m.cfg.Routes),
		LastError:   m.lastError,
		Routes:      append([]Route(nil), m.cfg.Routes...),
		StoragePath: m.cfg.CertStoragePath,
	}
	if m.certs != nil {
		certs := m.certs.CertInfos()
		s.CertDomains = len(certs)
		s.Certs = certs
	}
	// proxy 为 nil 说明 Serve 失败或已停，Listen 真实生效状态应清空
	if m.proxy == nil {
		s.Listen = ""
	}
	return s
}

// NewManager 构造 Manager，statePath 是持久化 ManagerConfig 的 JSON 文件路径。
func NewManager(statePath string) *Manager {
	return &Manager{statePath: statePath}
}

// Restore 从磁盘读取上次 Apply 的配置并启动。如无文件或文件损坏则静默跳过，
// 等待 server 第一次推送。
func (m *Manager) Restore() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.statePath == "" {
		return nil
	}
	data, err := os.ReadFile(m.statePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read state: %w", err)
	}
	var cfg ManagerConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		log.Printf("sniproxy: state file corrupted, ignoring: %v", err)
		return nil
	}
	return m.applyLocked(cfg, false)
}

// Apply 启动或热更新到新配置，并把配置持久化到磁盘。
// 相同配置重复调用是幂等的。
func (m *Manager) Apply(cfg ManagerConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.applyLocked(cfg, true)
}

// Config 返回当前生效的配置（快照）。
func (m *Manager) Config() ManagerConfig {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cfg
}

// Close 停止代理和证书续期，清理资源。
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopLocked()
	return nil
}

func (m *Manager) applyLocked(cfg ManagerConfig, persist bool) error {
	// 先探活：如果上一次 Serve 已经异常退出（如端口被占），把 m.proxy 清掉
	// 让接下来走重启分支而不是"往死的 proxy 热更新"。
	m.checkLivenessLocked()

	// 情况 1：无 TCP 路由且无 CertDomains = 完全停止；否则即使没有 TCP 路由
	// 也需要保留 certmgr 来管理 hy2 等"仅证书"域名。
	if (cfg.Listen == "" || len(cfg.Routes) == 0) && len(cfg.CertDomains) == 0 {
		m.stopLocked()
		m.cfg = cfg
		if persist {
			_ = m.persistLocked()
		}
		return nil
	}

	// 仅 CertDomains 非空、无 TCP 路由：停掉 UnifiedProxy 但保留/启动 certmgr
	certOnly := cfg.Listen == "" || len(cfg.Routes) == 0
	if certOnly {
		m.stopProxyLocked()
	}

	// 情况 2：Listen 地址变化或 proxy 不存在 = 重启
	needRestart := m.proxy == nil || m.cfg.Listen != cfg.Listen

	// 情况 3：证书相关字段变化 = 重启 certmgr（ACME 账户 key 可能变）
	needCertRestart := m.certs == nil ||
		m.cfg.CloudflareToken != cfg.CloudflareToken ||
		m.cfg.ACMEEmail != cfg.ACMEEmail ||
		m.cfg.CertStoragePath != cfg.CertStoragePath ||
		m.cfg.ACMEStaging != cfg.ACMEStaging

	// 重建 certmgr（Terminating/HTTPReverse 路由 或 CertDomains 列表非空时需要）
	hasCertRoutes := len(cfg.CertDomains) > 0
	for _, r := range cfg.Routes {
		if r.Mode == ModeTerminating || r.Mode == ModeHTTPReverse {
			hasCertRoutes = true
			break
		}
	}
	needCerts := hasCertRoutes

	if needCerts && needCertRestart {
		if m.certs != nil {
			m.certs.Close()
			m.certs = nil
		}
		if cfg.CloudflareToken == "" {
			return fmt.Errorf("sniproxy: cloudflare_token required when certs are needed (terminating routes)")
		}
		if cfg.CertStoragePath == "" {
			return fmt.Errorf("sniproxy: cert_storage_path required when certs are needed (terminating routes)")
		}
		c, err := certmgr.New(certmgr.Config{
			StoragePath:        cfg.CertStoragePath,
			Email:              cfg.ACMEEmail,
			CloudflareAPIToken: cfg.CloudflareToken,
			Staging:            cfg.ACMEStaging,
		})
		if err != nil {
			return fmt.Errorf("new certmgr: %w", err)
		}
		m.certs = c
	} else if !needCerts && m.certs != nil {
		m.certs.Close()
		m.certs = nil
	}

	// 同步 certmgr 管理的域名：terminating/http-reverse 路由 SNI + CertDomains 额外域名
	if m.certs != nil {
		seen := make(map[string]struct{})
		var domains []string
		for _, r := range cfg.Routes {
			if (r.Mode == ModeTerminating || r.Mode == ModeHTTPReverse) && r.SNI != "" {
				if _, dup := seen[r.SNI]; !dup {
					seen[r.SNI] = struct{}{}
					domains = append(domains, r.SNI)
				}
			}
		}
		for _, d := range cfg.CertDomains {
			if d == "" {
				continue
			}
			if _, dup := seen[d]; !dup {
				seen[d] = struct{}{}
				domains = append(domains, d)
			}
		}
		if err := m.certs.Replace(domains); err != nil {
			log.Printf("sniproxy: certmgr replace: %v", err)
		}
	}

	// 启动或重启 proxy（cert-only 模式跳过：UnifiedProxy 已在前面 stopProxyLocked）
	if certOnly {
		// 仅持久化新配置，不启动 proxy
	} else if needRestart {
		m.stopProxyLocked()
		proxy := &UnifiedProxy{
			Addr:    cfg.Listen,
			Started: make(chan struct{}),
		}
		if m.certs != nil {
			proxy.SetTLSConfig(m.certs.TLSConfig())
		}
		proxy.SetRoutes(cfg.Routes)

		ctx, cancel := context.WithCancel(context.Background())
		serveErr := make(chan error, 1)
		go func() {
			err := proxy.Serve(ctx)
			serveErr <- err
			close(serveErr)
			if err != nil {
				log.Printf("sniproxy: serve: %v", err)
			}
		}()

		// 同步等监听就绪（或立即失败）：Started 在 Listen 成功后关闭，
		// 或 Serve 异常返回时 defer 关闭。
		<-proxy.Started

		// 非阻塞检查：监听是否立即失败
		select {
		case err := <-serveErr:
			cancel()
			if err != nil {
				m.lastError = err.Error()
				return fmt.Errorf("sniproxy: %w", err)
			}
			// err == nil 但 Serve 已返回不应发生；保险起见视为失败
			return fmt.Errorf("sniproxy: serve exited unexpectedly")
		default:
			// Listen 成功，Serve 正在跑 → 提交状态
		}

		m.proxy = proxy
		m.ctx = ctx
		m.cancel = cancel
		m.serveErr = serveErr
		m.lastError = ""
	} else {
		// 只更新路由（热更新）
		if m.certs != nil {
			m.proxy.SetTLSConfig(m.certs.TLSConfig())
		} else {
			m.proxy.SetTLSConfig(nil)
		}
		m.proxy.SetRoutes(cfg.Routes)
	}

	m.cfg = cfg
	if persist {
		if err := m.persistLocked(); err != nil {
			log.Printf("sniproxy: persist state: %v", err)
		}
	}
	return nil
}

func (m *Manager) stopLocked() {
	m.stopProxyLocked()
	if m.certs != nil {
		m.certs.Close()
		m.certs = nil
	}
}

func (m *Manager) stopProxyLocked() {
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	if m.proxy != nil {
		_ = m.proxy.Close()
		m.proxy = nil
	}
	// proxy.Close() 关闭 listener，Serve goroutine 会很快退出并写入 serveErr（buf=1 不阻塞）。
	// 短暂等待确保端口释放，避免立即重启时 EADDRINUSE。
	if m.serveErr != nil {
		select {
		case <-m.serveErr:
		case <-time.After(200 * time.Millisecond):
		}
		m.serveErr = nil
	}
}

func (m *Manager) persistLocked() error {
	if m.statePath == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(m.statePath), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m.cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.statePath, data, 0o600)
}
