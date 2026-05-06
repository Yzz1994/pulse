-- name: UpsertUserGroup :exec
INSERT INTO user_groups (id, name, remark, inbound_ids)
VALUES ($1, $2, $3, $4)
ON CONFLICT(id) DO UPDATE SET
    name        = excluded.name,
    remark      = excluded.remark,
    inbound_ids = excluded.inbound_ids;

-- name: GetUserGroupByID :one
SELECT id, name, remark, inbound_ids FROM user_groups WHERE id = $1;

-- name: ListUserGroups :many
SELECT id, name, remark, inbound_ids FROM user_groups ORDER BY name;

-- name: DeleteUserGroupByID :execresult
DELETE FROM user_groups WHERE id = $1;

-- name: AddUserGroupMember :exec
INSERT INTO user_group_members (group_id, user_id) VALUES ($1, $2) ON CONFLICT DO NOTHING;

-- name: RemoveUserGroupMember :exec
DELETE FROM user_group_members WHERE group_id = $1 AND user_id = $2;

-- name: ListUserGroupMembers :many
SELECT user_id FROM user_group_members WHERE group_id = $1 ORDER BY user_id;

-- name: ListUserGroupsByUser :many
SELECT group_id FROM user_group_members WHERE user_id = $1;

-- name: DeleteAllUserGroupMembers :exec
DELETE FROM user_group_members WHERE group_id = $1;
