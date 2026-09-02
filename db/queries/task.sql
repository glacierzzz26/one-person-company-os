-- name: CreateTask :one
INSERT INTO task (id, company_id, capability_id, workflow_id, agent_id, title, description, tool_name, status, priority, attempt, risk, qstatus, lease_worker_id, lease_until, max_attempts, timeout_sec, last_error, result, workspace_path, parent_task_id, round_no, conflict_count, writer_endpoint_id, reviewer_endpoint_id, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetTask :one
SELECT * FROM task WHERE id = ?;

-- name: ListTasks :many
SELECT * FROM task
WHERE (sqlc.arg('company_filter') = '' OR company_id = sqlc.arg('company_id'))
  AND (sqlc.arg('status_filter') = '' OR status = sqlc.arg('status'))
  AND (sqlc.arg('risk_filter') = '' OR risk = sqlc.arg('risk'))
  AND (sqlc.arg('attempt_filter') = -1 OR attempt >= sqlc.arg('attempt_filter'))
ORDER BY created_at;

-- name: ClaimTask :one
UPDATE task
SET qstatus = 'leased', lease_worker_id = ?, lease_until = ?, updated_at = ?
WHERE id = ? AND qstatus = 'ready'
RETURNING *;

-- name: LeaseNextTask :one
UPDATE task
SET qstatus = 'leased', lease_worker_id = ?, lease_until = ?, updated_at = ?
WHERE id = (SELECT id FROM task WHERE qstatus = 'ready' ORDER BY priority DESC, created_at ASC LIMIT 1)
RETURNING *;

-- name: MarkTaskRunning :one
UPDATE task SET qstatus = 'running', status = 'running', updated_at = ? WHERE id = ? RETURNING *;

-- name: CompleteTask :one
UPDATE task SET qstatus = 'completed', status = 'completed', result = ?, updated_at = ? WHERE id = ? RETURNING *;

-- name: FailTask :one
UPDATE task SET qstatus = 'failed', status = 'failed', last_error = ?, updated_at = ? WHERE id = ? RETURNING *;

-- name: RequeueTask :one
UPDATE task
SET qstatus = 'ready', status = 'pending', attempt = attempt + 1,
    lease_worker_id = '', lease_until = 0, last_error = ?, updated_at = ?
WHERE id = ?
RETURNING *;

-- name: UpdateTaskRound :one
UPDATE task
SET round_no = ?, conflict_count = ?, updated_at = ?
WHERE id = ?
RETURNING *;

-- name: RecoverLeasedTasks :many
UPDATE task
SET qstatus = 'ready', status = 'pending', lease_worker_id = '', lease_until = 0, updated_at = ?
WHERE qstatus = 'leased' AND lease_until != 0 AND lease_until < ?
RETURNING *;

-- name: RequestApprovalTask :one
UPDATE task
SET qstatus = 'waiting_approval', status = 'waiting_approval',
    lease_worker_id = '', lease_until = 0, updated_at = ?
WHERE id = ?
RETURNING *;

-- name: ApproveTask :one
UPDATE task
SET qstatus = 'ready', status = 'pending',
    lease_worker_id = '', lease_until = 0, last_error = '', updated_at = ?
WHERE id = ?
RETURNING *;

-- name: ListRecentTasks :many
SELECT * FROM task
WHERE company_id = ?
ORDER BY created_at DESC
LIMIT ?;

-- name: TaskStatusCountsByWorkflow :many
SELECT workflow_id, status, COUNT(*) AS cnt
FROM task
WHERE company_id = ? AND workflow_id IS NOT NULL
GROUP BY workflow_id, status
ORDER BY workflow_id;
