-- name: CreateAgent :one
INSERT INTO agent (id, capability_id, name, role, model_hint, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetAgent :one
SELECT * FROM agent WHERE id = ?;

-- name: ListAgentsByCapability :many
SELECT * FROM agent WHERE capability_id = ? ORDER BY name;
