-- name: CreateAgent :one
INSERT INTO agent (id, capability_id, name, role, model_hint, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetAgent :one
SELECT * FROM agent WHERE id = ?;

-- name: ListAgentsByCapability :many
SELECT * FROM agent WHERE capability_id = ? ORDER BY name;

-- name: GetAgentByRole :one
SELECT a.id, a.capability_id, a.name, a.role, a.model_hint, a.created_at, a.updated_at
FROM agent a
JOIN capability c ON c.id = a.capability_id
WHERE c.company_id = ? AND a.role = ?
ORDER BY a.created_at
LIMIT 1;

-- name: GetAgentByCapabilityAndRole :one
SELECT a.id, a.capability_id, a.name, a.role, a.model_hint, a.created_at, a.updated_at
FROM agent a
WHERE a.capability_id = ? AND a.role = ?
ORDER BY a.created_at
LIMIT 1;
