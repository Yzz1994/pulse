package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"pulse/internal/accesslogs"
)

type AccessLogStore struct {
	db *pgxpool.Pool
}

func (s *AccessLogStore) Insert(entries []accesslogs.Entry) error {
	if len(entries) == 0 {
		return nil
	}
	ctx := context.Background()
	const cols = 10
	args := make([]any, 0, len(entries)*cols)
	placeholders := make([]string, 0, len(entries))
	for i, e := range entries {
		base := i * cols
		placeholders = append(placeholders, fmt.Sprintf(
			"($%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d)",
			base+1, base+2, base+3, base+4, base+5, base+6, base+7, base+8, base+9, base+10,
		))
		args = append(args,
			e.NodeID, e.Username, e.SourceIP, e.SourcePort,
			e.Destination, e.RemoteIP, e.RouteTag, e.Protocol, e.InboundTag, e.CreatedAt,
		)
	}
	query := `INSERT INTO access_logs (node_id,username,source_ip,source_port,destination,remote_ip,route_tag,protocol,inbound_tag,created_at) VALUES ` +
		strings.Join(placeholders, ",")
	_, err := s.db.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("insert access logs: %w", err)
	}
	return nil
}

func (s *AccessLogStore) List(params accesslogs.ListParams) ([]accesslogs.Entry, error) {
	ctx := context.Background()
	where := []string{"1=1"}
	args := []any{}
	idx := 1

	if params.NodeID != "" {
		where = append(where, fmt.Sprintf("node_id = $%d", idx))
		args = append(args, params.NodeID)
		idx++
	}
	if params.Username != "" {
		where = append(where, fmt.Sprintf("SPLIT_PART(username, '@', 1) = $%d", idx))
		args = append(args, params.Username)
		idx++
	}
	if !params.Since.IsZero() {
		where = append(where, fmt.Sprintf("created_at >= $%d", idx))
		args = append(args, params.Since)
		idx++
	}
	if !params.Until.IsZero() {
		where = append(where, fmt.Sprintf("created_at <= $%d", idx))
		args = append(args, params.Until)
		idx++
	}

	limit := 0
	if params.Limit > 0 {
		limit = params.Limit
	}

	const selectCols = `id,node_id,username,source_ip,source_port,destination,remote_ip,route_tag,protocol,inbound_tag,created_at`
	var query string
	if limit > 0 {
		query = fmt.Sprintf(
			`SELECT %s FROM access_logs WHERE %s ORDER BY created_at DESC LIMIT %d OFFSET %d`,
			selectCols, strings.Join(where, " AND "), limit, params.Offset,
		)
	} else {
		query = fmt.Sprintf(
			`SELECT %s FROM access_logs WHERE %s ORDER BY created_at DESC OFFSET %d`,
			selectCols, strings.Join(where, " AND "), params.Offset,
		)
	}

	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list access logs: %w", err)
	}
	defer rows.Close()

	var out []accesslogs.Entry
	for rows.Next() {
		var e accesslogs.Entry
		if err := rows.Scan(
			&e.ID, &e.NodeID, &e.Username, &e.SourceIP, &e.SourcePort,
			&e.Destination, &e.RemoteIP, &e.RouteTag, &e.Protocol, &e.InboundTag, &e.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan access log: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ListDistinctUsers 返回最近 since 时间段内出现过的 distinct username（截取 @ 前缀，去重）。
func (s *AccessLogStore) ListDistinctUsers(since time.Time) ([]string, error) {
	ctx := context.Background()
	rows, err := s.db.Query(ctx,
		`SELECT DISTINCT SPLIT_PART(username, '@', 1) FROM access_logs WHERE created_at >= $1 ORDER BY 1`,
		since,
	)
	if err != nil {
		return nil, fmt.Errorf("list distinct users: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *AccessLogStore) ListUserAnalysis(since, until time.Time) ([]accesslogs.UserAnalysis, error) {
	ctx := context.Background()
	// access_logs 按用户聚合连接数和独立 IP，JOIN user_node_daily_usage 汇总流量
	const query = `
WITH access_stats AS (
  SELECT
    SPLIT_PART(username, '@', 1)   AS username,
    COUNT(*)                        AS connections,
    COUNT(DISTINCT source_ip)       AS distinct_ips,
    MAX(created_at)                 AS last_seen
  FROM access_logs
  WHERE created_at >= $1 AND created_at <= $2
  GROUP BY 1
),
traffic_stats AS (
  SELECT
    u.username,
    COALESCE(SUM(unu.upload_bytes + unu.download_bytes), 0) AS total_bytes
  FROM user_node_daily_usage unu
  JOIN users u ON u.id = unu.user_id
  WHERE unu.date >= TO_CHAR($1::timestamptz AT TIME ZONE 'UTC', 'YYYY-MM-DD')
    AND unu.date <= TO_CHAR($2::timestamptz AT TIME ZONE 'UTC', 'YYYY-MM-DD')
  GROUP BY u.username
)
SELECT
  a.username,
  a.connections,
  a.distinct_ips,
  COALESCE(t.total_bytes, 0) AS total_bytes,
  a.last_seen
FROM access_stats a
LEFT JOIN traffic_stats t ON t.username = a.username
ORDER BY a.distinct_ips DESC, total_bytes DESC`

	rows, err := s.db.Query(ctx, query, since, until)
	if err != nil {
		return nil, fmt.Errorf("list user analysis: %w", err)
	}
	defer rows.Close()
	var out []accesslogs.UserAnalysis
	for rows.Next() {
		var a accesslogs.UserAnalysis
		if err := rows.Scan(&a.Username, &a.Connections, &a.DistinctIPs, &a.TotalBytes, &a.LastSeen); err != nil {
			return nil, fmt.Errorf("scan user analysis: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *AccessLogStore) Count() (int64, error) {
	ctx := context.Background()
	var n int64
	err := s.db.QueryRow(ctx, `SELECT COUNT(*) FROM access_logs`).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count access logs: %w", err)
	}
	return n, nil
}

func (s *AccessLogStore) DeleteOlderThan(t time.Time) (int64, error) {
	ctx := context.Background()
	tag, err := s.db.Exec(ctx, `DELETE FROM access_logs WHERE created_at < $1`, t)
	if err != nil {
		return 0, fmt.Errorf("delete old access logs: %w", err)
	}
	return tag.RowsAffected(), nil
}
