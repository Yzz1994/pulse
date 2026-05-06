-- name: UpsertRouteRule :exec
INSERT INTO route_rules (id, name, rule_type, patterns, outbound_id, priority, rule_set_url, rule_set_format, node_ids, inbound_ids)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
ON CONFLICT(id) DO UPDATE SET
    name            = excluded.name,
    rule_type       = excluded.rule_type,
    patterns        = excluded.patterns,
    outbound_id     = excluded.outbound_id,
    priority        = excluded.priority,
    rule_set_url    = excluded.rule_set_url,
    rule_set_format = excluded.rule_set_format,
    node_ids        = excluded.node_ids,
    inbound_ids     = excluded.inbound_ids;

-- name: GetRouteRuleByID :one
SELECT id, name, rule_type, patterns, outbound_id, priority, rule_set_url, rule_set_format, node_ids, inbound_ids
FROM route_rules WHERE id = $1;

-- name: ListRouteRules :many
SELECT id, name, rule_type, patterns, outbound_id, priority, rule_set_url, rule_set_format, node_ids, inbound_ids
FROM route_rules ORDER BY priority, id;

-- name: DeleteRouteRuleByID :execresult
DELETE FROM route_rules WHERE id = $1;
