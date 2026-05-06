-- name: UpsertNodeDomain :one
INSERT INTO node_domains (id, node_id, cf_domain_id, fqdn, record_type, content, proxied, synced_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
ON CONFLICT(fqdn) DO UPDATE SET
    node_id     = excluded.node_id,
    cf_domain_id = excluded.cf_domain_id,
    record_type = excluded.record_type,
    content     = excluded.content,
    proxied     = excluded.proxied,
    synced_at   = NOW()
RETURNING id, node_id, cf_domain_id, fqdn, record_type, content, proxied, synced_at;

-- name: ListNodeDomains :many
SELECT id, node_id, cf_domain_id, fqdn, record_type, content, proxied, synced_at
FROM node_domains ORDER BY fqdn;

-- name: ListNodeDomainsByCFDomain :many
SELECT id, node_id, cf_domain_id, fqdn, record_type, content, proxied, synced_at
FROM node_domains WHERE cf_domain_id = $1 ORDER BY fqdn;

-- name: UpdateNodeDomainNode :one
UPDATE node_domains SET node_id = $2
WHERE id = $1
RETURNING id, node_id, cf_domain_id, fqdn, record_type, content, proxied, synced_at;

-- name: DeleteNodeDomain :execresult
DELETE FROM node_domains WHERE id = $1;
