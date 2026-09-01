-- name: CreatePermission :one
INSERT INTO permission (id, policy_id, subject, action, resource, created_at)
VALUES (?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: ListPermissionsByPolicy :many
SELECT * FROM permission WHERE policy_id = ? ORDER BY created_at;

-- name: CheckPermission :one
SELECT COUNT(*) FROM permission p
JOIN policy po ON po.id = p.policy_id
WHERE p.subject = ? AND p.action = ? AND p.resource = ? AND po.enabled = 1 AND po.kind = 'allow';
