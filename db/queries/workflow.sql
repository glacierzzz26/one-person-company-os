-- name: CreateWorkflow :one
INSERT INTO workflow (id, company_id, name, description, definition, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetWorkflow :one
SELECT * FROM workflow WHERE id = ?;

-- name: ListWorkflowsByCompany :many
SELECT * FROM workflow WHERE company_id = ? ORDER BY created_at;
