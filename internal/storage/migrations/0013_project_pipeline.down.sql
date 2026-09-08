-- Phase 10.1 回滚:task.project_id 列 + 两表移除(只增不改,直接删)。

ALTER TABLE task DROP COLUMN project_id;
DROP TABLE pipelines;
DROP TABLE projects;
