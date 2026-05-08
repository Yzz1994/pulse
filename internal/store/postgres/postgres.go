package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DB struct {
	conn *pgxpool.Pool
}

func Open(dsn string) (*DB, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("open postgres: parse config: %w", err)
	}
	cfg.MaxConns = 20
	conn, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}

	db := &DB{conn: conn}
	if err := db.init(); err != nil {
		conn.Close()
		return nil, err
	}
	return db, nil
}

func (db *DB) Close() error {
	db.conn.Close()
	return nil
}

// CleanupResult 记录数据核查清理的结果。
type CleanupResult struct {
	UserInboundsByUser      int64 `json:"user_inbounds_by_user"`        // user 已删的 user_inbounds
	UserInboundsByInbound   int64 `json:"user_inbounds_by_inbound"`     // inbound 已删的 user_inbounds
	HostsByInbound          int64 `json:"hosts_by_inbound"`             // inbound 已删的 hosts
	InboundsByNode          int64 `json:"inbounds_by_node"`             // node 已删的 inbounds
	DailyUsageByNode        int64 `json:"daily_usage_by_node"`          // node 已删的流量记录
	SubAccessLogsByUser     int64 `json:"sub_access_logs_by_user"`      // user 已删的订阅访问日志
	UserNodeUsageByUser     int64 `json:"user_node_usage_by_user"`      // user 已删的节点流量记录
	UserNodeUsageByNode     int64 `json:"user_node_usage_by_node"`      // node 已删的节点流量记录
	NodeUptimeByNode        int64 `json:"node_uptime_by_node"`          // node 已删的可用性日志
	NodeSpeedtestByNode     int64 `json:"node_speedtest_by_node"`       // node 已删的测速记录
	NodeCheckResultsByNode  int64 `json:"node_check_results_by_node"`   // node 已删的解锁检测记录
	TracerouteByNode        int64 `json:"traceroute_snapshots_by_node"` // node 已删的路由追踪记录
	NodeDomainsByNode       int64 `json:"node_domains_by_node"`         // node 已删的域名记录
	IPSentinelConfigByNode  int64 `json:"ip_sentinel_config_by_node"`   // node 已删的 IP Sentinel 配置
	IPSentinelRunsByNode    int64 `json:"ip_sentinel_runs_by_node"`     // node 已删的 IP Sentinel 运行记录
	TicketMessagesByTicket  int64 `json:"ticket_messages_by_ticket"`    // ticket 已删的消息
	TicketImagesByTicket    int64 `json:"ticket_images_by_ticket"`      // ticket 已删的图片
	HostExclusionsByUser    int64 `json:"host_exclusions_by_user"`      // user 已删的排除记录
	HostExclusionsByHost    int64 `json:"host_exclusions_by_host"`      // host 已删的排除记录
	UserGroupMembersByUser  int64 `json:"user_group_members_by_user"`   // user 已删的组成员记录
	UserGroupMembersByGroup int64 `json:"user_group_members_by_group"`  // group 已删的组成员记录
	Total                   int64 `json:"total"`
}

// Cleanup 清理数据库中所有孤立记录，返回各表删除数量。
// 所有删除在同一事务中执行，保证原子性。
func (db *DB) Cleanup() (CleanupResult, error) {
	var r CleanupResult
	ctx := context.Background()

	tx, err := db.conn.Begin(ctx)
	if err != nil {
		return r, fmt.Errorf("cleanup begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	steps := []struct {
		sql  string
		dest *int64
	}{
		{`DELETE FROM user_inbounds WHERE user_id NOT IN (SELECT id FROM users)`, &r.UserInboundsByUser},
		{`DELETE FROM user_inbounds WHERE inbound_id != '' AND inbound_id NOT IN (SELECT id FROM inbounds)`, &r.UserInboundsByInbound},
		{`DELETE FROM hosts WHERE inbound_id NOT IN (SELECT id FROM inbounds)`, &r.HostsByInbound},
		{`DELETE FROM inbounds WHERE node_id NOT IN (SELECT id FROM nodes)`, &r.InboundsByNode},
		{`DELETE FROM node_daily_usage WHERE node_id NOT IN (SELECT id FROM nodes)`, &r.DailyUsageByNode},
		{`DELETE FROM sub_access_logs WHERE user_id NOT IN (SELECT id FROM users)`, &r.SubAccessLogsByUser},
		{`DELETE FROM user_node_daily_usage WHERE user_id NOT IN (SELECT id FROM users)`, &r.UserNodeUsageByUser},
		{`DELETE FROM user_node_daily_usage WHERE node_id NOT IN (SELECT id FROM nodes)`, &r.UserNodeUsageByNode},
		{`DELETE FROM node_uptime_log WHERE node_id NOT IN (SELECT id FROM nodes)`, &r.NodeUptimeByNode},
		{`DELETE FROM node_speedtest WHERE node_id NOT IN (SELECT id FROM nodes)`, &r.NodeSpeedtestByNode},
		{`DELETE FROM node_check_results WHERE node_id NOT IN (SELECT id FROM nodes)`, &r.NodeCheckResultsByNode},
		{`DELETE FROM traceroute_snapshots WHERE node_id NOT IN (SELECT id FROM nodes)`, &r.TracerouteByNode},
		{`DELETE FROM node_domains WHERE node_id NOT IN (SELECT id FROM nodes)`, &r.NodeDomainsByNode},
		{`DELETE FROM ip_sentinel_configs WHERE node_id NOT IN (SELECT id FROM nodes)`, &r.IPSentinelConfigByNode},
		{`DELETE FROM ip_sentinel_runs WHERE node_id NOT IN (SELECT id FROM nodes)`, &r.IPSentinelRunsByNode},
		{`DELETE FROM ticket_messages WHERE ticket_id NOT IN (SELECT id FROM tickets)`, &r.TicketMessagesByTicket},
		{`DELETE FROM ticket_images WHERE ticket_id NOT IN (SELECT id FROM tickets)`, &r.TicketImagesByTicket},
		{`DELETE FROM user_host_exclusions WHERE user_id NOT IN (SELECT id FROM users)`, &r.HostExclusionsByUser},
		{`DELETE FROM user_host_exclusions WHERE host_id NOT IN (SELECT id FROM hosts)`, &r.HostExclusionsByHost},
		{`DELETE FROM user_group_members WHERE user_id NOT IN (SELECT id FROM users)`, &r.UserGroupMembersByUser},
		{`DELETE FROM user_group_members WHERE group_id NOT IN (SELECT id FROM user_groups)`, &r.UserGroupMembersByGroup},
	}

	for _, step := range steps {
		res, err := tx.Exec(ctx, step.sql)
		if err != nil {
			return r, fmt.Errorf("cleanup: %w", err)
		}
		n := res.RowsAffected()
		*step.dest = n
		r.Total += n
	}

	if err := tx.Commit(ctx); err != nil {
		return r, fmt.Errorf("cleanup commit: %w", err)
	}
	return r, nil
}

func (db *DB) NodeStore() *NodeStore {
	return &NodeStore{db: db.conn}
}

func (db *DB) UserStore() *UserStore {
	return &UserStore{db: db.conn}
}

func (db *DB) SessionStore() *SessionStore {
	return &SessionStore{db: db.conn}
}

func (db *DB) SettingsStore() *SettingsStore {
	return &SettingsStore{db: db.conn}
}

func (db *DB) PlanStore() *PlanStore {
	return &PlanStore{db: db.conn}
}

func (db *DB) OrderStore() *OrderStore {
	return &OrderStore{db: db.conn}
}

// QueryResult 保存一次查询的列名与行数据。
type QueryResult struct {
	Columns []string `json:"columns"`
	Rows    [][]any  `json:"rows"`
	Elapsed float64  `json:"elapsed_ms"`
}

// Query 在只读事务中执行 SQL，最多返回 500 行，超过 5 秒自动取消。
// 使用只读事务隔离，防止 DDL/DML 误操作。
func (db *DB) Query(sqlStr string) (*QueryResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	start := time.Now()

	tx, err := db.conn.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	rows, err := tx.Query(ctx, sqlStr)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	fds := rows.FieldDescriptions()
	cols := make([]string, len(fds))
	for i, fd := range fds {
		cols[i] = fd.Name
	}

	result := &QueryResult{Columns: cols}
	for rows.Next() {
		if len(result.Rows) >= 500 {
			break
		}
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		// 将 []byte 转为 string，方便 JSON 序列化
		row := make([]any, len(cols))
		for i, v := range vals {
			if b, ok := v.([]byte); ok {
				row[i] = string(b)
			} else {
				row[i] = v
			}
		}
		result.Rows = append(result.Rows, row)
	}
	result.Elapsed = float64(time.Since(start).Microseconds()) / 1000.0
	return result, rows.Err()
}

// UserGroupStore 返回用户组 Store 实例。
func (db *DB) UserGroupStore() *UserGroupStore {
	return &UserGroupStore{db: db.conn}
}

// CFDomainStore 返回 CF 域名 Store 实例。
func (db *DB) CFDomainStore() *CFDomainStore {
	return &CFDomainStore{db: db.conn}
}

// NodeDomainStore 返回节点域名 Store 实例。
func (db *DB) NodeDomainStore() *NodeDomainStore {
	return &NodeDomainStore{db: db.conn}
}

// AnnouncementStore 返回公告 Store 实例。
func (db *DB) AnnouncementStore() *AnnouncementStore {
	return &AnnouncementStore{db: db.conn}
}

// TicketStore 返回工单 Store 实例。
func (db *DB) TicketStore() *TicketStore {
	return &TicketStore{db: db.conn}
}

func (db *DB) AccessLogStore() *AccessLogStore {
	return &AccessLogStore{db: db.conn}
}

func (db *DB) AuditRuleStore() *AuditRuleStore {
	return &AuditRuleStore{db: db.conn}
}

func (db *DB) PortalSessionStore() *PortalSessionStore {
	return &PortalSessionStore{db: db.conn}
}

// DBStats 数据库文件、性能及各表行数指标。
type DBStats struct {
	FileSizeBytes int64            `json:"file_size_bytes"`
	WALSizeBytes  int64            `json:"wal_size_bytes"`
	PageCount     int64            `json:"page_count"`
	PageSize      int64            `json:"page_size"`
	FreePages     int64            `json:"free_pages"`
	PingLatencyMs float64          `json:"ping_latency_ms"`
	Tables        map[string]int64 `json:"tables"`
}

// Stats 采集当前数据库大小、Ping 延迟及各表行数。
// 调用方负责在后台 goroutine 中调用，不要在 HTTP handler 中直接调用。
func (db *DB) Stats(_ string) DBStats {
	var s DBStats
	ctx := context.Background()

	// 查询数据库大小
	db.conn.QueryRow(ctx, "SELECT pg_database_size(current_database())").Scan(&s.FileSizeBytes)
	// WALSizeBytes、PageCount、PageSize、FreePages 字段设为 0（PostgreSQL 无对应概念）

	start := time.Now()
	db.conn.QueryRow(ctx, "SELECT 1").Scan(new(int))
	s.PingLatencyMs = float64(time.Since(start).Microseconds()) / 1000.0

	// 枚举 public schema 下的用户表并统计行数
	// 表名来自 pg_tables.tablename（系统表，非用户输入），安全。
	rows, err := db.conn.Query(ctx, "SELECT tablename FROM pg_tables WHERE schemaname = 'public' ORDER BY tablename")
	if err == nil {
		defer rows.Close()
		s.Tables = make(map[string]int64)
		for rows.Next() {
			var tbl string
			if rows.Scan(&tbl) == nil {
				var count int64
				// tbl 来自 pg_tables，只含小写字母、数字和下划线，无注入风险。
				db.conn.QueryRow(ctx, "SELECT COUNT(*) FROM "+tbl).Scan(&count) //nolint:gosec
				s.Tables[tbl] = count
			}
		}
		if err := rows.Err(); err != nil {
			// 统计失败不致命，直接忽略
			_ = err
		}
	}

	return s
}
