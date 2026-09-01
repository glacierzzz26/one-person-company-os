DROP TABLE execution;

ALTER TABLE task DROP COLUMN workspace_path;
ALTER TABLE task DROP COLUMN result;
ALTER TABLE task DROP COLUMN last_error;
ALTER TABLE task DROP COLUMN timeout_sec;
ALTER TABLE task DROP COLUMN max_attempts;
ALTER TABLE task DROP COLUMN lease_until;
ALTER TABLE task DROP COLUMN lease_worker_id;
ALTER TABLE task DROP COLUMN qstatus;
