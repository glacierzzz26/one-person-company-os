-- Phase 6.2: task 回合扩展(Engineering Driver 回合状态 + 端点 + 拆解父链)
-- round_no / conflict_count 落 task 行(回合在驱动内部,不另建状态机)。
-- writer/reviewer_endpoint_id:本条任务写/审模型端点;空 = 走 agent 默认。
-- parent_task_id:planner 拆解子任务挂父请求(6.4 Intake 使用)。

ALTER TABLE task ADD COLUMN parent_task_id TEXT;
ALTER TABLE task ADD COLUMN round_no INTEGER NOT NULL DEFAULT 0;
ALTER TABLE task ADD COLUMN conflict_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE task ADD COLUMN writer_endpoint_id TEXT;
ALTER TABLE task ADD COLUMN reviewer_endpoint_id TEXT;
