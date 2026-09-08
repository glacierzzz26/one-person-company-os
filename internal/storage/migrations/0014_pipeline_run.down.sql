-- Phase 10.2 回滚:task.pipeline_id 列移除(只增不改,直接删列)。

ALTER TABLE task DROP COLUMN pipeline_id;
