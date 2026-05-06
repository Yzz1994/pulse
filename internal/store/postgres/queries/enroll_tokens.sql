-- ─── Enroll Tokens ────────────────────────────────────────────────────────────

-- name: InsertEnrollToken :exec
INSERT INTO enroll_tokens (token, node_id, expires_at)
VALUES ($1, $2, $3);

-- name: GetEnrollToken :one
SELECT token, node_id, expires_at, consumed_at, created_at
FROM enroll_tokens
WHERE token = $1;

-- name: ConsumeEnrollToken :execresult
UPDATE enroll_tokens
SET consumed_at = NOW()
WHERE token = $1
  AND consumed_at IS NULL
  AND expires_at > NOW();

-- name: CleanupExpiredEnrollTokens :execresult
DELETE FROM enroll_tokens WHERE expires_at < $1;
