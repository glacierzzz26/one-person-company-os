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

-- name: UpdatePipelineSchedule :one
UPDATE pipelines SET schedule = ?, updated_at = ? WHERE id = ? RETURNING *;

-- name: ListScheduledPipelines :many
SELECT id, project_id, name, kind, schedule FROM pipelines
WHERE status = 'active' AND schedule != ''
ORDER BY created_at ASC;
