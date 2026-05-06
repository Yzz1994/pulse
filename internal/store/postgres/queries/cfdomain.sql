-- name: UpsertCFDomain :exec
INSERT INTO cf_domains (id, cf_token, zone_id, zone_name, remark)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT(id) DO UPDATE SET
    cf_token  = excluded.cf_token,
    zone_id   = excluded.zone_id,
    zone_name = excluded.zone_name,
    remark    = excluded.remark;

-- name: GetCFDomainByID :one
SELECT id, cf_token, zone_id, zone_name, COALESCE(remark, '') AS remark
FROM cf_domains WHERE id = $1;

-- name: ListCFDomains :many
SELECT id, cf_token, zone_id, zone_name, COALESCE(remark, '') AS remark
FROM cf_domains ORDER BY id;

-- name: DeleteCFDomainByID :execresult
DELETE FROM cf_domains WHERE id = $1;
