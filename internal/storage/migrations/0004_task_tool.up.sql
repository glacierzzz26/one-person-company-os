-- Phase 2.1: task 增加 tool_name 字段 — 引擎按 Tool 名取用执行器(默认 shell)

ALTER TABLE task ADD COLUMN tool_name TEXT NOT NULL DEFAULT 'shell';
