-- name: CreateRepo :one
INSERT INTO repos (id, company_id, project_id, name, repo_url, workspace_path, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetRepo :one
SELECT * FROM repos WHERE id = ?;

-- name: GetRepoByProject :one
SELECT * FROM repos WHERE project_id = ? LIMIT 1;

-- name: ListRepos :many
SELECT * FROM repos WHERE company_id = ? ORDER BY created_at DESC;

-- name: ListAllRepos :many
SELECT * FROM repos ORDER BY company_id, created_at DESC;

-- name: SetRepoProject :execrows
UPDATE repos SET project_id = ? WHERE id = ?;

-- name: SetRepoSource :execrows
UPDATE repos SET name = ?, repo_url = ?, workspace_path = ? WHERE id = ?;

-- name: ClearRepoProject :execrows
UPDATE repos SET project_id = NULL WHERE project_id = ?;

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

-- name: GetIssueSyncByTask :one
SELECT * FROM issue_sync WHERE task_id = ? LIMIT 1;
