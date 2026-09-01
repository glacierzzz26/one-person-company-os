-- name: CreateAudit :one
INSERT INTO audit (id, entity_type, entity_id, action, actor, detail, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: ListAudits :many
SELECT * FROM audit
WHERE (sqlc.arg('entity_type_filter') = '' OR entity_type = sqlc.arg('entity_type'))
ORDER BY created_at DESC;
