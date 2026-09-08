-- Phase 10 D7 回滚:去掉 repos→project 的 code 源绑定列与部分唯一索引(只增不改,直接删)。
DROP INDEX idx_repos_project;

ALTER TABLE repos DROP COLUMN project_id;
