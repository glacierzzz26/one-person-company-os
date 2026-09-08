-- name: CreatePipeline :one
INSERT INTO pipelines (id, project_id, name, kind, description, risk, status, schedule, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetPipeline :one
SELECT * FROM pipelines WHERE id = ?;

-- name: ListPipelinesByProject :many
SELECT * FROM pipelines WHERE project_id = ? ORDER BY created_at DESC;

-- name: DeletePipeline :execrows
DELETE FROM pipelines WHERE id = ?;

-- name: DeletePipelinesByProject :execrows
DELETE FROM pipelines WHERE project_id = ?;
