-- Phase 6.1: endpoint 表 — 模型池端点落库(DevLoop 模型接入)
-- 事实源 = 此表;config providers 仅作引导/默认。token_enc 为 aesgcm 密文(密钥自 env,不落明文)。

CREATE TABLE endpoint (
    id             TEXT PRIMARY KEY,
    company_id     TEXT NOT NULL REFERENCES company(id),
    name           TEXT NOT NULL,
    base_url       TEXT NOT NULL,
    token_enc      TEXT NOT NULL DEFAULT '',
    proto          TEXT NOT NULL DEFAULT 'auto',   -- auto | anthropic | openai
    vendor         TEXT NOT NULL DEFAULT '',        -- 厂商标识(域名自动识别,可手改)
    selected_model TEXT NOT NULL DEFAULT '',
    role           TEXT NOT NULL DEFAULT 'pool',    -- pool | planner | standby
    status         TEXT NOT NULL DEFAULT 'active',  -- active | disabled
    models_cache   TEXT NOT NULL DEFAULT '',        -- json:最近一次 /v1/models 拉取结果
    created_at     INTEGER NOT NULL,
    updated_at     INTEGER NOT NULL,
    UNIQUE (company_id, name)
);

CREATE INDEX idx_endpoint_company ON endpoint (company_id);
