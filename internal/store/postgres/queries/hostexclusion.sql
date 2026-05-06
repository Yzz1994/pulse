-- name: ListHostExclusionsByUser :many
SELECT host_id FROM user_host_exclusions WHERE user_id = $1;

-- name: SetHostExclusion :exec
INSERT INTO user_host_exclusions (user_id, host_id)
VALUES ($1, $2) ON CONFLICT DO NOTHING;

-- name: ClearHostExclusion :exec
DELETE FROM user_host_exclusions WHERE user_id = $1 AND host_id = $2;

-- name: ClearAllHostExclusionsByUser :exec
DELETE FROM user_host_exclusions WHERE user_id = $1;
