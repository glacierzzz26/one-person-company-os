CREATE TABLE company (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    vision     TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE TABLE capability (
    id          TEXT PRIMARY KEY,
    company_id  TEXT NOT NULL REFERENCES company(id),
    code        TEXT NOT NULL,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    created_at  INTEGER NOT NULL,
    updated_at  INTEGER NOT NULL,
    UNIQUE (company_id, code)
);

CREATE TABLE agent (
    id           TEXT PRIMARY KEY,
    capability_id TEXT NOT NULL REFERENCES capability(id),
    name         TEXT NOT NULL,
    role         TEXT NOT NULL,
    model_hint   TEXT NOT NULL DEFAULT '',
    created_at   INTEGER NOT NULL,
    updated_at   INTEGER NOT NULL
);

CREATE TABLE policy (
    id         TEXT PRIMARY KEY,
    company_id TEXT NOT NULL REFERENCES company(id),
    name       TEXT NOT NULL,
    kind       TEXT NOT NULL,
    statement  TEXT NOT NULL,
    enabled    INTEGER NOT NULL DEFAULT 1,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE TABLE permission (
    id         TEXT PRIMARY KEY,
    policy_id  TEXT NOT NULL REFERENCES policy(id),
    subject    TEXT NOT NULL,
    action     TEXT NOT NULL,
    resource   TEXT NOT NULL,
    created_at INTEGER NOT NULL
);

CREATE TABLE workflow (
    id          TEXT PRIMARY KEY,
    company_id  TEXT NOT NULL REFERENCES company(id),
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    definition  TEXT NOT NULL DEFAULT '',
    created_at  INTEGER NOT NULL,
    updated_at  INTEGER NOT NULL
);

CREATE TABLE task (
    id             TEXT PRIMARY KEY,
    company_id     TEXT NOT NULL REFERENCES company(id),
    capability_id  TEXT REFERENCES capability(id),
    workflow_id    TEXT REFERENCES workflow(id),
    agent_id       TEXT REFERENCES agent(id),
    title          TEXT NOT NULL,
    description    TEXT NOT NULL DEFAULT '',
    tool_name      TEXT NOT NULL DEFAULT 'shell',
    status         TEXT NOT NULL,
    priority       INTEGER NOT NULL DEFAULT 0,
    attempt        INTEGER NOT NULL DEFAULT 0,
    risk           TEXT NOT NULL DEFAULT 'low',
    qstatus        TEXT NOT NULL DEFAULT 'ready',
    lease_worker_id TEXT NOT NULL DEFAULT '',
    lease_until    INTEGER NOT NULL DEFAULT 0,
    max_attempts   INTEGER NOT NULL DEFAULT 1,
    timeout_sec    INTEGER NOT NULL DEFAULT 0,
    last_error     TEXT NOT NULL DEFAULT '',
    result         TEXT NOT NULL DEFAULT '',
    workspace_path TEXT NOT NULL DEFAULT '',
    parent_task_id TEXT REFERENCES task(id),
    round_no       INTEGER NOT NULL DEFAULT 0,
    conflict_count INTEGER NOT NULL DEFAULT 0,
    writer_endpoint_id   TEXT REFERENCES endpoint(id),
    reviewer_endpoint_id TEXT REFERENCES endpoint(id),
    test_endpoint_id     TEXT REFERENCES endpoint(id),  -- test 判读槽(8.4;默认 standard,与 review 分槽)
    project_id           TEXT REFERENCES projects(id),  -- Phase 10.1:流水线 run 产物挂项目(可空;删项目先断引用)
    pipeline_id          TEXT REFERENCES pipelines(id), -- Phase 10.2:run 反链流水线(可空;认领判 kind / Web 显形态;删流水线先断引用)
    pull_request_url     TEXT,                          -- Phase 10.5:run 收尾 GitHub PR url(幂等账本;NULL=未发)
    pull_request_number  INTEGER,                       -- Phase 10.5:PR 号(Web 外链 / publish-pr 判据)
    created_at     INTEGER NOT NULL,
    updated_at     INTEGER NOT NULL
);

CREATE TABLE execution (
    id          TEXT PRIMARY KEY,
    task_id     TEXT NOT NULL REFERENCES task(id),
    worker_id   TEXT NOT NULL DEFAULT '',
    attempt     INTEGER NOT NULL DEFAULT 0,
    status      TEXT NOT NULL,
    started_at  INTEGER NOT NULL,
    finished_at INTEGER,
    result      TEXT NOT NULL DEFAULT '',
    error       TEXT NOT NULL DEFAULT '',
    created_at  INTEGER NOT NULL
);

CREATE INDEX idx_execution_task ON execution (task_id);

CREATE TABLE approval (
    id            TEXT PRIMARY KEY,
    task_id       TEXT NOT NULL REFERENCES task(id),
    risk          TEXT NOT NULL,
    reason        TEXT NOT NULL DEFAULT '',
    status        TEXT NOT NULL DEFAULT 'pending',
    requested_by  TEXT NOT NULL DEFAULT '',
    decided_by    TEXT NOT NULL DEFAULT '',
    decision_note TEXT NOT NULL DEFAULT '',
    created_at    INTEGER NOT NULL,
    decided_at    INTEGER
);

CREATE INDEX idx_approval_task ON approval (task_id);

CREATE TABLE audit (
    id          TEXT PRIMARY KEY,
    entity_type TEXT NOT NULL,
    entity_id   TEXT NOT NULL,
    action      TEXT NOT NULL,
    actor       TEXT NOT NULL DEFAULT '',
    detail      TEXT NOT NULL DEFAULT '',
    created_at  INTEGER NOT NULL
);

CREATE INDEX idx_audit_entity ON audit (entity_type, entity_id);

CREATE TABLE memory (
    id          TEXT PRIMARY KEY,
    company_id  TEXT NOT NULL REFERENCES company(id),
    type        TEXT NOT NULL,
    title       TEXT NOT NULL,
    content     TEXT NOT NULL,
    source      TEXT NOT NULL DEFAULT '',
    tags        TEXT NOT NULL DEFAULT '',
    created_at  INTEGER NOT NULL,
    updated_at  INTEGER NOT NULL
);

CREATE INDEX idx_memory_company ON memory (company_id);

CREATE TABLE decision (
    id          TEXT PRIMARY KEY,
    company_id  TEXT NOT NULL REFERENCES company(id),
    title       TEXT NOT NULL,
    kind        TEXT NOT NULL,
    status      TEXT NOT NULL,
    body        TEXT NOT NULL DEFAULT '',
    decided_by  TEXT NOT NULL DEFAULT '',
    source      TEXT NOT NULL DEFAULT '',
    created_at  INTEGER NOT NULL,
    updated_at  INTEGER NOT NULL
);

CREATE INDEX idx_decision_company ON decision (company_id);

CREATE TABLE endpoint (
    id             TEXT PRIMARY KEY,
    company_id     TEXT NOT NULL REFERENCES company(id),
    name           TEXT NOT NULL,
    base_url       TEXT NOT NULL,
    token_enc      TEXT NOT NULL DEFAULT '',
    proto          TEXT NOT NULL DEFAULT 'auto',
    vendor         TEXT NOT NULL DEFAULT '',
    selected_model TEXT NOT NULL DEFAULT '',
    role           TEXT NOT NULL DEFAULT 'pool',
    tier           TEXT NOT NULL DEFAULT 'standard',  -- frontier | standard | cheap(8.4 档位分层;与 role 正交)
    status         TEXT NOT NULL DEFAULT 'active',
    models_cache   TEXT NOT NULL DEFAULT '',
    created_at     INTEGER NOT NULL,
    updated_at     INTEGER NOT NULL,
    UNIQUE (company_id, name)
);

CREATE INDEX idx_endpoint_company ON endpoint (company_id);

CREATE TABLE repos (
    id             TEXT PRIMARY KEY,
    company_id     TEXT NOT NULL REFERENCES company(id),
    name           TEXT NOT NULL,
    repo_url       TEXT NOT NULL,
    workspace_path TEXT NOT NULL DEFAULT '',
    project_id     TEXT REFERENCES projects(id),  -- Phase 10 D7:项目 code 源绑定(可空=legacy 手工登记行)
    created_at     INTEGER NOT NULL,
    UNIQUE (company_id, name)
);

CREATE INDEX idx_repos_company ON repos (company_id);

CREATE UNIQUE INDEX idx_repos_project ON repos (project_id) WHERE project_id IS NOT NULL;

CREATE TABLE issue_sync (
    id           TEXT PRIMARY KEY,
    company_id   TEXT NOT NULL REFERENCES company(id),
    repo_id      TEXT NOT NULL REFERENCES repos(id),
    issue_number INTEGER NOT NULL,
    title        TEXT NOT NULL DEFAULT '',
    disposition  TEXT NOT NULL DEFAULT '',
    task_id      TEXT,
    note         TEXT NOT NULL DEFAULT '',
    created_at   INTEGER NOT NULL,
    UNIQUE (repo_id, issue_number)
);

CREATE INDEX idx_issue_sync_repo ON issue_sync (repo_id);

CREATE INDEX idx_issue_sync_task ON issue_sync (task_id);  -- 10.5:按 task 反查来源 issue(PR 圈定)

-- Phase 10.5 项目级机密(契约 github-roundtrip-pr.md;与迁移 0019 终态一致)。
-- project_secret:每项目 GitHub token(写路径唯一凭据),同公司 secret 同款 enc:v2: 加密封存。
CREATE TABLE project_secret (
    project_id TEXT NOT NULL REFERENCES projects(id),
    id         TEXT NOT NULL,            -- 'github_token'(service 白名单校验)
    cipher     TEXT NOT NULL,            -- enc:v2:<b64>(settings.SealSecret,主密钥)
    updated_at INTEGER NOT NULL,
    PRIMARY KEY (project_id, id)
);

-- Phase 9 配置治理地基(app_setting/company_setting/secret) —— 与迁移 0012 终态一致。

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
    schedule_poll_sec   INTEGER NOT NULL DEFAULT 0,             -- 10.2 独立调度开关(秒;0=关,与 queue_work 正交)
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

-- Phase 10.1 项目实体 + 声明式流水线(契约 project-pipeline-foundation.md) —— 与迁移 0013 终态一致。

CREATE TABLE projects (
    id          TEXT PRIMARY KEY,
    company_id  TEXT NOT NULL REFERENCES company(id),
    name        TEXT NOT NULL,
    root_path   TEXT NOT NULL,                 -- 绝对路径;整项目 git 仓库根(layout A)
    description TEXT NOT NULL DEFAULT '',
    created_at  INTEGER NOT NULL,
    updated_at  INTEGER NOT NULL,
    UNIQUE (company_id, name)
);

CREATE INDEX idx_projects_company ON projects (company_id);

CREATE TABLE pipelines (
    id          TEXT PRIMARY KEY,
    project_id  TEXT NOT NULL REFERENCES projects(id),
    name        TEXT NOT NULL,
    kind        TEXT NOT NULL DEFAULT 'bugfix',   -- bugfix | develop | ops_patrol(D6;service 白名单校验)
    description TEXT NOT NULL DEFAULT '',         -- 意图(自然语言;run 无 request 时即 writer 请求)
    risk        TEXT NOT NULL DEFAULT 'medium',   -- 护栏:low|medium|high
    status      TEXT NOT NULL DEFAULT 'active',   -- active | disabled(建即 active;run 须 active)
    schedule    TEXT NOT NULL DEFAULT '',         -- 10.2 才解析;本期恒空
    plan_policy TEXT NOT NULL DEFAULT 'adaptive', -- 10.4:adaptive=grow 记账 / synthesize=frontier 合成(service 白名单校验)
    created_at  INTEGER NOT NULL,
    updated_at  INTEGER NOT NULL,
    UNIQUE (project_id, name)
);

CREATE INDEX idx_pipelines_project ON pipelines (project_id);

-- Phase 10.3 run 计划账本(契约 phase-plan-contract.md §3.1;迁移 0016 同步维护)。
-- 每条流水线 run 一份计划;materialized = upfront(ops_patrol 建单预铺全) | grow(engineering 执行中 append)。
CREATE TABLE task_plan (
    id           TEXT PRIMARY KEY,
    task_id      TEXT NOT NULL UNIQUE REFERENCES task(id),
    kind         TEXT NOT NULL,
    materialized TEXT NOT NULL DEFAULT 'grow',
    created_at   INTEGER NOT NULL,
    updated_at   INTEGER NOT NULL
);

CREATE INDEX idx_task_plan_task ON task_plan (task_id);

CREATE TABLE task_plan_phase (
    id          TEXT PRIMARY KEY,
    plan_id     TEXT NOT NULL REFERENCES task_plan(id),
    seq         INTEGER NOT NULL,
    kind        TEXT NOT NULL,
    title       TEXT NOT NULL,
    allocator   TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'pending',
    evidence    TEXT NOT NULL DEFAULT '',
    note        TEXT NOT NULL DEFAULT '',
    started_at  INTEGER,
    finished_at INTEGER,
    UNIQUE (plan_id, seq)
);

CREATE INDEX idx_task_plan_phase_plan ON task_plan_phase (plan_id);
