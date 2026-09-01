-- name: CreateCapability :one
INSERT INTO capability (id, company_id, code, name, description, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetCapability :one
SELECT * FROM capability WHERE id = ?;

-- name: GetCapabilityByCode :one
SELECT * FROM capability WHERE company_id = ? AND code = ?;

-- name: ListCapabilitiesByCompany :many
SELECT * FROM capability WHERE company_id = ? ORDER BY code;
