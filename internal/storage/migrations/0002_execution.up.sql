-- Phase 1.1: Execution Core — execution 表 + task 队列字段

ALTER TABLE task ADD COLUMN qstatus         TEXT NOT NULL DEFAULT 'ready';
ALTER TABLE task ADD COLUMN lease_worker_id TEXT NOT NULL DEFAULT '';
ALTER TABLE task ADD COLUMN lease_until     INTEGER NOT NULL DEFAULT 0;
ALTER TABLE task ADD COLUMN max_attempts    INTEGER NOT NULL DEFAULT 1;
ALTER TABLE task ADD COLUMN timeout_sec     INTEGER NOT NULL DEFAULT 0;
ALTER TABLE task ADD COLUMN last_error      TEXT NOT NULL DEFAULT '';
ALTER TABLE task ADD COLUMN result          TEXT NOT NULL DEFAULT '';
ALTER TABLE task ADD COLUMN workspace_path  TEXT NOT NULL DEFAULT '';

CREATE TABLE execution (
    id          TEXT PRIMARY KEY,
    task_id     TEXT NOT NULL REFERENCES task(id),
    worker_id   TEXT NOT NULL DEFAULT '',
    attempt     INTEGER NOT NULL DEFAULT 0,
    status      TEXT NOT NULL,
    started_at  INTEGER NOT NULL,
    finished_at INTEGER,
    result      TEXT NOT NULL DEFAULT '',
    error       TEXT NOT NULL DEFAULT '',
    created_at  INTEGER NOT NULL
);

CREATE INDEX idx_execution_task ON execution (task_id);
