-- Phase 10.3 回滚:task_plan_phase(先,其 REFERENCES task_plan)→ task_plan(只增不改,直接删)。
DROP TABLE task_plan_phase;
DROP TABLE task_plan;
