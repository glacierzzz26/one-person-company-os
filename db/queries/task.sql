-- name: CreateTask :one
INSERT INTO task (id, company_id, capability_id, workflow_id, agent_id, title, description, status, priority, attempt, risk, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetTask :one
SELECT * FROM task WHERE id = ?;

-- name: ListTasks :many
SELECT * FROM task
WHERE (sqlc.arg('company_filter') = '' OR company_id = sqlc.arg('company_id'))
  AND (sqlc.arg('status_filter') = '' OR status = sqlc.arg('status'))
ORDER BY created_at;
