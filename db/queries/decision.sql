-- name: CreateDecision :one
INSERT INTO decision (id, company_id, title, kind, status, body, decided_by, source, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetDecision :one
SELECT * FROM decision WHERE id = ?;

-- name: ListDecisions :many
SELECT * FROM decision
WHERE (sqlc.arg('company_filter') = '' OR company_id = sqlc.arg('company_id'))
  AND (sqlc.arg('kind_filter') = '' OR kind = sqlc.arg('kind'))
ORDER BY created_at DESC;
