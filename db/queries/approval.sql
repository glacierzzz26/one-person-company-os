-- name: CreateApproval :one
INSERT INTO approval (id, task_id, risk, reason, status, requested_by, created_at)
VALUES (?, ?, ?, ?, 'pending', ?, ?)
RETURNING *;

-- name: GetApproval :one
SELECT * FROM approval WHERE id = ?;

-- name: ListApprovals :many
SELECT * FROM approval
WHERE (sqlc.arg('status_filter') = '' OR status = sqlc.arg('status'))
ORDER BY created_at;

-- name: UpdateApproval :one
UPDATE approval
SET status = ?, decided_by = ?, decision_note = ?, decided_at = ?
WHERE id = ?
RETURNING *;

-- name: HasApprovedApproval :one
SELECT COUNT(*) FROM approval WHERE task_id = ? AND status = 'approved';
