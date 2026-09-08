-- Phase 10.4 合成运行时触发(契约 run-synthesis.md §3.1):pipelines.plan_policy。
--   adaptive(缺省)= 现状 grow 记账(engineering run 认领走 runEngineering);
--   synthesize = frontier 对意图合成计划(首次认领)→ 落 upfront 账本 → 先审后干 → runSynthesized 逐相位驱动。
--   service 白名单校验(非法 → ErrInvalid),DB 不 CHECK;只增不改。
ALTER TABLE pipelines ADD COLUMN plan_policy TEXT NOT NULL DEFAULT 'adaptive';
