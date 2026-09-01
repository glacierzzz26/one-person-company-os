-- name: CreateCompany :one
INSERT INTO company (id, name, vision, created_at, updated_at)
VALUES (?, ?, ?, ?, ?)
RETURNING *;

-- name: GetCompany :one
SELECT * FROM company WHERE id = ?;

-- name: ListCompanies :many
SELECT * FROM company ORDER BY created_at;
