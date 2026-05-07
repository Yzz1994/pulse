-- ─── Nodes ────────────────────────────────────────────────────────────────────

-- name: UpsertNode :exec
INSERT INTO nodes (id, name, base_url, expire_at, panel_url, remark, ip_override, disabled,
                   acme_email, panel_domain, extra_proxies, https_port,
                   is_landing)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
ON CONFLICT(id) DO UPDATE SET
    name          = excluded.name,
    base_url      = excluded.base_url,
    expire_at     = excluded.expire_at,
    panel_url     = excluded.panel_url,
    remark        = excluded.remark,
    ip_override   = excluded.ip_override,
    disabled      = excluded.disabled,
    acme_email    = excluded.acme_email,
    panel_domain  = excluded.panel_domain,
    extra_proxies = excluded.extra_proxies,
    https_port    = excluded.https_port,
    is_landing    = excluded.is_landing;

-- name: DeleteNodeByID :execresult
DELETE FROM nodes WHERE id = $1;

-- name: GetNodeByID :one
SELECT id, name, base_url, upload_bytes, download_bytes,
       COALESCE(acme_email, '') AS acme_email,
       COALESCE(panel_domain, '') AS panel_domain,
       COALESCE(extra_proxies, '') AS extra_proxies,
       COALESCE(https_port, 0) AS https_port,
       expire_at, panel_url, remark,
       COALESCE(ip_override, '') AS ip_override,
       COALESCE(disabled, 0) AS disabled,
       COALESCE(is_landing, TRUE) AS is_landing
FROM nodes
WHERE id = $1;

-- name: ListNodes :many
SELECT id, name, base_url, upload_bytes, download_bytes,
       COALESCE(acme_email, '') AS acme_email,
       COALESCE(panel_domain, '') AS panel_domain,
       COALESCE(extra_proxies, '') AS extra_proxies,
       COALESCE(https_port, 0) AS https_port,
       expire_at, panel_url, remark,
       COALESCE(ip_override, '') AS ip_override,
       COALESCE(disabled, 0) AS disabled,
       COALESCE(is_landing, TRUE) AS is_landing
FROM nodes
ORDER BY id;

-- name: AddNodeTraffic :exec
UPDATE nodes
SET upload_bytes   = upload_bytes   + $1,
    download_bytes = download_bytes + $2
WHERE id = $3;

-- ─── node_daily_usage ─────────────────────────────────────────────────────────

-- name: UpsertNodeDailyUsage :exec
INSERT INTO node_daily_usage (node_id, date, upload_bytes, download_bytes)
VALUES ($1, $2, $3, $4)
ON CONFLICT(node_id, date) DO UPDATE SET
    upload_bytes   = node_daily_usage.upload_bytes   + excluded.upload_bytes,
    download_bytes = node_daily_usage.download_bytes + excluded.download_bytes;

-- name: ListNodeDailyUsage :many
SELECT node_id, date, upload_bytes, download_bytes
FROM node_daily_usage
WHERE date >= $1
ORDER BY date ASC;

-- name: DeleteOldNodeDailyUsage :exec
DELETE FROM node_daily_usage WHERE date < $1;

-- ─── node_speedtest ───────────────────────────────────────────────────────────

-- name: UpsertNodeSpeedTest :exec
INSERT INTO node_speedtest (node_id, down_bps, up_bps, tested_at)
VALUES ($1, $2, $3, $4)
ON CONFLICT(node_id) DO UPDATE SET
    down_bps  = excluded.down_bps,
    up_bps    = excluded.up_bps,
    tested_at = excluded.tested_at;

-- name: ListAllNodeSpeedTests :many
SELECT node_id, down_bps, up_bps, tested_at FROM node_speedtest;

-- ─── node_check_results ───────────────────────────────────────────────────────

-- name: DeleteNodeCheckResults :exec
DELETE FROM node_check_results WHERE node_id = $1;

-- name: InsertNodeCheckResult :exec
INSERT INTO node_check_results (node_id, service, check_type, unlocked, region, note, checked_at)
VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: ListAllNodeCheckResults :many
SELECT node_id, service, check_type, unlocked, region, note, checked_at
FROM node_check_results
ORDER BY node_id, check_type, service;

-- ─── node_uptime_log ──────────────────────────────────────────────────────────

-- name: InsertNodeUptime :exec
INSERT INTO node_uptime_log (node_id, checked_at, online, running)
VALUES ($1, $2, $3, $4)
ON CONFLICT (node_id, checked_at) DO NOTHING;

-- name: ListNodeUptimeSummary :many
SELECT node_id,
       COUNT(*)::bigint        AS total_checks,
       SUM(online)::bigint     AS online_sum,
       SUM(running)::bigint    AS running_sum,
       MIN(checked_at)::text   AS first_at
FROM node_uptime_log
WHERE checked_at >= $1
GROUP BY node_id;

-- name: DeleteOldNodeUptime :exec
DELETE FROM node_uptime_log WHERE checked_at < $1;

-- name: ListNodeUptimeBars :many
SELECT node_id,
       to_char(date_trunc('hour', checked_at::timestamptz), 'YYYY-MM-DD HH24') AS period,
       COUNT(*)::bigint    AS total,
       SUM(online)::bigint AS online_sum
FROM node_uptime_log
WHERE checked_at >= $1
GROUP BY node_id, period
ORDER BY node_id, period;

-- ─── traceroute_snapshots ─────────────────────────────────────────────────────

-- name: InsertTracerouteSnapshot :exec
INSERT INTO traceroute_snapshots (id, node_id, direction, target, hops, quality, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: ListNodeTracerouteSnapshots :many
SELECT id, node_id, direction, target, hops, quality, created_at
FROM traceroute_snapshots
WHERE node_id = $1
ORDER BY created_at DESC
LIMIT $2;

-- name: ListLatestTracerouteSnapshots :many
SELECT id, node_id, direction, target, hops, quality, created_at
FROM traceroute_snapshots t1
WHERE NOT EXISTS (
    SELECT 1 FROM traceroute_snapshots t2
    WHERE t2.node_id   = t1.node_id
      AND t2.direction = t1.direction
      AND t2.target    = t1.target
      AND (t2.created_at > t1.created_at
           OR (t2.created_at = t1.created_at AND t2.id > t1.id))
)
ORDER BY node_id, direction, created_at DESC;

-- name: DeleteTracerouteSnapshot :exec
DELETE FROM traceroute_snapshots WHERE id = $1;
