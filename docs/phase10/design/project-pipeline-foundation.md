# Phase 10.1 — 项目实体 + 声明式流水线落库 + Web 作者面(实施契约)

> 实施契约(方向:declarative-pipelines.md 定稿 §八 10.1;定稿门三项方向决策 + 本契约定稿门两项)。
> 目标:**一个「项目」= company 下一个目录容器(整目录一个 git 仓库),绑定的流水线=声明式意图,
> 在 Web 控制台编写落库即生效;run 复用 engineering driver 在项目目录内真执行;Web 全程跟上。**

## 一、范围与边界

### 做(10.1 交付)
1. 新一等实体 **project**(company 下;name/root_path/description)—— root_path = 整项目 git 仓库
   根(layout A)。建项目 root 就绪策略(**定稿门①**):已存在且 git → 直接用;不存在或空目录 →
   OS `mkdir -p` + `git init`;存在且非空但非 git → 400 报错(带指引)。**不** clone、不搬、不写入内容。
2. 新一等实体 **pipeline**(绑 project;name/kind/description(意图,自然语言)/risk(护栏)/status/
   schedule(10.2 预留,本期空串))—— kind ∈ {bugfix, develop, ops_patrol}(D6 三形态)。
3. `task` 加可空列 `project_id`(只增不改):run 产物挂项目,支撑项目详情「最近 runs」/审计按项目/
   **同目录串行守卫**(同 project 至多一条活跃 engineering task)。
4. **run 语义(方向决策②锁定)= 复用 engineering driver**:`RunPipeline` 在项目 root 目录建一条
   engineering task(Title=pipeline.name, Description=意图(request 覆盖则用 request), Risk=pipeline.risk,
   Workspace=project.root_path, ProjectID 挂上);档位默认解析/硬失败走 `createTask`(8.4,internal/service/task.go)。
   认领执行 = 既有 driver 回合闭环(planner→writer 委派 claude→test→review→熔断转审批),**不另起执行体**。
   run = 异步建单(qstatus ready),由 worker(server QueueLoop 自驱 或 `os queue work`)消费。
5. **受控 DELETE(定稿门②)**:pipeline 直接可删;project 删除须**无活跃 run**,历史任务先断
   project 关联(NOT NULL 归零,任务本体与审计保留),级联删其流水线,**绝不碰 root_path 磁盘目录**。
6. API + CLI + **Web 作者面**(项目列表/详情、建项目、流水线列表/新建/删除、运行、最近 runs 可见)。

### 不做(边界)
- 不做运行时代码/计划合成(10.3/10.4);不做 schedule 调度与 ops_patrol 真形态(10.2)。
- 不做 project↔repo 代码源绑定与通道 B issue→流水线链式触发(后续子阶段;方向 D7 只增不改已兼容)。
- 不改 0-9 冻结契约/表/语义;迁移只增不改;新实体产品零 env(9.4 反向门控沿用)。
- 不做 code//knowledge/ 子目录实体化(10.1 run 委派面=整项目根);不物理删磁盘。

## 二、代码现状核实(立足 2026-09-08,phase 9.4 完结态;行号以当前工作树为准)

- **迁移系统**:`internal/storage/migrations/` embed FS,自动按名扫描应用 `NNN_<name>.up/down.sql`;
  最新 0012_config_governance(见 `internal/storage/migrate.go:16 embed`)。schema 权威副本维护在
  `db/schema.sql`(sqlc 用)。**FK 强制**:`internal/storage/db.go:19 PRAGMA foreign_keys=ON` → 删除必须先断子行引用。
- **数据层管线**:`db/schema.sql` + `db/queries/*.sql`(sqlc)→ 生成 `internal/storage/query/*.sql.go`(入库,
  sqlc.yaml out=internal/storage/query)→ `internal/storage/repository/<entity>.go`(Store 方法,query 行 ↔ 领域
  struct,见 workflow.go:10-48 范式)→ 领域 struct `internal/<entity>/model.go`(workflow.Workflow 范式)。
  store 结构 `internal/storage/repository/store.go:10 Store{db, q}`。
- **task 模型/建单**:`internal/task/model.go`(Task 字段,含 8.4 三端点槽);建单入口
  `internal/service/task.go:69 createTask` —— engineering(live)空槽默认落档 `applyEndpointDefaults`
  (`internal/service/tier.go:33`;reviewer=frontier+openai/test=standard+openai 解析不到→**硬错误**带指引文案
  tier.go:66-68;writer=cheap+anthropic 留空合法;scripted 公司跳过)。TaskParams 含三端点/Workspace 等。
- **driver/委派**:`internal/service/execution.go:74` engineering 任务分流 `runEngineering`
  (`internal/service/driver.go:34`;回合 writer→test→review,冲突熔断转审批)。live writer = 委派 claude Code,
  cwd 钉 workspace(`internal/service/delegate.go:105-109`),起点须 git+clean(8.3 baseline ref
  `refs/os/tasks/<id>`,逐委派自动 commit,`.claude/.codex`→`.git/info/exclude`,git 走 exec.Command
  delegate.go:162);委派简报消费 Task.Title/Description(delegate.go:414-416)→ run 的意图文本即 writer 请求。
  scripted 全角色确定性桩(`internal/service/engine.go:52-53, 122+`)——无网关可离线收敛 driver 全流程。
- **worker**:`internal/server/server.go:216 QueueLoop`(queue_work 开启时 server 自驱,worker="server",
  单 tick ≤16 条;关则 `os queue work`)。run 建单后由它消费,Web 轮询任务状态即可。
- **HTTP**:`internal/server/api.go` chi 路由注册(companies/{id}/… /tasks /approvals /endpoints /repos
  /settings…)+ `apiOK/handleServiceErr`(信封 `{ok,data}`/`{ok:false,error{code,message}}`;
  错误映射仅 sql.ErrNoRows→404,余→500 —— 本契约新增 400/409 需扩 handleServiceErr 识别 sentinel)。
  鉴权:console token DB 哈希 constant-time(Bearer),写 actor 惯例 human:console / human:cli。
- **CLI**:cobra 组 `root.go:66+` AddCommand;组范式 internal/cli/company.go(create/list/show)。
- **Web(React18+TS+AntD5,`web/`)**:路由 `src/App.tsx`(BootGate→Routes);菜单 `src/components/AppLayout.tsx`
  NAV(分组 + active 校验数组:76-78 行 valid 列表);数据 `src/hooks/useApi.ts` useData;请求
  `src/api/client.ts` request<T>(信封+Bearer+401 AuthModal)+ `src/api/endpoints.ts` 逐端点薄 helper +
  `src/api/types.ts` snake_case;CRUD 范式 `src/pages/Repos.tsx` + `src/components/modals.tsx` RepoCreateModal;
  公司作用域取 `useApp().companyId`。`make ui`(web→`internal/console/ui` go:embed)+ `tsc --noEmit`。

## 三、契约设计

### 3.1 实体与迁移(0013_project_pipeline)

`internal/storage/migrations/0013_project_pipeline.up.sql`(只增不改;含 down):

```sql
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
    kind        TEXT NOT NULL DEFAULT 'bugfix',   -- bugfix | develop | ops_patrol(D6;service 白名单校验,不 CHECK 以免第 4 形态要改迁移)
    description TEXT NOT NULL DEFAULT '',         -- 意图(自然语言;run 无 request 时即 writer 请求)
    risk        TEXT NOT NULL DEFAULT 'medium',   -- 护栏:low|medium|high
    status      TEXT NOT NULL DEFAULT 'active',   -- active | disabled(本期建即 active;run 须 active)
    schedule    TEXT NOT NULL DEFAULT '',         -- 10.2 才解析;本期恒空
    created_at  INTEGER NOT NULL,
    updated_at  INTEGER NOT NULL,
    UNIQUE (project_id, name)
);
CREATE INDEX idx_pipelines_project ON pipelines (project_id);

ALTER TABLE task ADD COLUMN project_id TEXT REFERENCES projects(id);
```

`db/schema.sql` 同步追加(表体 + 注释行);repos 表不动。

领域 struct(仿 workflow/model.go):
- `internal/project/model.go`: Project{ID, CompanyID, Name, RootPath, Description, CreatedAt, UpdatedAt}
- `internal/pipeline/model.go`: Pipeline{ID, ProjectID, Name, Kind, Description, Risk, Status, Schedule,
  CreatedAt, UpdatedAt} + `KindBugfix/KindDevelop/KindOpsPatrol` + `ValidKinds` + 合法 risk 常量(沿 task risk)。
- `internal/task/model.go` 加 `ProjectID *string`(只增字段;既有构造点只 service.createTask 一处要填)。

### 3.2 repository(仿 workflow.go/repo.go)
- project.go: CreateProject / GetProject / ListProjectsByCompany / DeleteProject
- pipeline.go: CreatePipeline / GetPipeline / ListPipelinesByProject / DeletePipeline / DeletePipelinesByProject
- task 补: ListTasksByProject(ctx, projectID, limit) / ClearTaskProject(ctx, projectID)(
  `UPDATE tasks SET project_id=NULL WHERE project_id=?`;删项目前断引用,任务/审计保留)
- 活跃判定由 service 组合现有 store.ListTasks(company, status…) 或新增 store 查询;契约定用
  `CountActiveTasksByProject(projectID)`(`WHERE project_id=? AND status IN ('pending','running','waiting_approval')`),
  一次性原子于 service 检查(单 worker 环境足够;同目录串行本来靠 OS 单 worker + git 捕获)。

### 3.3 service(internal/service/project.go + pipeline.go + task.go 增一行)
- `CreateProjectAs(actor, companyID, name, rootPath, description)`:
  ① rootPath 校验 + 就绪(root 策略定稿门①,见 §一;用 exec.Command git,沿 delegate.go:162 用法);
  ② store.CreateProject;③ audit("project", id, "create", actor, rootPath detail)。重复名 → ErrConflict。
- `GetProject/ListProjectsByCompany/DeleteProjectAs(actor, id)`:
  Delete 前查 `CountActiveTasksByProject` > 0 → **ErrProjectBusy**(409);有历史任务 → ClearTaskProject →
  DeletePipelinesByProject → DeleteProject;audit("project", id, "delete", actor, …)。不碰磁盘。
- `CreatePipelineAs(actor, projectID, name, kind, desc, risk)`:kind ∈ ValidKinds(否则 ErrInvalid)、
  risk 归一到 low/medium/high(缺省 medium);store.CreatePipeline;audit。
- `GetPipeline/ListPipelinesByProject/DeletePipelineAs(actor, id)`:直接删(其 run 历史任务保留,project 不删);
  audit。
- `RunPipelineAs(actor, pipelineID, request string) (task.Task, error)`:
  ① GetPipeline + GetProject;status 非 active → ErrInvalid;② **串行守卫**:CountActiveTasksByProject
  (projectID)>0 → **ErrPipelineBusy**(409,「同目录仅一条活跃 run」);③ intent 文本 = trim(request)非空用
  request,否则 pipeline.description;④ `CreateTaskAs(TaskParams{CompanyID: prj.CompanyID, ToolName:"engineering",
  Title: pipeline.name, Description: intent, Risk: pipeline.risk, Workspace: prj.RootPath, ProjectID: &prj.ID,
  MaxAttempts:1}, actor)` —— 8.4 落槽/硬失败在 createTask 内自然发生(无端点/live 报指引错误面上抛);
  scripted 公司跳过落槽、driver 确定性收敛;⑤ audit("pipeline", id, "run", actor, task short id)。
- service 层 sentinel: `ErrProjectBusy/ErrPipelineBusy`(→409)、`ErrInvalid`(→400)、`ErrConflict`(→409)。

### 3.4 HTTP `/api/v1`(api.go 追加注册;信封/鉴权沿用)
- `GET  /companies/{id}/projects`   → apiListProjects
- `POST /companies/{id}/projects`   → apiCreateProject {name, root_path, description}
- `GET  /projects/{id}`             → apiGetProject
- `DELETE /projects/{id}`           → apiDeleteProject
- `GET  /projects/{id}/pipelines`   → apiListPipelines
- `POST /projects/{id}/pipelines`   → apiCreatePipeline {name, kind, description?, risk?}
- `GET  /projects/{id}/tasks?limit=`→ apiListProjectTasks(最近 runs;复用 ListTasksByProject)
- `GET  /pipelines/{id}`            → apiGetPipeline
- `DELETE /pipelines/{id}`          → apiDeletePipeline
- `POST /pipelines/{id}/run`        → apiRunPipeline {request?} → {task_id, project_id, pipeline_id}
写 actor = human:console;`handleServiceErr` 扩展识别 sentinel→400/409。

### 3.5 CLI(internal/cli/project.go + pipeline.go,root.go 挂组)
- `os project list|show|create(--company --name --root-path [--description])|delete <id>`(create 同 service 就绪逻辑;
  actor human:cli)
- `os pipeline list --project|show <id>|run <id> [--request]|delete <id>`

### 3.6 Web(作者面 = 验收核心)
- types.ts + endpoints.ts:Project/Pipeline/CreateProjectReq/CreatePipelineReq/RunReq + helpers(路径逐字对 §3.4)。
- App.tsx:路由 `/projects`(Projects 列表)与 `/projects/:id`(ProjectDetail)。
- AppLayout.tsx:NAV「执行」组下加 `{key:'/projects', label:'项目 · 流水线', icon:<FolderOutlined/>}` +
  active valid 数组补 `/projects`。
- pages/Projects.tsx(沿 Repos 范式):项目表(name/root_path/描述/流水线数/最近活动/创建时间)+
  「新建项目」Modal(name + root_path 绝对路径 + 描述 + Alert:须 git 或空目录将 git init;非空非 git 会 400);
  行点开 → /projects/:id;行「删除」二次确认(文案:仅无活跃 run 可删,不碰磁盘目录)。
- pages/ProjectDetail.tsx:`useData` 拉 project + pipelines + 最近 tasks(三 fetcher / 单接口聚合);
  流水线表(kind 徽标 bugfix/develop/ops_patrol + 名称 + 描述截断 + risk + status + 运行按钮 + 删除按钮);
  「新建流水线」Modal(kind 下拉 + 名称 + 意图描述 textarea + 风险);
  「运行」→ 弹 request Modal(本次运行请求,缺省用流水线描述)→ POST run → toast(task id)→ 跳 /tasks 或
  页内轮询该项目最近 tasks 状态;删除确认。
- utils/dicts.ts:kind/risk/status 文案。

### 3.7 边界裁定说明(供实施/review 对照)
- run 异步不阻塞 HTTP;任务推进靠 worker(server QueueLoop 或 os queue work),Web 轮询任务状态;
  live 无判读端点时建单即 400/报错(tier.go 文案),scripted 公司可离线跑通(验证用)。
- FK ON ⇒ project 删除必先 ClearTaskProject(断引用保历史),再删 pipelines,再删 project;顺序不可乱。
- 审计 actor 统一 human:console(Web)/human:cli(CLI);产品零 env,git 依赖沿用既有(8.3)。

## 四、文件落地清单
(新)migrations/0013_project_pipeline.up.sql/.down.sql;(改)db/schema.sql + db/queries/project.sql、
pipeline.sql、task.sql(补 project 查询)+ `sqlc generate`;(新)internal/project/model.go、
internal/pipeline/model.go;(改)internal/task/model.go;(新)internal/storage/repository/project.go、
pipeline.go;(改)internal/storage/repository/task.go、query/*(gen);(新)internal/service/project.go、
pipeline.go + service sentinel;(改)internal/service/task.go(填 ProjectID)、cli/root.go;(新)cli/project.go、
cli/pipeline.go;(改)server/api.go;(改)web/src/api/types.ts、endpoints.ts、App.tsx、components/AppLayout.tsx;
(新)web/src/pages/Projects.tsx、ProjectDetail.tsx;(改)web/src/components/modals.tsx、utils/dicts.ts;
(新)test 文件(见 §五)。

## 五、用例清单(定稿后逐条落地;命名 P*)
- P1 project create 成功(git 现成目录);P2 空目录 → mkdir+git init 后成功;P3 不存在路径 → mkdir+git init 成功;
  P4 非空非 git → ErrInvalid(400)且零写入;P5 重名 → ErrConflict(409);
- P6 pipeline create(三种 kind + 默认 risk medium + 非法 kind 400);P7 流水线列表按 project;
- P8 RunPipeline 建 engineering task(挂 project_id、workspace=root、description=request 覆盖逻辑);
  P9 同 project 已有活跃 task → ErrPipelineBusy(409);P10 完成态历史 task 不阻塞新 run;
- P11 project delete:活跃 run → 409;仅有历史任务 → 任务 project_id 置空 + pipelines 级联删 + project 删 + 磁盘目录仍存;
- P12 pipeline delete(其历史 run 任务保留);P13 CLI run/delete 与 Web actor 审计可见;
- P14 server/api:各端点信封 + 401(无令牌)/404;P15 无端点 live run → 错误文案含 tier 指引;
- 回归:既有 600+ 用例零改零删,仅 query/model 生成文件随 gen 变化;scripted driver 族绿。

## 六、风险与取舍
- root 目录在 OS 之外(委派面) → 只 git init/读状态,绝不 mkdir 业务子目录、绝不动已存在内容;删除绝不碰磁盘。
- 串行守卫 = service 查活跃任务,单 worker 稳态够用;极端并发由 8.3 git baseline 起点语义兜底。
- DELETE project 断任务 project 引用保历史(取舍:任务不再能按项目聚合——换「可删项目而审计不破」)。
- 无端点 live run 建单失败是 8.4 既定,不是 10.1 回归;scripted 提供离线全流程验证。
- schedule/ops_patrol/repo 绑定不做在 10.1(边界),防范围膨胀。

## 七、登记
- 契约登记 phase10/design/README.md;进度总表挂 10.1 实施契约 ✅(🔶 实施中)。
- 定稿门:方向三决策 + 本契约定稿门两项(root 就绪=OS git init 空目录;DELETE=受控)。归档阶段收尾写 stages/1.md。
