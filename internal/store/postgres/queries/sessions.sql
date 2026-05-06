-- name: UpsertSession :exec
INSERT INTO sessions (token, username, created_at) VALUES ($1, $2, $3)
ON CONFLICT(token) DO UPDATE SET username = excluded.username, created_at = excluded.created_at;

-- name: GetSessionUsername :one
SELECT username FROM sessions WHERE token = $1;

-- name: DeleteSession :exec
DELETE FROM sessions WHERE token = $1;
