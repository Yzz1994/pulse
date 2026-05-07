package nodes

import (
	"errors"
	"time"
)

var ErrNodeNotFound = errors.New("node not found")

type Node struct {
	ID                string     `json:"id"`
	Name              string     `json:"name"`
	BaseURL           string     `json:"base_url"`
	UploadBytes       int64      `json:"upload_bytes"`
	DownloadBytes     int64      `json:"download_bytes"`
	ACMEEmail    string     `json:"acme_email"`
	PanelDomain  string     `json:"panel_domain"`
	ExtraProxies string     `json:"extra_proxies"` // 额外反代规则，每行一条 "domain:port"
	HTTPSPort    int        `json:"https_port"` // AnyTLS/Trojan 监听端口，0 = 默认 443
	ExpireAt          *time.Time `json:"expire_at"`  // VPS 到期时间（可选）
	PanelURL          string     `json:"panel_url"`  // VPS 控制面板地址
	Remark            string     `json:"remark"`     // 备注
	IPOverride        string     `json:"ip_override"` // GeoIP 查询用的 IP/域名（base_url 为内网时填写公网地址）
	Disabled          bool       `json:"disabled"`   // 禁用后跳过流量同步和配置下发
	// IsLanding 标记该节点为落地机（默认 true）。
	// 落地机不采集 access log、不做延迟检测和路由追踪。
	// 设为 false 表示线路机，启用上述功能。
	IsLanding bool `json:"is_landing"`
}

// CheckResult 节点解锁检测结果，按 (node_id, service, check_type) 唯一存储。
type CheckResult struct {
	Service   string    `json:"service"`
	CheckType string    `json:"check_type"` // "direct" 或 "proxied"
	Unlocked  bool      `json:"unlocked"`
	Region    string    `json:"region"`
	Note      string    `json:"note"`
	CheckedAt time.Time `json:"checked_at"`
}

// SpeedTestResult 节点测速结果，按 node_id 唯一存储。
type SpeedTestResult struct {
	DownBps  int64     `json:"down_bps"`
	UpBps    int64     `json:"up_bps"`
	TestedAt time.Time `json:"tested_at"`
}

// UptimeSummary 某节点在指定天数内的可用性汇总。
type UptimeSummary struct {
	TotalChecks   int    `json:"total_checks"`
	OnlineChecks  int    `json:"online_checks"`
	RunningChecks int    `json:"running_checks"`
	OnlinePct     int    `json:"online_pct"`  // 0-100
	RunningPct    int    `json:"running_pct"` // 0-100
	Label         string `json:"label"`        // 实际覆盖时长标签，如 "2h" / "3d"，不足 1h 时为空
}

// UptimeBar 单个时间段的可用性柱，用于状态条形图。
type UptimeBar struct {
	Label     string // 悬停提示标签，如 "04/05" 或 "04/05 14:00"
	OnlinePct int    // 0-100；-1 表示该时段无数据
}

// UptimeBarsResult 某节点的可用性条形图数据。
type UptimeBarsResult struct {
	Bars        []UptimeBar
	Granularity string // "day" 或 "hour"
	OverallPct  int    // 总体在线率 0-100
}

// NodeDailyUsage 某节点某日的流量快照。
type NodeDailyUsage struct {
	NodeID        string
	Date          string // YYYY-MM-DD
	UploadBytes   int64
	DownloadBytes int64
}

// TracerouteSnapshot 单次路由追踪快照。
type TracerouteSnapshot struct {
	ID        string    `json:"id"`
	NodeID    string    `json:"node_id"`
	Direction string    `json:"direction"` // "inbound" 或 "outbound"
	Target    string    `json:"target"`    // 回程：目标地址描述；去程：城市名
	Hops      string    `json:"hops"`      // JSON 序列化的跳数据
	Quality   string    `json:"quality"`   // "CN2 GIA" / "CN2 GT" / "163" 等，空表示未识别
	CreatedAt time.Time `json:"created_at"`
}

type Store interface {
	Upsert(node Node) (Node, error)
	Delete(id string) error
	Get(id string) (Node, error)
	List() ([]Node, error)
	// AddTraffic 原子性地将 upload/download 字节数累加到节点流量计数器。
	AddTraffic(nodeID string, upload, download int64) error
	// AddNodeDailyUsage 将 delta 流量累加到当日统计桶（幂等 upsert）。
	AddNodeDailyUsage(nodeID, date string, upload, download int64) error
	// ListNodeDailyUsage 返回最近 days 天内所有节点的日流量记录。
	ListNodeDailyUsage(days int) ([]NodeDailyUsage, error)
	// ListNodeDailyUsageRange 返回指定节点在 [since, until]（YYYY-MM-DD）的日流量记录，按日期升序。
	ListNodeDailyUsageRange(nodeID, since, until string) ([]NodeDailyUsage, error)
	// CleanupOldDailyUsage 删除超过 retainDays 天的历史日流量记录。
	CleanupOldDailyUsage(retainDays int) error
	// UpsertNodeCheckResults 批量写入节点解锁检测结果（按 node_id+service 唯一）。
	UpsertNodeCheckResults(nodeID string, results []CheckResult) error
	// ListAllNodeCheckResults 返回所有节点的解锁检测结果，按 nodeID 分组。
	ListAllNodeCheckResults() (map[string][]CheckResult, error)
	// UpsertNodeSpeedTest 写入节点测速结果（按 node_id 唯一）。
	UpsertNodeSpeedTest(nodeID string, result SpeedTestResult) error
	// ListAllNodeSpeedTests 返回所有节点的最新测速结果。
	ListAllNodeSpeedTests() (map[string]SpeedTestResult, error)
	// RecordNodeUptime 写入一条按分钟可用性快照。
	RecordNodeUptime(nodeID string, online, running bool) error
	// ListNodeUptimeSummary 返回最近 days 天内所有节点的可用性汇总（基于分钟级快照）。
	ListNodeUptimeSummary(days int) (map[string]UptimeSummary, error)
	// CleanupOldNodeUptime 删除超过 retainDays 天的历史快照。
	CleanupOldNodeUptime(retainDays int) error
	// ListNodeUptimeBars 返回所有节点的可用性条形图数据，粒度由数据跨度自动决定（<3天→小时，否则→天）。
	ListNodeUptimeBars(maxDays int) (map[string]UptimeBarsResult, error)
	// SaveTracerouteSnapshot 保存一条追踪快照（自动生成 ID）。
	SaveTracerouteSnapshot(snapshot TracerouteSnapshot) error
	// ListNodeTracerouteSnapshots 返回节点最近 N 条快照，按时间降序。
	ListNodeTracerouteSnapshots(nodeID string, limit int) ([]TracerouteSnapshot, error)
	// ListLatestTracerouteSnapshots 返回所有节点的最新快照（每个 nodeID+direction+target 各取最新一条），按 nodeID 分组。
	ListLatestTracerouteSnapshots() (map[string][]TracerouteSnapshot, error)
	// DeleteTracerouteSnapshot 删除指定 ID 的追踪快照。
	DeleteTracerouteSnapshot(id string) error
	// SaveLatencySamples 批量保存延迟采样记录。
	SaveLatencySamples(samples []LatencySample) error
	// QueryLatencySamples 查询指定节点在时间范围内的延迟采样，按 sampled_at 升序。
	QueryLatencySamples(nodeIDs []string, from, to time.Time) ([]LatencySample, error)
	// CleanupOldLatencySamples 删除 before 之前的采样记录。
	CleanupOldLatencySamples(before time.Time) error
}

// LatencySample 单次延迟采样记录。
type LatencySample struct {
	NodeID    string    `json:"node_id"`
	ISP       string    `json:"isp"`      // "ct" / "cu" / "cm"
	RttMs     *int      `json:"rtt_ms"`   // nil = 超时/不可达
	SampledAt time.Time `json:"sampled_at"`
}
