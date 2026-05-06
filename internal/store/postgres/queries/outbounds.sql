-- name: UpsertOutbound :exec
INSERT INTO outbounds (id, name, protocol, server, username, password, method, uuid, sni, public_key, short_id, fingerprint, flow)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
ON CONFLICT(id) DO UPDATE SET
    name        = excluded.name,
    protocol    = excluded.protocol,
    server      = excluded.server,
    username    = excluded.username,
    password    = excluded.password,
    method      = excluded.method,
    uuid        = excluded.uuid,
    sni         = excluded.sni,
    public_key  = excluded.public_key,
    short_id    = excluded.short_id,
    fingerprint = excluded.fingerprint,
    flow        = excluded.flow;

-- name: GetOutboundByID :one
SELECT id, name, protocol, server, username, password, method, uuid, sni, public_key, short_id, fingerprint, COALESCE(flow, '') AS flow
FROM outbounds WHERE id = $1;

-- name: ListOutbounds :many
SELECT id, name, protocol, server, username, password, method, uuid, sni, public_key, short_id, fingerprint, COALESCE(flow, '') AS flow
FROM outbounds ORDER BY id;

-- name: DeleteOutboundByID :execresult
DELETE FROM outbounds WHERE id = $1;
