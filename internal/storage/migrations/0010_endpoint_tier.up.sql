-- Phase 8.4: endpoint 档位分层(方向 model-runtime §三/§十)。
-- tier: frontier|standard|cheap(与 role pool|planner|standby 正交;UI 高智/均衡/经济)。
-- 存量 0007 端点回填 'standard'(不因未标档而失效);真实档位由 os endpoint select --tier 指派。

ALTER TABLE endpoint ADD COLUMN tier TEXT NOT NULL DEFAULT 'standard';
