-- Phase 9 配置治理地基(方向 config-governance.md,9.1 契约 settings-foundation.md):
--   app_setting(global,单行 id='self') / company_setting(每公司覆盖行,NULL=继承 global)
--   / secret(company 机密,密文 enc:v2:,PRIMARY KEY(company_id,id))。
-- 只增不改:存量表零改动;endpoint token 仍存 endpoint.token_enc(enc:v1),re-key 走 UpdateEndpointToken。

CREATE TABLE app_setting (
    id                  TEXT PRIMARY KEY,                       -- 'self' 单行(global 设置)
    engine_mode_default TEXT NOT NULL DEFAULT 'live',           -- live | scripted
    agent_cli_default   TEXT NOT NULL DEFAULT 'claude',         -- claude | codex
    console_token_hash  TEXT NOT NULL DEFAULT '',               -- '' = 未初始化(/setup 开放)
    digest_time         TEXT NOT NULL DEFAULT '09:00',
    http_port           INTEGER NOT NULL DEFAULT 8787,
    poll_min            INTEGER NOT NULL DEFAULT 5,
    queue_work          INTEGER NOT NULL DEFAULT 0,             -- 0|1
    queue_interval_sec  INTEGER NOT NULL DEFAULT 10,
    updated_at          INTEGER NOT NULL
);

CREATE TABLE company_setting (
    company_id          TEXT PRIMARY KEY REFERENCES company(id),
    engine_mode         TEXT,                                   -- NULL=继承 global
    agent_cli           TEXT,
    issue_source        TEXT,
    issue_fixture_path  TEXT,
    updated_at          INTEGER NOT NULL
);

CREATE TABLE secret (
    company_id TEXT NOT NULL REFERENCES company(id),
    id         TEXT NOT NULL,            -- 'github_token' | 'feishu_webhook' | 'feishu_secret' …
    cipher     TEXT NOT NULL,            -- enc:v2:<b64>(settings.SealSecret,主密钥)
    updated_at INTEGER NOT NULL,
    PRIMARY KEY (company_id, id)
);
