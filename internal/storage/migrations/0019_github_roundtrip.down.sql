-- Phase 10.5 回滚(只增不改,直接删)。
DROP INDEX idx_issue_sync_task;

ALTER TABLE task DROP COLUMN pull_request_number;
ALTER TABLE task DROP COLUMN pull_request_url;

DROP TABLE project_secret;
