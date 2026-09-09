# Phase 10.5 — 项目级 GitHub token + issue→分支→PR 闭环(方向 + 实施契约)

> 实施契约 + 方向(2026-09-09;**本会话 ExitPlanMode 计划通过 = 契约定稿门**)。承
> [project-github-code.md](project-github-code.md)(建时 clone + run 前拉 = 代码**取回**半环已完结)与
> [declarative-pipelines.md](declarative-pipelines.md) 方向 D7:本契约补上**回写**半环 —— 每项目一个 GitHub
> token,OS 经它回拉该绑定仓库的 issue、下载代码、按 issue 建 bugfix/feature 分支、在分支上提交、发起 PR;
> **最终审批由人在 GitHub 的 PR/MR 页面点击合入(OS 永不代合、不做本地开 PR 前审批 UI)**。

> 用户需求原话(逐条保留):「我希望一个项目一个token，能通过这个token，回去issue，下载代码，创建bugfix 和 feature
> 分支，然后提交代码，发起pr，最后的审批必须人在页面上审核」「我只需要在github的mr页面点击合入」
> 「已有的项目需要支持编辑」「目前不考虑老项目」。

## 一、范围与边界

### 做
1. **项目级 GitHub token**:新表 `project_secret(project_id,id,cipher,updated_at,PK(project_id,id))`,
   同公司 `secret` 的 `enc:v2:` AES-GCM + master key 密封;id 白名单暂仅 `github_token`。Web 放在项目
   代码源卡上(设/换/删,永不回显明文)。
2. **token 解析双层**:读(同步 issue / 回拉代码)= 项目 token → 公司 `github_token` 回退;写
   (git push + 开 PR)= **仅项目 token**(公司只读 token 永不推)。
3. **按项目同步 issue**:ProjectDetail「同步 issue」由公司级改打**项目级** `SyncProject(projectID)`
   —— 只同步该项目绑定仓库的 open issues(通道 B 全链路:ListOpenIssues → IntakeIssues → triage →
   issue_sync 记账 + 工程任务),沿用 company `issue_source=fixture` 离线语义。
4. **issue→分支工程**:工程 run(engineering / synthesize)在绑定仓库上为**来源 GitHub issue 的任务**建
   `bugfix/<issue#>-<slug>` 或 `feature/<issue#>-<slug>` 分支并 checkout(标题规则定 prefix;slug=sanitize);
   每相位 OS 提交骑分支;`refs/os/tasks/<id>` 基线锚分支起点。scripted 不建分支。
5. **收尾发 PR**:engineering / synthesize / planner 拆分子任务聚合父任务**三处完成点**经统一 `finishRun`
   best-effort 发布:git push 分支(GitHub 认证经 `http.extraheader` Basic env 注入,不入 argv/不入 origin;
   本地/非 https origin 明文推)→ `POST /repos/{o}/{r}/pulls`(base=origin/HEAD,兜底 GitHub default_branch;
   body `Resolves #<issue>`)。publish 失败/无 token → 审计 `pr_fail`/`pr_skip`,任务照常 complete 不复活;
   补 `POST /tasks/{id}/publish-pr` 人工重试(改 token 后 / 瞬时 GitHub 抖动)。
6. **已有项目编辑**:名称/描述 + GitHub 绑定地址;root 空/不存在 → clone 认领;仅 git init 的零提交占位仓
   (clean、无 origin、无 HEAD)→ 删 `.git` 再 clone;**老项目(带本地 git 历史 / 指向异仓库)拒绝改挂**
   (用户拍板暂不考虑迁移);root_path 不可编辑。

### 不做(边界)
- **OS 不做开 PR 前审批、永不 merge**:合入一律由人在 GitHub PR/MR 页面完成(用户明确)。OS 只推分支 + 开 PR。
- **老项目(带本地 git 历史)迁入 GitHub 闭环**:不做历史并轨/改写(用户明确「目前不考虑老项目」);带历史目录改
  GitHub 地址 → `ErrInvalid` 并指引新建项目绑定。
- 不新增 `task.type`/issue 引用列:走 PR 的圈定靠既有 `issue_sync.task_id` 回链(唯一新增查询 `GetIssueSyncByTask`)。
- root_path 编辑、私有仓库建时首次 clone、ssh-origin token push、webhook 实时改表(沿用既有 webhook/轮询)。
- 并发多任务共享同一 workspace 的分支竞争(现状即单 worker 语义,不加锁)。
- 不改 `internal/tool/git.go` agent 网络 git 墙(宿主侧 push 不经它)。

## 二、现状缺口核实(立足 2026-09-09,Phase 10.4 + 自动代码获取完结态;行号以当前工作树为准)

- 机密仅公司级 `secret`(PK `(company_id,id)`,migrations/0012);运行时唯一消费 `issueSourceFor`
  (runtime.go:61)fail-closed 只读公司 `github_token`(今天「同步 issue 无 token」报错即此处)。
- `github.Client`(github.go:41)仅 `ListOpenIssues`(读)与 `PostComment`(写);**无 branch / push / PR**;api base
  硬编码 `https://api.github.com`,测试不可覆写。
- OS 提交骑当前 HEAD(`delegateBaseline`→`ensureBaseline` 钉 `refs/os/tasks/<id>` → 逐委派
  `commitDelegation`);`internal/service` **无 `checkout -b` / `push`**(grep 零命中);agent git 墙只拦 agent,宿主
  `gitDirCmd`(delegate.go:161)直执行可绕。
- **项目无 update 接口**(api.go:123-125 仅 GET/DELETE);`POST /projects/{id}/code-source/refresh`
  (projects_api.go:112,注释「建后补/换 remote → 重认领」)可复用作克隆后派生认领。
- 工程任务真正完成点三处:`driver.go:150`(逐 round approve)、`synthesize.go:463`(全相位过)、
  `plan_driver.go:141`(planner 拆分子任务后**聚合父任务**——父才是带 issue 那个,PR 须挂父收尾);
  `execution.go:154` 是非 engineering tool 路径(engine 已在前分发),patrol 独立,二者到不了 issue 闭环。
- 最近迁移 0018_repo_project;sqlc 输出 `internal/storage/query`;`db/schema.sql` 累积快照需手工同步。

## 三、决策与默认(定稿门,本会话计划通过 = 批准)

1. token 存独立 `project_secret` 表(改公司表 PK 不可行),同款加密封存;白名单 id = `github_token`。
2. 写路径(push/PR)只认项目 token;读路径项目 token → 公司回退(公司 token 只读)。
3. **分支两处幂等挂载**:①`delegateBaseline`(常规+合成保 default 分支干净、任务间隔离);②收尾
   `finishRun`(兜 planner 拆分子任务父路径——父在子前返回、从未经 delegateBaseline,完成时才从 HEAD 起分支)。
   `ensureIssueBranch`:分支不存在 → `checkout -b`(自 HEAD,允许携带现有提交/改动);存在 → checkout(已在则 no-op)。
4. **收尾即发 + 人工重试**:auto publish best-effort 不阻塞完成;publish-pr 端点人工触发;idempotent
   (`task.pull_request_url` 已置 → 直接返回既有,不重复建 PR)。
5. prefix 标题规则:`bug|fix|defect|crash|修复|缺陷|崩溃|错误` 命中 → `bugfix`,否则 `feature`。
6. git push 认证:**https-github origin 才注入**,`GIT_CONFIG_COUNT/KEY/VALUE` env 设 `http.https://github.com/.extraheader`
   = `Authorization: Basic base64("x-access-token:<token>")`(token 不入 argv、不改写 origin);其余 origin 明文推。
7. scripted 一律不建分支/不发 PR(离线确定性、不碰网络)。

## 四、实施改动

### A. 数据层(迁移 0019 + sqlc 再生成)
- `internal/storage/migrations/0019_github_roundtrip.up/.down.sql`:
  `CREATE TABLE project_secret(project_id TEXT NOT NULL REFERENCES projects(id), id TEXT NOT NULL,
  cipher TEXT NOT NULL, updated_at INTEGER NOT NULL, PRIMARY KEY(project_id,id))`;
  `ALTER TABLE task ADD COLUMN pull_request_url TEXT`;`ALTER TABLE task ADD COLUMN pull_request_number INTEGER`;
  `CREATE INDEX idx_issue_sync_task ON issue_sync(task_id)`。
- 同步 `db/schema.sql`;`db/queries/settings.sql`(project_secret CRUD)、`repo.sql`(`GetIssueSyncByTask:one`)、
  `task.sql`(`SetTaskPullRequest:execrows`)加 named queries → sqlc 再生成;repository 按既有 mapper+method 模式
  补方法;`internal/settings/model.go` 加 `ProjectSecret`。

### B. 机密服务 + 解析(runtime 改造)
- `internal/service/settings.go`:项目机密方法(镜像公司版 :201-219)——
  `SetProjectSecretAs(ctx,projectID,id,value,actor)` / `OpenProjectSecretCurrent(ctx,projectID,id)` /
  `ProjectSecretMeta(ctx,projectID)` / `DeleteProjectSecretAs` + `validateProjectSecretID`(github_token 白名单)。
- `internal/service/runtime.go`:`githubTokenFor(ctx, companyID, projectID)`(项目 → 公司回退);读 client 共用。

### C. 通道 B 按项目同步 + 已有项目编辑
- `internal/service/intake.go`:抽 `syncRepo(ctx, r)`(source = `sourceForRepo`:repo 挂项目 → 项目 token → 公司
  回退);`SyncRepos` 逐 repo 复用;新增 `SyncProject(ctx, projectID)` = `CodeSourceFor` 绑仓 + `syncRepo`。
- `internal/service/project.go` + store:新 `UpdateProjectAs(ctx, projectID, {name?, description?, repoURL?}, actor)`;
  repoURL 按决策三.4 安全子集分支(`gitCloneURL` + `RefreshProjectCodeSourceAs` 派生认领);store `UpdateProject`
  处理 UNIQUE(company_id,name) 冲突 → ErrInvalid。

### D. 分支 + 发 PR(service 新 `pr.go` + 三处接线)
- 新 `internal/service/pr.go`:`prTarget`(ProjectID + GitHub-parseable 绑仓 + `GetIssueSyncByTask` 命中)/
  `issueBranchName`+slug+prefix 规则 / `ensureIssueBranch` / `originDefaultBranch`(symbolic-ref,兜底 API)/
  `gitAuthEnv(origin, token)`(纯函数)/ `pushIssueBranch`(30s 超时、合并输出、audit)/
  `publishRunPR(ctx, t)`(scripted / 无 target / 无项目 token → 静默 skip;成功 `SetTaskPullRequest` + audit
  `pr_opened`;失败 audit `pr_fail`)。`finishRun(ctx,t,result)` = publish(best-effort)+ `CompleteTask`;
  替换 driver.go:150、synthesize.go:463、plan_driver.go:141 三完成点。
- `internal/service/delegate.go`:`delegateBaseline` 在 git/clean 判定处插 `ensureIssueBranch`(仅非 scripted +
  `prTarget` 命中)。
- `internal/github/github.go`:`Pull{Number,HTMLURL}`、`PullParams{Title,Head,Base,Body}`、
  `CreatePull(ctx,o,r,PullParams)`(POST /repos/{o}/{r}/pulls,201;错误风络同 PostComment)、
  `DefaultBranch(ctx,o,r)`(GET /repos/{o}/{r} default_branch);`PullPublisher` 接口 + `NewClientAt(token, apiBase)`
  供 httptest 覆写 base。

### E. server 路由(api.go 注册 + handlers,consoleActor)
- `PUT /projects/{id}`(name/desc/repo_url → projectView);
- `GET/PUT/DELETE /projects/{id}/secrets[/{secretID}]`(镜像 settings_api.go:169-200,白名单 github_token);
- `POST /projects/{id}/intake/sync`(项目绑仓同步);
- `POST /tasks/{id}/publish-pr`(完成态 + 无 url + prTarget + 项目 token 门)。

### F. Web
- `types.ts`:`Task.pull_request_url?/pull_request_number?`、`ProjectSecretMeta`、project update 请求类型;
  `endpoints.ts`:updateProject / projectSecrets(list,set,delete) / syncProjectIssues(projectId) / publishPullRequest(taskId)。
- `ProjectDetail.tsx`:代码源卡加项目 GitHub Token mini 卡(镜像 Settings CompanySecretsCard,Input.Password 不回显);
  「同步 issue」改打项目级;编辑按钮 + modal(name/desc/repo_url + 绑定约束提示);runs 表 PR 列(有 url → 外链 Tag;
  completed + 绑 GitHub + 无 url → 「发起 PR」)。

## 五、验证(离线为主,免网络)

- `go vet ./...` / `go build ./...` 干净;`go test -count=1 ./...` 全绿(既有用例零 body 改;仅直调方补形参)+
  `-race`(service/server);`npm run typecheck` 干净 + `vite build` 重嵌 go build(提交前还原占位 index.html)。
- service(gitcode_test 同款本地 bare remote + fake PullPublisher 记录):
  项目机密 CRUD/解析(项目覆盖公司;写无项目 token → skip);分支名/prefix/slug 单测;`ensureIssueBranch` 幂等;
  发 PR 端到端(push 后 bare 见分支 + CreatePull(owner/repo/title/head/base/body Resolves #N)被调 +
  pull_request_url/number 落库 + audit pr_opened;无 token / scripted 静默 skip);项目编辑(meta 更新 /
  占位仓 clone 认领 / 带历史 ErrInvalid / name 冲突);SyncProject(fixture)issue→task。
- github client httptest(NewClientAt):CreatePull 路径/body/auth header/201;DefaultBranch。
- server 路由:项目 secrets PUT/GET/DELETE、PUT project、POST project intake/sync(fixture + bound repo seed)、
  POST tasks/{id}/publish-pr(未完成/无 token → 映射错误,不发网络)。
- live dogfood(手动,需用户 GitHub PAT):os-self 代码源卡填项目 token → Web 同步 issue → 认领 run → 自动推分支
  开 PR → 人 GitHub merge;无 PAT/live 端点则离线等价覆盖 + 如实标注边界(同 stages/5 惯例)。

## 六、登记

- 登记 phase10/design/README.md;进度总表 Phase 10 行续记;归档收尾写 stages/6.md。
- 定稿门:本会话 ExitPlanMode 计划通过(2026-09-09)。方向文档 declarative-pipelines.md **正文未改**(记忆偏好:方向文档最终版)。
