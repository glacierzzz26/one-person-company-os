-- name: CreateMemory :one
INSERT INTO memory (id, company_id, type, title, content, source, tags, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetMemory :one
SELECT * FROM memory WHERE id = ?;

-- name: ListMemories :many
SELECT * FROM memory
WHERE (sqlc.arg('company_filter') = '' OR company_id = sqlc.arg('company_id'))
  AND (sqlc.arg('type_filter') = '' OR type = sqlc.arg('type'))
ORDER BY created_at DESC;
