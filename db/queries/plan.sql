-- name: CreateTaskPlan :one
INSERT INTO task_plan (id, task_id, kind, materialized, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetTaskPlanByTask :one
SELECT * FROM task_plan WHERE task_id = ?;

-- name: SetPlanUpdated :exec
UPDATE task_plan SET updated_at = ? WHERE id = ?;

-- name: CreatePlanPhase :one
INSERT INTO task_plan_phase (id, plan_id, seq, kind, title, allocator, status, evidence, note, started_at, finished_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetPlanPhase :one
SELECT * FROM task_plan_phase WHERE id = ?;

-- name: ListPlanPhases :many
SELECT * FROM task_plan_phase WHERE plan_id = ? ORDER BY seq ASC;

-- name: ReplacePlanPhase :one
UPDATE task_plan_phase
SET status = ?, evidence = ?, note = ?, started_at = ?, finished_at = ?
WHERE id = ?
RETURNING *;
