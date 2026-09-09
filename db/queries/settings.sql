-- name: GetAppSetting :one
SELECT * FROM app_setting WHERE id = ?;

-- name: InsertAppSetting :one
INSERT INTO app_setting (id, engine_mode_default, agent_cli_default, console_token_hash, digest_time, http_port, poll_min, queue_work, queue_interval_sec, schedule_poll_sec, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: UpdateAppSetting :one
UPDATE app_setting SET engine_mode_default = ?, agent_cli_default = ?, console_token_hash = ?, digest_time = ?, http_port = ?, poll_min = ?, queue_work = ?, queue_interval_sec = ?, schedule_poll_sec = ?, updated_at = ? WHERE id = ? RETURNING *;

-- name: GetCompanySetting :one
SELECT * FROM company_setting WHERE company_id = ?;

-- name: InsertCompanySetting :one
INSERT INTO company_setting (company_id, engine_mode, agent_cli, issue_source, issue_fixture_path, updated_at)
VALUES (?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: UpdateCompanySetting :one
UPDATE company_setting SET engine_mode = ?, agent_cli = ?, issue_source = ?, issue_fixture_path = ?, updated_at = ? WHERE company_id = ? RETURNING *;

-- name: DeleteCompanySetting :exec
DELETE FROM company_setting WHERE company_id = ?;

-- name: GetSecret :one
SELECT * FROM secret WHERE company_id = ? AND id = ?;

-- name: InsertSecret :one
INSERT INTO secret (company_id, id, cipher, updated_at)
VALUES (?, ?, ?, ?)
RETURNING *;

-- name: UpdateSecret :one
UPDATE secret SET cipher = ?, updated_at = ? WHERE company_id = ? AND id = ? RETURNING *;

-- name: ListSecrets :many
SELECT * FROM secret WHERE company_id = ? ORDER BY id;

-- name: DeleteSecret :exec
DELETE FROM secret WHERE company_id = ? AND id = ?;

-- name: GetProjectSecret :one
SELECT * FROM project_secret WHERE project_id = ? AND id = ?;

-- name: InsertProjectSecret :one
INSERT INTO project_secret (project_id, id, cipher, updated_at)
VALUES (?, ?, ?, ?)
RETURNING *;

-- name: UpdateProjectSecret :one
UPDATE project_secret SET cipher = ?, updated_at = ? WHERE project_id = ? AND id = ? RETURNING *;

-- name: ListProjectSecrets :many
SELECT * FROM project_secret WHERE project_id = ? ORDER BY id;

-- name: DeleteProjectSecret :exec
DELETE FROM project_secret WHERE project_id = ? AND id = ?;
