-- Phase 10.2 巡检 + 调度(契约 ops-patrol-schedule.md):task 反链 pipeline_id(只增不改)。
--   用途:worker 认领 run 时能按流水线 kind 分流(ops_patrol → runPatrol);Web runs 显形态;
--   审计可追到产出它的流水线。FK ON(db.go)→ 删除流水线/项目都须先断 task 引用(service 编排)。
-- schedule 列 0013 已落库,本期才解析(cron 五段),此迁移不动 pipelines。

ALTER TABLE task ADD COLUMN pipeline_id TEXT REFERENCES pipelines(id);
