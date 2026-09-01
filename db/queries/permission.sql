-- name: CreatePermission :one
INSERT INTO permission (id, policy_id, subject, action, resource, created_at)
VALUES (?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: ListPermissionsByPolicy :many
SELECT * FROM permission WHERE policy_id = ? ORDER BY created_at;
