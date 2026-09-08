-- Phase 10.4 回滚:pipelines.plan_policy 列移除(只增不改,直接删列)。
ALTER TABLE pipelines DROP COLUMN plan_policy;
