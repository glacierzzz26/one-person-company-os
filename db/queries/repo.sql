-- name: CreateRepo :one
INSERT INTO repos (id, company_id, name, repo_url, workspace_path, created_at)
VALUES (?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetRepo :one
SELECT * FROM repos WHERE id = ?;

-- name: ListRepos :many
SELECT * FROM repos WHERE company_id = ? ORDER BY created_at DESC;

-- name: ListAllRepos :many
SELECT * FROM repos ORDER BY company_id, created_at DESC;

-- name: UpsertIssueSync :one
INSERT INTO issue_sync (id, company_id, repo_id, issue_number, title, disposition, task_id, note, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(repo_id, issue_number) DO UPDATE SET
    title = excluded.title,
    disposition = excluded.disposition,
    task_id = excluded.task_id,
    note = excluded.note
RETURNING *;

-- name: GetIssueSync :one
SELECT * FROM issue_sync WHERE repo_id = ? AND issue_number = ?;

-- name: ListIssueSync :many
SELECT * FROM issue_sync WHERE company_id = ? ORDER BY created_at DESC;
