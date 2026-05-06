-- name: UpsertIXDomain :exec
INSERT INTO ix_domains (id, name, domain, remark)
VALUES ($1, $2, $3, $4)
ON CONFLICT(id) DO UPDATE SET
    name   = excluded.name,
    domain = excluded.domain,
    remark = excluded.remark;

-- name: GetIXDomainByID :one
SELECT id, name, domain, COALESCE(remark, '') AS remark
FROM ix_domains WHERE id = $1;

-- name: ListIXDomains :many
SELECT id, name, domain, COALESCE(remark, '') AS remark
FROM ix_domains ORDER BY name;

-- name: DeleteIXDomainByID :execresult
DELETE FROM ix_domains WHERE id = $1;
