-- name: InsertTicket :exec
INSERT INTO tickets (id, user_id, username, title, status, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: GetTicketByID :one
SELECT id, user_id, username, title, status, created_at, updated_at
FROM tickets WHERE id = $1;

-- name: ListTickets :many
SELECT id, user_id, username, title, status, created_at, updated_at
FROM tickets ORDER BY updated_at DESC;

-- name: ListTicketsByUser :many
SELECT id, user_id, username, title, status, created_at, updated_at
FROM tickets WHERE user_id = $1 ORDER BY updated_at DESC;

-- name: UpdateTicketStatus :exec
UPDATE tickets SET status = $1, updated_at = $2 WHERE id = $3;

-- name: UpdateTicketUpdatedAt :exec
UPDATE tickets SET updated_at = $1 WHERE id = $2;

-- name: InsertTicketMessage :exec
INSERT INTO ticket_messages (id, ticket_id, content, is_admin, created_at)
VALUES ($1, $2, $3, $4, $5);

-- name: ListTicketMessages :many
SELECT id, ticket_id, content, is_admin, created_at
FROM ticket_messages WHERE ticket_id = $1 ORDER BY created_at ASC;

-- name: InsertTicketImage :exec
INSERT INTO ticket_images (id, ticket_id, filename, stored_name, size, created_at)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: ListTicketImages :many
SELECT id, ticket_id, filename, stored_name, size, created_at
FROM ticket_images WHERE ticket_id = $1 ORDER BY created_at ASC;
