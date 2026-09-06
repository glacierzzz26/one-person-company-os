-- name: CreateEndpoint :one
INSERT INTO endpoint (id, company_id, name, base_url, token_enc, proto, vendor, selected_model, role, tier, status, models_cache, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetEndpoint :one
SELECT * FROM endpoint WHERE id = ?;

-- name: ListEndpoints :many
SELECT * FROM endpoint WHERE company_id = ? ORDER BY created_at DESC;

-- name: UpdateEndpointModel :one
UPDATE endpoint SET selected_model = ?, models_cache = ?, updated_at = ? WHERE id = ? RETURNING *;

-- name: UpdateEndpointRole :one
UPDATE endpoint SET role = ?, updated_at = ? WHERE id = ? RETURNING *;

-- name: UpdateEndpointTier :one
UPDATE endpoint SET tier = ?, updated_at = ? WHERE id = ? RETURNING *;

-- name: UpdateEndpointStatus :one
UPDATE endpoint SET status = ?, updated_at = ? WHERE id = ? RETURNING *;

-- name: UpdateEndpointToken :one
UPDATE endpoint SET token_enc = ?, updated_at = ? WHERE id = ? RETURNING *;
