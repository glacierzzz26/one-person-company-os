-- name: CreatePolicy :one
INSERT INTO policy (id, company_id, name, kind, statement, enabled, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetPolicy :one
SELECT * FROM policy WHERE id = ?;

-- name: ListPoliciesByCompany :many
SELECT * FROM policy WHERE company_id = ? ORDER BY created_at;
