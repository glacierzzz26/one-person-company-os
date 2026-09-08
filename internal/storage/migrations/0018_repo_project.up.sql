-- Phase 10 D7 落地:研发仓库收敛为项目的 code 源绑定(一项目一代码仓)。
--   repos.project_id 可空列(只增不改):存量手工登记行 project_id=NULL = legacy(仍可被同步/收养);
--   新建项目 git remote 可解析为 GitHub 时自动派生并绑定;部分唯一索引兜底「一项目至多一条代码源」。
--   service 负责收养/派生(ensureCodeSource);DB 不写业务,仅持约束。
ALTER TABLE repos ADD COLUMN project_id TEXT REFERENCES projects(id);

CREATE UNIQUE INDEX idx_repos_project ON repos (project_id) WHERE project_id IS NOT NULL;
