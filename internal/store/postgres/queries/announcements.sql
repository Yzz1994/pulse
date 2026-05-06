-- name: InsertAnnouncement :exec
INSERT INTO announcements (id, title, content, enabled, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: UpdateAnnouncement :exec
UPDATE announcements SET title = $1, content = $2, enabled = $3, updated_at = $4 WHERE id = $5;

-- name: DeleteAnnouncementByID :exec
DELETE FROM announcements WHERE id = $1;

-- name: GetAnnouncementByID :one
SELECT id, title, content, enabled, created_at, updated_at FROM announcements WHERE id = $1;

-- name: ListAnnouncements :many
SELECT id, title, content, enabled, created_at, updated_at
FROM announcements ORDER BY created_at DESC;

-- name: GetActiveAnnouncement :one
SELECT id, title, content, enabled, created_at, updated_at
FROM announcements WHERE enabled = true LIMIT 1;

-- name: DisableAllAnnouncements :exec
UPDATE announcements SET enabled = false, updated_at = $1;

-- name: EnableAnnouncement :exec
UPDATE announcements SET enabled = true, updated_at = $1 WHERE id = $2;

-- name: DisableAnnouncement :exec
UPDATE announcements SET enabled = false, updated_at = $1 WHERE id = $2;
