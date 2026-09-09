-- name: CreateProject :one
INSERT INTO projects (id, company_id, name, root_path, description, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetProject :one
SELECT * FROM projects WHERE id = ?;

-- name: ListProjectsByCompany :many
SELECT * FROM projects WHERE company_id = ? ORDER BY created_at DESC;

-- name: UpdateProject :one
UPDATE projects SET name = ?, description = ?, updated_at = ? WHERE id = ? RETURNING *;

-- name: DeleteProject :execrows
DELETE FROM projects WHERE id = ?;
