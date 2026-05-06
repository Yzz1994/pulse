package nodes

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"pulse/internal/ipsentinel"
)

type Client struct {
	baseURL    string
	httpClient *http.Client
	initErr    error
}

type ClientOptions struct {
	ClientCertFile string
	ClientKeyFile  string
}

type RuntimeInfo struct {
	Available   bool   `json:"available"`
	Module      string `json:"module"`
	Version     string `json:"version,omitempty"`
	LastError   string `json:"last_error,omitempty"`
	NodeVersion string `json:"node_version,omitempty"`
}

type Status struct {
	Running   bool      `json:"running"`
	StartedAt time.Time `json:"started_at,omitempty"`
}

type LogsResponse struct {
	Logs []string `json:"logs"`
}

type AccessLogEntry struct {
	SourceIP    string    `json:"source_ip"`
	SourcePort  string    `json:"source_port"`
	Destination string    `json:"destination"`
	RemoteIP    string    `json:"remote_ip"`
	RouteTag    string    `json:"route_tag"`
	Protocol    string    `json:"protocol"`
	User        string    `json:"user"`
	InboundTag  string    `json:"inbound_tag"`
	Time        time.Time `json:"time"`
}

type AccessLogsResponse struct {
	Entries []AccessLogEntry `json:"entries"`
}

type UsageStats struct {
	Available     bool        `json:"available"`
	Running       bool        `json:"running"`
	StartedAt     time.Time   `json:"started_at,omitempty"`
	UploadTotal   int64       `json:"upload_total"`
	DownloadTotal int64       `json:"download_total"`
	UploadSpeed   int64       `json:"upload_speed"`   // bytes/s
	DownloadSpeed int64       `json:"download_speed"` // bytes/s
	Connections   int         `json:"connections"`
	Users         []UserUsage `json:"users"`
}

type UserUsage struct {
	User          string   `json:"user"`
	UploadTotal   int64    `json:"upload_total"`
	DownloadTotal int64    `json:"download_total"`
	Connections   int      `json:"connections"`
	Devices       int      `json:"devices"`
	SourceIPs     []string `json:"source_ips,omitempty"` // 原始源 IP 列表，用于跨节点精确去重
}

type ConfigRequest struct {
	Config string `json:"config"`
}

// UserChangeRequest 描述向节点热增或热删单个用户所需的凭证信息。
type UserChangeRequest struct {
	InboundTag string `json:"inbound_tag"`
	Protocol   string `json:"protocol"`            // "vless" | "trojan" | "shadowsocks" | "anytls"
	Email      string `json:"email"`               // username@@@tag
	UUID       string `json:"uuid,omitempty"`      // vless
	Password   string `json:"password,omitempty"`  // trojan / anytls
	Flow       string `json:"flow,omitempty"`      // vless+reality: "xtls-rprx-vision"
}

func NewClient(node Node, options ClientOptions) *Client {
	httpClient, err := buildHTTPClient(node, options)
	return &Client{
		baseURL:    strings.TrimRight(node.BaseURL, "/"),
		httpClient: httpClient,
		initErr:    err,
	}
}

func NewClientWithHTTPClient(baseURL string, httpClient *http.Client) *Client {
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: httpClient,
	}
}

// InitErr 返回客户端初始化时的错误（如 TLS 握手失败）。nil 表示可正常使用。
func (c *Client) InitErr() error { return c.initErr }

func buildHTTPClient(node Node, options ClientOptions) (*http.Client, error) {
	httpClient := &http.Client{
		Timeout: 5 * time.Second,
	}
	if !strings.HasPrefix(strings.TrimSpace(node.BaseURL), "https://") {
		return httpClient, nil
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}
	var clientPair *tls.Certificate
	if options.ClientCertFile != "" || options.ClientKeyFile != "" {
		if options.ClientCertFile == "" || options.ClientKeyFile == "" {
			return nil, fmt.Errorf("client cert file and key file must be configured together")
		}
		pair, err := tls.LoadX509KeyPair(options.ClientCertFile, options.ClientKeyFile)
		if err != nil {
			return nil, fmt.Errorf("load client key pair: %w", err)
		}
		clientPair = &pair
		tlsConfig.Certificates = []tls.Certificate{pair}
	}
	serverCert, err := fetchServerCertificatePEM(node.BaseURL, clientPair)
	if err != nil {
		return nil, err
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM([]byte(serverCert)) {
		return nil, fmt.Errorf("parse node certificate")
	}
	tlsConfig.InsecureSkipVerify = true
	tlsConfig.VerifyConnection = func(cs tls.ConnectionState) error {
		if len(cs.PeerCertificates) == 0 {
			return fmt.Errorf("node tls handshake did not present a certificate")
		}
		opts := x509.VerifyOptions{
			Roots:         roots,
			Intermediates: x509.NewCertPool(),
		}
		for _, cert := range cs.PeerCertificates[1:] {
			opts.Intermediates.AddCert(cert)
		}
		_, err := cs.PeerCertificates[0].Verify(opts)
		if err != nil {
			return fmt.Errorf("verify node certificate: %w", err)
		}
		return nil
	}
	transport.TLSClientConfig = tlsConfig
	httpClient.Transport = transport
	return httpClient, nil
}

func fetchServerCertificatePEM(rawURL string, clientPair *tls.Certificate) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("parse node url: %w", err)
	}
	address := parsed.Host
	if _, _, err := net.SplitHostPort(address); err != nil {
		address = net.JoinHostPort(parsed.Hostname(), "443")
	}
	tlsConfig := &tls.Config{
		InsecureSkipVerify: true,
		MinVersion:         tls.VersionTLS12,
	}
	if clientPair != nil {
		tlsConfig.Certificates = []tls.Certificate{*clientPair}
	}

	conn, err := tls.Dial("tcp", address, tlsConfig)
	if err != nil {
		return "", fmt.Errorf("dial node tls: %w", err)
	}
	defer conn.Close()

	state := conn.ConnectionState()
	if len(state.PeerCertificates) == 0 {
		return "", fmt.Errorf("node did not present a certificate")
	}
	pemBytes := pemEncodeCertificate(state.PeerCertificates[0].Raw)
	return string(pemBytes), nil
}

func pemEncodeCertificate(raw []byte) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: raw})
}

func (c *Client) Runtime(ctx context.Context) (RuntimeInfo, error) {
	var out RuntimeInfo
	err := c.do(ctx, http.MethodGet, "/v1/node/runtime", nil, &out)
	return out, err
}

func (c *Client) Status(ctx context.Context) (Status, error) {
	var out Status
	err := c.do(ctx, http.MethodGet, "/v1/node/runtime/status", nil, &out)
	return out, err
}

func (c *Client) Logs(ctx context.Context) (LogsResponse, error) {
	var out LogsResponse
	err := c.do(ctx, http.MethodGet, "/v1/node/runtime/logs", nil, &out)
	return out, err
}

func (c *Client) AccessLogs(ctx context.Context) (AccessLogsResponse, error) {
	var out AccessLogsResponse
	err := c.do(ctx, http.MethodGet, "/v1/node/runtime/accesslogs", nil, &out)
	return out, err
}

// LogsStream 打开节点的 SSE 日志流，返回 response body（调用方负责 Close）。
// 使用无超时的 HTTP 客户端，由 ctx 控制生命周期。
func (c *Client) LogsStream(ctx context.Context) (io.ReadCloser, error) {
	if c.initErr != nil {
		return nil, fmt.Errorf("configure node client: %w", c.initErr)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/node/runtime/logs/stream", nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	// 流式请求不能有超时，复用 transport 但去掉 Timeout
	sc := &http.Client{Transport: c.httpClient.Transport}
	resp, err := sc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request node: %w", err)
	}
	if resp.StatusCode >= 400 {
		resp.Body.Close()
		return nil, fmt.Errorf("node returned %d", resp.StatusCode)
	}
	return resp.Body, nil
}

type ConfigResponse struct {
	Config string `json:"config"`
}

func (c *Client) Config(ctx context.Context) (ConfigResponse, error) {
	var out ConfigResponse
	err := c.do(ctx, http.MethodGet, "/v1/node/runtime/config", nil, &out)
	return out, err
}

func (c *Client) Usage(ctx context.Context, reset bool) (UsageStats, error) {
	path := "/v1/node/runtime/usage"
	if reset {
		path += "?reset=true"
	}
	var out UsageStats
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

func (c *Client) Start(ctx context.Context, req ConfigRequest) (Status, error) {
	var out Status
	err := c.do(ctx, http.MethodPost, "/v1/node/runtime/start", req, &out)
	return out, err
}

func (c *Client) Stop(ctx context.Context) (Status, error) {
	var out Status
	err := c.do(ctx, http.MethodPost, "/v1/node/runtime/stop", nil, &out)
	return out, err
}

func (c *Client) Restart(ctx context.Context, req ConfigRequest) (Status, error) {
	var out Status
	err := c.do(ctx, http.MethodPost, "/v1/node/runtime/restart", req, &out)
	return out, err
}

// AddUser 向节点正在运行的 inbound 热增单个用户，无需重启核心。
func (c *Client) AddUser(ctx context.Context, req UserChangeRequest) error {
	return c.do(ctx, http.MethodPost, "/v1/node/runtime/users/add", req, nil)
}

// RemoveUser 从节点正在运行的 inbound 热删单个用户。
func (c *Client) RemoveUser(ctx context.Context, inboundTag, email string) error {
	return c.do(ctx, http.MethodPost, "/v1/node/runtime/users/remove",
		map[string]string{"inbound_tag": inboundTag, "email": email}, nil)
}

func (c *Client) Update(ctx context.Context) (map[string]any, error) {
	var out map[string]any
	err := c.do(ctx, http.MethodPost, "/v1/node/update", nil, &out)
	return out, err
}

type CertPaths struct {
	Domain   string `json:"domain"`
	CertPath string `json:"cert_path"`
	KeyPath  string `json:"key_path"`
}

func (c *Client) EnsureCert(ctx context.Context, domain, cfToken string) (CertPaths, error) {
	var out CertPaths
	// DNS-01 申请证书需等待 DNS 传播，远超默认 5s client timeout，必须用 doLong。
	err := c.doLong(ctx, http.MethodPost, "/v1/node/cert/ensure", map[string]string{"domain": domain, "cf_token": cfToken}, &out)
	return out, err
}

// CheckUnlockResult 节点检测单个服务的结果。
type CheckUnlockResult struct {
	Service  string `json:"service"`
	Unlocked bool   `json:"unlocked"`
	Region   string `json:"region,omitempty"`
	Note     string `json:"note,omitempty"`
}

// CheckUnlockResponse 节点解锁检测接口响应。
type CheckUnlockResponse struct {
	Direct         []CheckUnlockResult `json:"direct"`
	Proxied        []CheckUnlockResult `json:"proxied,omitempty"`
	ProxyAvailable bool                `json:"proxy_available"`
}

// SpeedTestResponse 节点测速接口响应。
type SpeedTestResponse struct {
	DownBps int64 `json:"down_bps"`
	UpBps   int64 `json:"up_bps"`
}

// SpeedTest 向节点发起测速请求（下载 + 上传各 10MB，总超时约 60s）。
// 使用无 Client.Timeout 的 http.Client，由 ctx 控制超时。
func (c *Client) SpeedTest(ctx context.Context) (SpeedTestResponse, error) {
	var out SpeedTestResponse
	err := c.doLong(ctx, http.MethodGet, "/v1/node/speedtest", nil, &out)
	return out, err
}

// CheckUnlock 向节点发起解锁检测请求，节点并发检测各服务并返回结果。
// 使用无 Client.Timeout 的 http.Client，由 ctx 控制超时。
func (c *Client) CheckUnlock(ctx context.Context) (CheckUnlockResponse, error) {
	var out CheckUnlockResponse
	err := c.doLong(ctx, http.MethodGet, "/v1/node/check", nil, &out)
	return out, err
}

// TracerouteHop 单跳追踪结果。
type TracerouteHop struct {
	Hop     int       `json:"hop"`
	IP      string    `json:"ip,omitempty"`
	RttMs   []float64 `json:"rtt_ms,omitempty"`
	Timeout bool      `json:"timeout,omitempty"`
}

// TracerouteResult 路由追踪结果。
type TracerouteResult struct {
	Host   string          `json:"host"`
	Method string          `json:"method"`
	Hops   []TracerouteHop `json:"hops"`
	Error  string          `json:"error,omitempty"`
}

// TracerouteRequest 路由追踪请求参数。
type TracerouteRequest struct {
	Host   string // 目标地址
	Method string // "icmp" 或 "tcp"
	Port   int    // TCP 模式下的目标端口（默认 80）
}

// LatencyProbeResult 节点三网延迟探测结果（ms），nil 表示超时/不可达。
type LatencyProbeResult struct {
	CT *int `json:"ct"`
	CU *int `json:"cu"`
	CM *int `json:"cm"`
}

// ProbeLatency 触发节点对上海三网目标的 TCP 延迟探测。
func (c *Client) ProbeLatency(ctx context.Context) (LatencyProbeResult, error) {
	var result LatencyProbeResult
	err := c.do(ctx, http.MethodGet, "/v1/node/latency/probe", nil, &result)
	return result, err
}

// TracerouteStream 向节点发起路由追踪 SSE 请求，返回 response body（调用方负责 Close）。
// 使用无超时的 HTTP 客户端，由 ctx 控制生命周期。
func (c *Client) TracerouteStream(ctx context.Context, req TracerouteRequest) (io.ReadCloser, error) {
	if c.initErr != nil {
		return nil, fmt.Errorf("configure node client: %w", c.initErr)
	}
	path := fmt.Sprintf("/v1/node/traceroute?host=%s&method=%s&port=%d",
		req.Host, req.Method, req.Port)
	r, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	sc := &http.Client{Transport: c.httpClient.Transport}
	resp, err := sc.Do(r)
	if err != nil {
		return nil, fmt.Errorf("request node: %w", err)
	}
	if resp.StatusCode >= 400 {
		resp.Body.Close()
		return nil, fmt.Errorf("node returned %d", resp.StatusCode)
	}
	return resp.Body, nil
}

// SNIProxyRoute 对应节点 sniproxy.Route（这里用独立类型避免 nodes 依赖 sniproxy 包）。
type SNIProxyRoute struct {
	SNI     string `json:"sni"`
	Backend string `json:"backend"`
	// Mode 取值："transparent" 或 "terminating"。
	Mode string `json:"mode"`
}

// SNIProxySyncRequest 推送给节点的完整 sniproxy 配置。
type SNIProxySyncRequest struct {
	Listen          string          `json:"listen"`
	CertStoragePath string          `json:"cert_storage_path,omitempty"`
	ACMEEmail       string          `json:"acme_email,omitempty"`
	CloudflareToken string          `json:"cloudflare_token,omitempty"`
	ACMEStaging     bool            `json:"acme_staging,omitempty"`
	Routes          []SNIProxyRoute `json:"routes"`
	// CertDomains 需要由 certmgr 管理、但不出现在 Terminating 路由中的额外域名。
	// direct TLS 模式下 Xray 自持证书，仍需 certmgr 统一申请和续期。
	CertDomains []string `json:"cert_domains,omitempty"`
}

// SyncSNIProxy 把内置 SNI 代理的完整配置推送给节点，节点端热更新。
// 空的 Routes 或空的 Listen 会让节点停止代理。
func (c *Client) SyncSNIProxy(ctx context.Context, req SNIProxySyncRequest) error {
	return c.do(ctx, http.MethodPost, "/v1/node/sniproxy/sync", req, nil)
}

// SNIProxyStatus 查询节点当前 SNI 代理运行状态。
// 返回结构是原样 JSON，由调用方按 nodeapi 定义解读（字段结构允许演进）。
func (c *Client) SNIProxyStatus(ctx context.Context) (map[string]any, error) {
	var out map[string]any
	err := c.do(ctx, http.MethodGet, "/v1/node/sniproxy/status", nil, &out)
	return out, err
}

// IPSentinelDetect 触发节点同步 IP 检测。
func (c *Client) IPSentinelDetect(ctx context.Context) (ipsentinel.DetectResult, error) {
	var out ipsentinel.DetectResult
	err := c.do(ctx, http.MethodPost, "/v1/node/ip-sentinel/detect", nil, &out)
	return out, err
}

// IPSentinelDetectGoogle 从节点实际访问 Google，检测 Google 对该 IP 的地区判断。
func (c *Client) IPSentinelDetectGoogle(ctx context.Context) (ipsentinel.GoogleDetectResult, error) {
	var out ipsentinel.GoogleDetectResult
	err := c.do(ctx, http.MethodPost, "/v1/node/ip-sentinel/detect-google", nil, &out)
	return out, err
}

// IPSentinelRun 触发节点运行 IP Sentinel 任务（google/trust/auto），异步执行。
func (c *Client) IPSentinelRun(ctx context.Context, taskType string) error {
	return c.do(ctx, http.MethodPost, "/v1/node/ip-sentinel/run", map[string]string{"type": taskType}, nil)
}

// IPSentinelStatus 获取节点 IP Sentinel 任务状态。
func (c *Client) IPSentinelStatus(ctx context.Context) (ipsentinel.NodeStatus, error) {
	var out ipsentinel.NodeStatus
	err := c.do(ctx, http.MethodGet, "/v1/node/ip-sentinel/status", nil, &out)
	return out, err
}

// IPSentinelGetConfig 获取节点当前 IP Sentinel 配置。
func (c *Client) IPSentinelGetConfig(ctx context.Context) (ipsentinel.Config, error) {
	var out ipsentinel.Config
	err := c.do(ctx, http.MethodGet, "/v1/node/ip-sentinel/config", nil, &out)
	return out, err
}

// IPSentinelSetConfig 设置节点 IP Sentinel 配置。
func (c *Client) IPSentinelSetConfig(ctx context.Context, cfg ipsentinel.Config) error {
	return c.do(ctx, http.MethodPut, "/v1/node/ip-sentinel/config", cfg, nil)
}

func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	if c.initErr != nil {
		return fmt.Errorf("configure node client: %w", c.initErr)
	}
	var bodyReader *bytes.Reader
	if body == nil {
		bodyReader = bytes.NewReader(nil)
	} else {
		payload, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		bodyReader = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request node: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		var apiErr struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&apiErr)
		if apiErr.Error == "" {
			apiErr.Error = resp.Status
		}
		return fmt.Errorf("node api error: %s", apiErr.Error)
	}

	if out == nil {
		return nil
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

// doLong is like do but uses an http.Client without Timeout,
// relying solely on ctx for deadline control. Use for long-running
// requests like speedtest that exceed the default 5s client timeout.
func (c *Client) doLong(ctx context.Context, method, path string, body any, out any) error {
	if c.initErr != nil {
		return fmt.Errorf("configure node client: %w", c.initErr)
	}
	var bodyReader *bytes.Reader
	if body == nil {
		bodyReader = bytes.NewReader(nil)
	} else {
		payload, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		bodyReader = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	longClient := &http.Client{Transport: c.httpClient.Transport}
	resp, err := longClient.Do(req)
	if err != nil {
		return fmt.Errorf("request node: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		var apiErr struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&apiErr)
		if apiErr.Error == "" {
			apiErr.Error = resp.Status
		}
		return fmt.Errorf("node api error: %s", apiErr.Error)
	}

	if out == nil {
		return nil
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}
