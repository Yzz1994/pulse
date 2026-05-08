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
	conn, err := pgxpool.New(context.Background(), dsn)
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
	HostExclusionsByUser       int64 `json:"host_exclusions_by_user"`        // user 已删的排除记录
	HostExclusionsByHost       int64 `json:"host_exclusions_by_host"`        // host 已删的排除记录
	UserGroupMembersByUser     int64 `json:"user_group_members_by_user"`    // user 已删的组成员记录
	UserGroupMembersByGroup    int64 `json:"user_group_members_by_group"`   // group 已删的组成员记录
	Total                      int64 `json:"total"`
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

func (db *DB) init() error {
	ctx := context.Background()
	stmts := []string{
		// inbounds：节点上的监听入站，含服务端 TLS/Reality 配置
		`CREATE TABLE IF NOT EXISTS inbounds (
			id                     TEXT PRIMARY KEY,
			node_id                TEXT NOT NULL,
			protocol               TEXT NOT NULL,
			tag                    TEXT NOT NULL,
			port                   INTEGER NOT NULL,
			method                 TEXT NOT NULL DEFAULT '',
			password               TEXT NOT NULL DEFAULT '',
			security               TEXT NOT NULL DEFAULT '',
			reality_private_key    TEXT NOT NULL DEFAULT '',
			reality_public_key     TEXT NOT NULL DEFAULT '',
			reality_handshake_addr TEXT NOT NULL DEFAULT '',
			reality_short_id       TEXT NOT NULL DEFAULT '',
			outbound_id            TEXT NOT NULL DEFAULT '',
			traffic_rate           DOUBLE PRECISION NOT NULL DEFAULT 1.0
		)`,
		// hosts：客户端连接模板（地址 + TLS 客户端参数）
		`CREATE TABLE IF NOT EXISTS hosts (
			id                 TEXT PRIMARY KEY,
			inbound_id         TEXT NOT NULL,
			remark             TEXT NOT NULL DEFAULT '',
			address            TEXT NOT NULL DEFAULT '',
			port               INTEGER NOT NULL DEFAULT 0,
			sni                TEXT NOT NULL DEFAULT '',
			host               TEXT NOT NULL DEFAULT '',
			path               TEXT NOT NULL DEFAULT '',
			security           TEXT NOT NULL DEFAULT 'none',
			alpn               TEXT NOT NULL DEFAULT '',
			fingerprint        TEXT NOT NULL DEFAULT '',
			allow_insecure     INTEGER NOT NULL DEFAULT 0,
			mux_enable         INTEGER NOT NULL DEFAULT 0,
			reality_public_key TEXT NOT NULL DEFAULT '',
			reality_short_id   TEXT NOT NULL DEFAULT '',
			reality_spider_x   TEXT NOT NULL DEFAULT '',
			country            TEXT NOT NULL DEFAULT '',
			region             TEXT NOT NULL DEFAULT '',
			network            TEXT NOT NULL DEFAULT '',
			entry              TEXT NOT NULL DEFAULT '',
			tags               TEXT NOT NULL DEFAULT '',
			relay_node_id      TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS nodes (
			id                  TEXT PRIMARY KEY,
			name                TEXT NOT NULL,
			base_url            TEXT NOT NULL,
			upload_bytes        BIGINT NOT NULL DEFAULT 0,
			download_bytes      BIGINT NOT NULL DEFAULT 0,
			acme_email          TEXT NOT NULL DEFAULT '',
			panel_domain        TEXT NOT NULL DEFAULT '',
			extra_proxies       TEXT NOT NULL DEFAULT '',
			https_port          INTEGER NOT NULL DEFAULT 0,
			expire_at           TEXT,
			panel_url           TEXT NOT NULL DEFAULT '',
			remark              TEXT NOT NULL DEFAULT '',
			ip_override         TEXT NOT NULL DEFAULT '',
			disabled            INTEGER NOT NULL DEFAULT 0,
			tls_mode            TEXT NOT NULL DEFAULT '',
			is_landing          BOOLEAN NOT NULL DEFAULT TRUE
		)`,
		// outbounds：独立的出口代理配置
		`CREATE TABLE IF NOT EXISTS outbounds (
			id          TEXT PRIMARY KEY,
			name        TEXT NOT NULL DEFAULT '',
			protocol    TEXT NOT NULL DEFAULT 'socks5',
			server      TEXT NOT NULL DEFAULT '',
			username    TEXT NOT NULL DEFAULT '',
			password    TEXT NOT NULL DEFAULT '',
			method      TEXT NOT NULL DEFAULT '',
			uuid        TEXT NOT NULL DEFAULT '',
			sni         TEXT NOT NULL DEFAULT '',
			public_key  TEXT NOT NULL DEFAULT '',
			short_id    TEXT NOT NULL DEFAULT '',
			fingerprint TEXT NOT NULL DEFAULT ''
		)`,
		// users：用户身份 + 流量统计
		`CREATE TABLE IF NOT EXISTS users (
			id                        TEXT PRIMARY KEY,
			username                  TEXT NOT NULL,
			status                    TEXT NOT NULL DEFAULT 'active',
			note                      TEXT NOT NULL DEFAULT '',
			expire_at                 TEXT,
			data_limit_reset_strategy TEXT NOT NULL DEFAULT 'no_reset',
			traffic_limit_bytes       BIGINT NOT NULL DEFAULT 0,
			upload_bytes              BIGINT NOT NULL DEFAULT 0,
			download_bytes            BIGINT NOT NULL DEFAULT 0,
			used_bytes                BIGINT NOT NULL DEFAULT 0,
			raw_upload_bytes          BIGINT NOT NULL DEFAULT 0,
			raw_download_bytes        BIGINT NOT NULL DEFAULT 0,
			on_hold_expire_at         TEXT,
			last_traffic_reset_at     TEXT,
			online_at                 TEXT,
			connections               INTEGER NOT NULL DEFAULT 0,
			devices                   INTEGER NOT NULL DEFAULT 0,
			created_at                TEXT NOT NULL,
			sub_token                 TEXT NOT NULL DEFAULT '',
			stripe_customer_id        TEXT NOT NULL DEFAULT '',
			current_plan_id           TEXT NOT NULL DEFAULT '',
			email                     TEXT NOT NULL DEFAULT ''
		)`,
		// user_inbounds：用户对具体 inbound 的访问凭据
		`CREATE TABLE IF NOT EXISTS user_inbounds (
			id                   TEXT PRIMARY KEY,
			user_id              TEXT NOT NULL,
			inbound_id           TEXT NOT NULL DEFAULT '',
			node_id              TEXT NOT NULL DEFAULT '',
			uuid                 TEXT NOT NULL DEFAULT '',
			secret               TEXT NOT NULL DEFAULT '',
			created_at           TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_user_inbounds_user_id ON user_inbounds(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_user_inbounds_node_id ON user_inbounds(node_id)`,
		// user_host_exclusions：用户排除的节点
		`CREATE TABLE IF NOT EXISTS user_host_exclusions (
			user_id TEXT NOT NULL,
			host_id TEXT NOT NULL,
			PRIMARY KEY (user_id, host_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_user_host_exclusions_user ON user_host_exclusions(user_id)`,
		// sessions：管理员登录 session
		`CREATE TABLE IF NOT EXISTS sessions (
			token      TEXT PRIMARY KEY,
			username   TEXT NOT NULL,
			created_at TEXT NOT NULL
		)`,
		// settings：系统配置 KV 表
		`CREATE TABLE IF NOT EXISTS settings (
			key   TEXT PRIMARY KEY,
			value TEXT NOT NULL DEFAULT ''
		)`,
		// node_daily_usage：节点按天流量 delta
		`CREATE TABLE IF NOT EXISTS node_daily_usage (
			node_id        TEXT NOT NULL,
			date           TEXT NOT NULL,
			upload_bytes   BIGINT NOT NULL DEFAULT 0,
			download_bytes BIGINT NOT NULL DEFAULT 0,
			PRIMARY KEY (node_id, date)
		)`,
		// sub_access_logs：记录 /sub/:token 的访问记录
		`CREATE TABLE IF NOT EXISTS sub_access_logs (
			id          BIGSERIAL PRIMARY KEY,
			user_id     TEXT NOT NULL,
			ip          TEXT NOT NULL DEFAULT '',
			user_agent  TEXT NOT NULL DEFAULT '',
			accessed_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_sub_access_logs_user_id ON sub_access_logs(user_id)`,
		// node_speedtest：节点测速结果
		`CREATE TABLE IF NOT EXISTS node_speedtest (
			node_id   TEXT PRIMARY KEY,
			down_bps  BIGINT NOT NULL DEFAULT 0,
			up_bps    BIGINT NOT NULL DEFAULT 0,
			tested_at TEXT NOT NULL DEFAULT ''
		)`,
		// node_check_results：节点解锁检测结果
		`CREATE TABLE IF NOT EXISTS node_check_results (
			node_id    TEXT NOT NULL,
			service    TEXT NOT NULL,
			check_type TEXT NOT NULL DEFAULT 'direct',
			unlocked   INTEGER NOT NULL DEFAULT 0,
			region     TEXT NOT NULL DEFAULT '',
			note       TEXT NOT NULL DEFAULT '',
			checked_at TEXT NOT NULL,
			PRIMARY KEY (node_id, service, check_type)
		)`,
		// node_uptime_log：节点可用性按分钟快照
		`CREATE TABLE IF NOT EXISTS node_uptime_log (
			node_id    TEXT NOT NULL,
			checked_at TEXT NOT NULL,
			online     INTEGER NOT NULL DEFAULT 0,
			running    INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (node_id, checked_at)
		)`,
		// user_node_daily_usage：用户在各节点的按天流量
		`CREATE TABLE IF NOT EXISTS user_node_daily_usage (
			user_id        TEXT NOT NULL,
			node_id        TEXT NOT NULL,
			date           TEXT NOT NULL,
			upload_bytes   BIGINT NOT NULL DEFAULT 0,
			download_bytes BIGINT NOT NULL DEFAULT 0,
			PRIMARY KEY (user_id, node_id, date)
		)`,
		// route_rules：全局分流规则
		`CREATE TABLE IF NOT EXISTS route_rules (
			id              TEXT PRIMARY KEY,
			name            TEXT NOT NULL DEFAULT '',
			rule_type       TEXT NOT NULL DEFAULT 'domain_suffix',
			patterns        TEXT NOT NULL DEFAULT '',
			outbound_id     TEXT NOT NULL DEFAULT '',
			priority        INTEGER NOT NULL DEFAULT 100,
			node_ids        TEXT NOT NULL DEFAULT '',
			inbound_ids     TEXT NOT NULL DEFAULT '',
			rule_set_url    TEXT NOT NULL DEFAULT '',
			rule_set_format TEXT NOT NULL DEFAULT 'binary'
		)`,
		// plans：套餐定义
		`CREATE TABLE IF NOT EXISTS plans (
			id                      TEXT PRIMARY KEY,
			name                    TEXT NOT NULL DEFAULT '',
			description             TEXT NOT NULL DEFAULT '',
			type                    TEXT NOT NULL DEFAULT 'one_time',
			price_cents             INTEGER NOT NULL DEFAULT 0,
			currency                TEXT NOT NULL DEFAULT 'usd',
			stripe_price_id         TEXT NOT NULL DEFAULT '',
			traffic_limit           BIGINT NOT NULL DEFAULT 0,
			duration_days           INTEGER NOT NULL DEFAULT 0,
			data_limit_reset_strategy TEXT NOT NULL DEFAULT 'no_reset',
			user_group_ids          TEXT NOT NULL DEFAULT '',
			sort_order              INTEGER NOT NULL DEFAULT 0,
			enabled                 INTEGER NOT NULL DEFAULT 0,
			mode                    TEXT NOT NULL DEFAULT 'live',
			stock_limit             INTEGER NOT NULL DEFAULT -1,
			stock_sold              INTEGER NOT NULL DEFAULT 0,
			created_at              TEXT NOT NULL
		)`,
		// orders：支付订单
		`CREATE TABLE IF NOT EXISTS orders (
			id                     TEXT PRIMARY KEY,
			user_id                TEXT NOT NULL DEFAULT '',
			plan_id                TEXT NOT NULL DEFAULT '',
			email                  TEXT NOT NULL DEFAULT '',
			stripe_session_id      TEXT NOT NULL DEFAULT '',
			stripe_subscription_id TEXT NOT NULL DEFAULT '',
			stripe_customer_id     TEXT NOT NULL DEFAULT '',
			status                 TEXT NOT NULL DEFAULT 'pending',
			amount_cents           INTEGER NOT NULL DEFAULT 0,
			currency               TEXT NOT NULL DEFAULT 'usd',
			created_at             TEXT NOT NULL,
			paid_at                TEXT,
			last_invoice_id        TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS idx_orders_user_id ON orders(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_orders_email ON orders(email)`,
		`CREATE INDEX IF NOT EXISTS idx_orders_stripe_session_id ON orders(stripe_session_id)`,
		`CREATE INDEX IF NOT EXISTS idx_orders_stripe_subscription_id ON orders(stripe_subscription_id)`,
		// cf_domains：Cloudflare 域名配置
		`CREATE TABLE IF NOT EXISTS cf_domains (
			id        TEXT PRIMARY KEY,
			cf_token  TEXT NOT NULL,
			zone_id   TEXT NOT NULL,
			zone_name TEXT NOT NULL,
			remark    TEXT DEFAULT ''
		)`,
		// announcements：公告表
		`CREATE TABLE IF NOT EXISTS announcements (
			id         TEXT PRIMARY KEY,
			title      TEXT NOT NULL DEFAULT '',
			content    TEXT NOT NULL DEFAULT '',
			enabled    BOOLEAN NOT NULL DEFAULT false,
			created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		// tickets：用户工单
		`CREATE TABLE IF NOT EXISTS tickets (
			id         TEXT PRIMARY KEY,
			user_id    TEXT NOT NULL,
			username   TEXT NOT NULL DEFAULT '',
			title      TEXT NOT NULL DEFAULT '',
			status     TEXT NOT NULL DEFAULT 'open',
			created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		// ticket_messages：工单对话消息
		`CREATE TABLE IF NOT EXISTS ticket_messages (
			id         TEXT PRIMARY KEY,
			ticket_id  TEXT NOT NULL,
			content    TEXT NOT NULL DEFAULT '',
			is_admin   BOOLEAN NOT NULL DEFAULT false,
			created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		// ticket_images：工单图片元数据
		`CREATE TABLE IF NOT EXISTS ticket_images (
			id          TEXT PRIMARY KEY,
			ticket_id   TEXT NOT NULL,
			filename    TEXT NOT NULL DEFAULT '',
			stored_name TEXT NOT NULL DEFAULT '',
			size        BIGINT NOT NULL DEFAULT 0,
			created_at  TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		// traceroute_snapshots：路由追踪历史快照
		`CREATE TABLE IF NOT EXISTS traceroute_snapshots (
			id          TEXT PRIMARY KEY,
			node_id     TEXT NOT NULL,
			direction   TEXT NOT NULL,
			target      TEXT NOT NULL,
			hops        TEXT NOT NULL,
			quality     TEXT NOT NULL DEFAULT '',
			created_at  TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_traceroute_node_created ON traceroute_snapshots(node_id, created_at DESC)`,
		// ip_sentinel_configs：IP Sentinel 节点配置
		`CREATE TABLE IF NOT EXISTS ip_sentinel_configs (
			node_id          TEXT PRIMARY KEY,
			region_code      TEXT NOT NULL DEFAULT '',
			region_name      TEXT NOT NULL DEFAULT '',
			base_lat         DOUBLE PRECISION NOT NULL DEFAULT 0,
			base_lon         DOUBLE PRECISION NOT NULL DEFAULT 0,
			lang_params      TEXT NOT NULL DEFAULT 'hl=en&gl=US',
			valid_url_suffix TEXT NOT NULL DEFAULT 'com',
			enable_google    BOOLEAN NOT NULL DEFAULT false,
			enable_trust     BOOLEAN NOT NULL DEFAULT false,
			white_urls       TEXT NOT NULL DEFAULT '[]',
			keywords         TEXT NOT NULL DEFAULT '[]',
			updated_at       TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		// ip_sentinel_runs：IP Sentinel 任务执行记录
		`CREATE TABLE IF NOT EXISTS ip_sentinel_runs (
			id           TEXT PRIMARY KEY,
			node_id      TEXT NOT NULL,
			task_type    TEXT NOT NULL DEFAULT '',
			triggered_by TEXT NOT NULL DEFAULT 'manual',
			status       TEXT NOT NULL DEFAULT 'pending',
			output       TEXT NOT NULL DEFAULT '',
			result       TEXT NOT NULL DEFAULT '',
			started_at   TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			finished_at  TIMESTAMPTZ
		)`,
		`CREATE INDEX IF NOT EXISTS idx_ip_sentinel_runs_node ON ip_sentinel_runs(node_id, started_at DESC)`,
		// node_domains：维护每个节点上用到的域名（从 CF 同步）
		`CREATE TABLE IF NOT EXISTS node_domains (
			id           TEXT PRIMARY KEY,
			node_id      TEXT NOT NULL DEFAULT '',
			cf_domain_id TEXT NOT NULL,
			fqdn         TEXT NOT NULL,
			record_type  TEXT NOT NULL DEFAULT 'A',
			content      TEXT NOT NULL DEFAULT '',
			proxied      BOOLEAN NOT NULL DEFAULT FALSE,
			synced_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_node_domains_fqdn ON node_domains(fqdn)`,
		// user_groups：用户组
		`CREATE TABLE IF NOT EXISTS user_groups (
			id          TEXT PRIMARY KEY,
			name        TEXT NOT NULL DEFAULT '',
			remark      TEXT NOT NULL DEFAULT '',
			inbound_ids TEXT NOT NULL DEFAULT ''
		)`,
		// user_group_members：用户组成员关系
		`CREATE TABLE IF NOT EXISTS user_group_members (
			group_id TEXT NOT NULL,
			user_id  TEXT NOT NULL,
			PRIMARY KEY (group_id, user_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_user_group_members_user ON user_group_members(user_id)`,
		`ALTER TABLE user_inbounds ADD COLUMN IF NOT EXISTS group_id TEXT NOT NULL DEFAULT ''`,
		`CREATE INDEX IF NOT EXISTS idx_user_inbounds_group ON user_inbounds(group_id) WHERE group_id != ''`,
		`ALTER TABLE plans ADD COLUMN IF NOT EXISTS user_group_ids TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE nodes ADD COLUMN IF NOT EXISTS https_port INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE nodes ADD COLUMN IF NOT EXISTS acme_email TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE nodes ADD COLUMN IF NOT EXISTS panel_domain TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE nodes ADD COLUMN IF NOT EXISTS extra_proxies TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE hosts ADD COLUMN IF NOT EXISTS https_port INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE hosts DROP COLUMN IF EXISTS cert_domain`,
		`ALTER TABLE hosts DROP COLUMN IF EXISTS relay_port`,
		`ALTER TABLE nodes ADD COLUMN IF NOT EXISTS tls_mode TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE nodes DROP COLUMN IF EXISTS tls_cert_domain`,
		`CREATE TABLE IF NOT EXISTS node_latency_samples (
			id         BIGSERIAL PRIMARY KEY,
			node_id    TEXT NOT NULL,
			isp        TEXT NOT NULL,
			rtt_ms     INTEGER,
			sampled_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_nls_node_sampled ON node_latency_samples(node_id, sampled_at DESC)`,
		// access_logs：xray access log 审计记录，保留 24 小时
		`CREATE TABLE IF NOT EXISTS access_logs (
			id          BIGSERIAL PRIMARY KEY,
			node_id     TEXT NOT NULL,
			username    TEXT NOT NULL,
			source_ip   TEXT NOT NULL,
			source_port TEXT NOT NULL DEFAULT '',
			destination TEXT NOT NULL,
			remote_ip   TEXT NOT NULL DEFAULT '',
			route_tag   TEXT NOT NULL DEFAULT '',
			protocol    TEXT NOT NULL DEFAULT '',
			inbound_tag TEXT NOT NULL DEFAULT '',
			created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_access_logs_created_at ON access_logs(created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_access_logs_username ON access_logs(username)`,
		`CREATE INDEX IF NOT EXISTS idx_access_logs_node_id ON access_logs(node_id)`,
		`ALTER TABLE nodes ADD COLUMN IF NOT EXISTS is_landing BOOLEAN NOT NULL DEFAULT TRUE`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS uuid TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS secret TEXT NOT NULL DEFAULT ''`,
		// 为空的 user 凭证从其 user_inbounds 任一非空记录回填（一次性补齐，幂等）。
		`UPDATE users u SET uuid = (SELECT ui.uuid FROM user_inbounds ui WHERE ui.user_id = u.id AND ui.uuid != '' LIMIT 1)
		 WHERE u.uuid = '' AND EXISTS (SELECT 1 FROM user_inbounds ui WHERE ui.user_id = u.id AND ui.uuid != '')`,
		`UPDATE users u SET secret = (SELECT ui.secret FROM user_inbounds ui WHERE ui.user_id = u.id AND ui.secret != '' LIMIT 1)
		 WHERE u.secret = '' AND EXISTS (SELECT 1 FROM user_inbounds ui WHERE ui.user_id = u.id AND ui.secret != '')`,
		`ALTER TABLE outbounds ADD COLUMN IF NOT EXISTS flow TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE inbounds DROP COLUMN IF EXISTS target_host`,
		`ALTER TABLE inbounds DROP COLUMN IF EXISTS target_port`,
		`ALTER TABLE nodes DROP COLUMN IF EXISTS audit_enabled`,
		`DROP TABLE IF EXISTS ix_domains`,
		// audit_rules：流量审计告警规则
		`CREATE TABLE IF NOT EXISTS audit_rules (
			id         TEXT PRIMARY KEY,
			type       TEXT NOT NULL,
			value      TEXT NOT NULL,
			enabled    BOOLEAN NOT NULL DEFAULT TRUE,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS password TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS is_admin BOOLEAN NOT NULL DEFAULT FALSE`,
		`CREATE UNIQUE INDEX IF NOT EXISTS users_one_admin ON users(is_admin) WHERE is_admin = TRUE`,
		// 修复历史建表错误：流量相关列在早期版本被建为 INTEGER（int4，溢出阈值 ~2.1GB），
		// 应当为 BIGINT。已存在的列在此原地升级，重复执行幂等。
		`ALTER TABLE nodes ALTER COLUMN upload_bytes TYPE BIGINT`,
		`ALTER TABLE nodes ALTER COLUMN download_bytes TYPE BIGINT`,
		`ALTER TABLE users ALTER COLUMN traffic_limit_bytes TYPE BIGINT`,
		`ALTER TABLE users ALTER COLUMN upload_bytes TYPE BIGINT`,
		`ALTER TABLE users ALTER COLUMN download_bytes TYPE BIGINT`,
		`ALTER TABLE users ALTER COLUMN used_bytes TYPE BIGINT`,
		`ALTER TABLE users ALTER COLUMN raw_upload_bytes TYPE BIGINT`,
		`ALTER TABLE users ALTER COLUMN raw_download_bytes TYPE BIGINT`,
		`ALTER TABLE node_daily_usage ALTER COLUMN upload_bytes TYPE BIGINT`,
		`ALTER TABLE node_daily_usage ALTER COLUMN download_bytes TYPE BIGINT`,
		`ALTER TABLE node_speedtest ALTER COLUMN down_bps TYPE BIGINT`,
		`ALTER TABLE node_speedtest ALTER COLUMN up_bps TYPE BIGINT`,
		`ALTER TABLE user_node_daily_usage ALTER COLUMN upload_bytes TYPE BIGINT`,
		`ALTER TABLE user_node_daily_usage ALTER COLUMN download_bytes TYPE BIGINT`,
		`ALTER TABLE plans ALTER COLUMN traffic_limit TYPE BIGINT`,
		// sub_token 唯一约束：避免手动改写或迁移导入造成重复，导致订阅链接错位。
		// 空字符串不参与唯一性（部分老用户可能历史无 token）。
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_users_sub_token ON users(sub_token) WHERE sub_token <> ''`,
		// portal_sessions：用户门户密码登录 session
		`CREATE TABLE IF NOT EXISTS portal_sessions (
			token      TEXT PRIMARY KEY,
			user_id    TEXT NOT NULL,
			expires_at TIMESTAMPTZ NOT NULL
		)`,
		// enroll_tokens：节点 enrollment 一次性令牌
		`CREATE TABLE IF NOT EXISTS enroll_tokens (
			token        TEXT PRIMARY KEY,
			node_id      TEXT NOT NULL,
			expires_at   TIMESTAMPTZ NOT NULL,
			consumed_at  TIMESTAMPTZ,
			created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_enroll_tokens_expires ON enroll_tokens(expires_at)`,
	}

	for _, stmt := range stmts {
		if _, err := db.conn.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("init postgres schema: %w\nSQL: %s", err, stmt)
		}
	}
	// users username 唯一索引
	if _, err := db.conn.Exec(ctx, `CREATE UNIQUE INDEX IF NOT EXISTS idx_users_username ON users(username)`); err != nil {
		return fmt.Errorf("init postgres schema: create idx_users_username: %w", err)
	}
	if _, err := db.conn.Exec(ctx, `CREATE INDEX IF NOT EXISTS idx_user_inbounds_inbound_id ON user_inbounds(inbound_id)`); err != nil {
		return fmt.Errorf("init postgres schema: create idx_user_inbounds_inbound_id: %w", err)
	}
	return nil
}
