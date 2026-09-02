-- Phase 6.3: repos 表 — 研发仓库登记(issue 归属 → workspace;git/file 边界沿 workspace 强制)
-- issue_sync 为通道 B(研发 Intake)同步账本(实现细化,不在 §5.2 设计表清单内):
-- UNIQUE(repo_id, issue_number) 原生去重 → webhook 优先 + 轮询兜底不会重复处理同一 issue。

CREATE TABLE repos (
    id             TEXT PRIMARY KEY,
    company_id     TEXT NOT NULL REFERENCES company(id),
    name           TEXT NOT NULL,
    repo_url       TEXT NOT NULL,
    workspace_path TEXT NOT NULL DEFAULT '',
    created_at     INTEGER NOT NULL,
    UNIQUE (company_id, name)
);

CREATE INDEX idx_repos_company ON repos (company_id);

CREATE TABLE issue_sync (
    id           TEXT PRIMARY KEY,
    company_id   TEXT NOT NULL REFERENCES company(id),
    repo_id      TEXT NOT NULL REFERENCES repos(id),
    issue_number INTEGER NOT NULL,
    title        TEXT NOT NULL DEFAULT '',
    disposition  TEXT NOT NULL DEFAULT '',  -- direct_work | ask | skip | merge
    task_id      TEXT,                        -- direct_work 建出的 engineering task(可空)
    note         TEXT NOT NULL DEFAULT '',    -- skip 理由 / ask 追问 / merge 批注
    created_at   INTEGER NOT NULL,
    UNIQUE (repo_id, issue_number)
);

CREATE INDEX idx_issue_sync_repo ON issue_sync (repo_id);
