-- name: ListEnabledIPSentinelConfigs :many
SELECT * FROM ip_sentinel_configs WHERE enable_google = true OR enable_trust = true;

-- name: UpsertIPSentinelConfig :exec
INSERT INTO ip_sentinel_configs (node_id, region_code, region_name, base_lat, base_lon, lang_params, valid_url_suffix, enable_google, enable_trust, white_urls, keywords, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
ON CONFLICT (node_id) DO UPDATE SET
    region_code      = EXCLUDED.region_code,
    region_name      = EXCLUDED.region_name,
    base_lat         = EXCLUDED.base_lat,
    base_lon         = EXCLUDED.base_lon,
    lang_params      = EXCLUDED.lang_params,
    valid_url_suffix = EXCLUDED.valid_url_suffix,
    enable_google    = EXCLUDED.enable_google,
    enable_trust     = EXCLUDED.enable_trust,
    white_urls       = EXCLUDED.white_urls,
    keywords         = EXCLUDED.keywords,
    updated_at       = EXCLUDED.updated_at;

-- name: GetIPSentinelConfig :one
SELECT * FROM ip_sentinel_configs WHERE node_id = $1;

-- name: InsertIPSentinelRun :exec
INSERT INTO ip_sentinel_runs (id, node_id, task_type, triggered_by, status, output, result, started_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: UpdateIPSentinelRun :exec
UPDATE ip_sentinel_runs SET status = $1, output = $2, result = $3, finished_at = $4 WHERE id = $5;

-- name: ListIPSentinelRuns :many
SELECT * FROM ip_sentinel_runs WHERE node_id = $1 ORDER BY started_at DESC LIMIT 20;
