-- Phase 10.1 项目实体 + 声明式流水线(契约 project-pipeline-foundation.md):
--   projects(company 下目录容器;root_path = 整项目 git 仓库根,layout A)
--   / pipelines(绑 project;kind ∈ bugfix|develop|ops_patrol,service 白名单校验不 CHECK;
--     schedule 10.2 才解析,本期空)/ task.project_id 可空列(只增不改;run 产物挂项目,
--     供项目详情「最近 runs」/审计按项目/同目录串行守卫)。FK ON 已由 db.go 开启 → 删 project 先断 task 引用。
-- 只增不改:存量表零改动;repos 表不动(project↔repo 绑定留后续子阶段)。

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
    created_at  INTEGER NOT NULL,
    updated_at  INTEGER NOT NULL,
    UNIQUE (project_id, name)
);

CREATE INDEX idx_pipelines_project ON pipelines (project_id);

ALTER TABLE task ADD COLUMN project_id TEXT REFERENCES projects(id);
