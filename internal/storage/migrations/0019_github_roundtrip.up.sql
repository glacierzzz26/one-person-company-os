-- Phase 10.5 项目级 GitHub token + issue→分支→PR 闭环(契约 github-roundtrip-pr.md §四 A)。
--   project_secret:每项目机密(同公司 secret 的 enc:v2: 加密封存;PK (project_id,id));
--     id 白名单暂仅 'github_token' —— 写路径(git push + 开 PR)只认项目 token(公司 token 只读回退)。
--   task.pull_request_url / task.pull_request_number:run 收尾发 PR 的幂等账本列(可空,只增不改);
--     pull_request_url 已置 = 已发过 PR,人工 publish-pr 重试直接返回既有,不重复建。
--   idx_issue_sync_task:按 task_id 反查来源 issue(issue_sync.task_id 回链 → PR 圈定)。
CREATE TABLE project_secret (
    project_id TEXT NOT NULL REFERENCES projects(id),
    id         TEXT NOT NULL,            -- 'github_token'(service 白名单校验,DB 不 CHECK)
    cipher     TEXT NOT NULL,            -- enc:v2:<b64>(settings.SealSecret,主密钥)
    updated_at INTEGER NOT NULL,
    PRIMARY KEY (project_id, id)
);

ALTER TABLE task ADD COLUMN pull_request_url TEXT;
ALTER TABLE task ADD COLUMN pull_request_number INTEGER;

CREATE INDEX idx_issue_sync_task ON issue_sync (task_id);
