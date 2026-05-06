-- ─── Inbounds ─────────────────────────────────────────────────────────────────

-- name: UpsertInbound :exec
INSERT INTO inbounds (
    id, node_id, protocol, tag, port,
    method, password, security, reality_private_key, reality_public_key,
    reality_handshake_addr, reality_short_id, outbound_id, traffic_rate,
    target_host, target_port
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
ON CONFLICT(id) DO UPDATE SET
    node_id                = excluded.node_id,
    protocol               = excluded.protocol,
    tag                    = excluded.tag,
    port                   = excluded.port,
    method                 = excluded.method,
    password               = excluded.password,
    security               = excluded.security,
    reality_private_key    = excluded.reality_private_key,
    reality_public_key     = excluded.reality_public_key,
    reality_handshake_addr = excluded.reality_handshake_addr,
    reality_short_id       = excluded.reality_short_id,
    outbound_id            = excluded.outbound_id,
    traffic_rate           = excluded.traffic_rate,
    target_host            = excluded.target_host,
    target_port            = excluded.target_port;

-- name: GetInboundByID :one
SELECT id, node_id, protocol, tag, port,
       method, password, security, reality_private_key, reality_public_key,
       reality_handshake_addr, reality_short_id, outbound_id, traffic_rate,
       target_host, target_port
FROM inbounds WHERE id = $1;

-- name: ListInbounds :many
SELECT id, node_id, protocol, tag, port,
       method, password, security, reality_private_key, reality_public_key,
       reality_handshake_addr, reality_short_id, outbound_id, traffic_rate,
       target_host, target_port
FROM inbounds ORDER BY id;

-- name: ListInboundsByNode :many
SELECT id, node_id, protocol, tag, port,
       method, password, security, reality_private_key, reality_public_key,
       reality_handshake_addr, reality_short_id, outbound_id, traffic_rate,
       target_host, target_port
FROM inbounds WHERE node_id = $1 ORDER BY id;

-- name: DeleteInboundByID :execresult
DELETE FROM inbounds WHERE id = $1;

-- name: DeleteUserInboundsByInboundID :exec
DELETE FROM user_inbounds WHERE inbound_id = $1;

-- ─── Hosts ────────────────────────────────────────────────────────────────────

-- name: UpsertHost :exec
INSERT INTO hosts (
    id, inbound_id, remark, address, port, sni, host, path,
    security, alpn, fingerprint, allow_insecure, mux_enable,
    reality_public_key, reality_short_id, reality_spider_x,
    country, region, network, entry, tags, relay_node_id, https_port
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23)
ON CONFLICT(id) DO UPDATE SET
    inbound_id         = excluded.inbound_id,
    remark             = excluded.remark,
    address            = excluded.address,
    port               = excluded.port,
    sni                = excluded.sni,
    host               = excluded.host,
    path               = excluded.path,
    security           = excluded.security,
    alpn               = excluded.alpn,
    fingerprint        = excluded.fingerprint,
    allow_insecure     = excluded.allow_insecure,
    mux_enable         = excluded.mux_enable,
    reality_public_key = excluded.reality_public_key,
    reality_short_id   = excluded.reality_short_id,
    reality_spider_x   = excluded.reality_spider_x,
    country            = excluded.country,
    region             = excluded.region,
    network            = excluded.network,
    entry              = excluded.entry,
    tags               = excluded.tags,
    relay_node_id      = excluded.relay_node_id,
    https_port   = excluded.https_port;

-- name: GetHostByID :one
SELECT id, inbound_id, remark, address, port, sni, host, path,
       security, alpn, fingerprint, allow_insecure, mux_enable,
       reality_public_key, reality_short_id, reality_spider_x,
       country, region, network, entry, tags, relay_node_id, https_port
FROM hosts WHERE id = $1;

-- name: ListHosts :many
SELECT id, inbound_id, remark, address, port, sni, host, path,
       security, alpn, fingerprint, allow_insecure, mux_enable,
       reality_public_key, reality_short_id, reality_spider_x,
       country, region, network, entry, tags, relay_node_id, https_port
FROM hosts ORDER BY id;

-- name: ListHostsByInbound :many
SELECT id, inbound_id, remark, address, port, sni, host, path,
       security, alpn, fingerprint, allow_insecure, mux_enable,
       reality_public_key, reality_short_id, reality_spider_x,
       country, region, network, entry, tags, relay_node_id, https_port
FROM hosts WHERE inbound_id = $1 ORDER BY id;

-- name: DeleteHostByID :execresult
DELETE FROM hosts WHERE id = $1;

