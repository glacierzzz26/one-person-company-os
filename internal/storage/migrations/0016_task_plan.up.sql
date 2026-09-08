-- Phase 10.3 run 计划账本(契约 phase-plan-contract.md §3.1):task_plan / task_plan_phase。
--   每条流水线 run(task_id UNIQUE)一份计划;phase = 有序阶段(seq;kind do|accept|dispose)。
--   materialized = upfront(ops_patrol:建单即铺全,可先审后干)| grow(engineering:执行中 append)。
--   只写读,驱动内部翻状态,无外部写口;计划整体状态 = task.status 单一来源(plan 表不重复存)。
--   plan 最小外键仅挂 task(project/pipeline/company 均经 task 联到)→ 删项目/流水线零新增语义,
--   ClearTaskProject/ClearTaskPipeline 沿用,plan 随 task 保留(历史)。只增不改。
CREATE TABLE task_plan (
    id           TEXT PRIMARY KEY,
    task_id      TEXT NOT NULL UNIQUE REFERENCES task(id),  -- 一条流水线 run 一份计划
    kind         TEXT NOT NULL,      -- patrol | engineering(计划形态;与驱动分流同源,pipeline.kind)
    materialized TEXT NOT NULL DEFAULT 'grow',  -- upfront(建单即铺全,可先审) | grow(执行中 append)
    created_at   INTEGER NOT NULL,
    updated_at   INTEGER NOT NULL
);

CREATE INDEX idx_task_plan_task ON task_plan (task_id);

CREATE TABLE task_plan_phase (
    id          TEXT PRIMARY KEY,
    plan_id     TEXT NOT NULL REFERENCES task_plan(id),
    seq         INTEGER NOT NULL,    -- 阶段序;grow 计划 append 递增
    kind        TEXT NOT NULL,       -- do | accept | dispose
    title       TEXT NOT NULL,       -- 人类可读阶段目标(含 round/报告名等)
    allocator   TEXT NOT NULL,       -- delegate | os | judge | planner | manual
    status      TEXT NOT NULL DEFAULT 'pending',  -- pending | running | ok | fail | skipped
    evidence    TEXT NOT NULL DEFAULT '',  -- 产出物引用/摘要:报告 rel / verdict 行 / execution id /
                                           -- diff 首行+行数 / commit —— 不整存大产出(execution/报告已存正文)
    note        TEXT NOT NULL DEFAULT '',
    started_at  INTEGER,
    finished_at INTEGER,
    UNIQUE (plan_id, seq)
);

CREATE INDEX idx_task_plan_phase_plan ON task_plan_phase (plan_id);
