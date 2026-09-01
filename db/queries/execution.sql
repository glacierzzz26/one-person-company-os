-- name: CreateExecution :one
INSERT INTO execution (id, task_id, worker_id, attempt, status, started_at, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetExecution :one
SELECT * FROM execution WHERE id = ?;

-- name: ListExecutions :many
SELECT * FROM execution
WHERE (sqlc.arg('task_filter') = '' OR task_id = sqlc.arg('task_id'))
  AND (sqlc.arg('status_filter') = '' OR status = sqlc.arg('status'))
ORDER BY created_at DESC;

-- name: FinishExecution :one
UPDATE execution
SET status = ?, finished_at = ?, result = ?, error = ?
WHERE id = ?
RETURNING *;
