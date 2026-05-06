package accesslogs

import "time"

// Entry 表示一条访问日志记录。
type Entry struct {
	ID          int64     `json:"id"`
	NodeID      string    `json:"node_id"`
	Username    string    `json:"username"`
	SourceIP    string    `json:"source_ip"`
	SourcePort  string    `json:"source_port"`
	Destination string    `json:"destination"`
	RemoteIP    string    `json:"remote_ip"`
	RouteTag    string    `json:"route_tag"`
	Protocol    string    `json:"protocol"`
	InboundTag  string    `json:"inbound_tag"`
	CreatedAt   time.Time `json:"created_at"`
}

// ListParams 历史查询参数。
type ListParams struct {
	NodeID   string
	Username string
	Since    time.Time
	Until    time.Time
	Limit    int
	Offset   int
}

// UserAnalysis 单用户聚合分析结果。
type UserAnalysis struct {
	Username    string    `json:"username"`
	Connections int64     `json:"connections"`   // 时间段内总连接数
	DistinctIPs int64     `json:"distinct_ips"`  // 时间段内独立源 IP 数
	TotalBytes  int64     `json:"total_bytes"`   // 时间段内上下行总字节（来自 user_node_daily_usage）
	LastSeen    time.Time `json:"last_seen"`     // 最近一次连接时间
}

type Store interface {
	Insert(entries []Entry) error
	List(params ListParams) ([]Entry, error)
	ListDistinctUsers(since time.Time) ([]string, error)
	ListUserAnalysis(since, until time.Time) ([]UserAnalysis, error)
	Count() (int64, error)
	DeleteOlderThan(t time.Time) (int64, error)
}
