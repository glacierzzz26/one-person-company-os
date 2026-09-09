# Phase 10 — 项目 ⇄ GitHub 自动代码获取(方向 + 实施契约)

> 实施契约 + 方向(2026-09-09;**本会话 ExitPlanMode 计划通过 = 契约定稿门**,用户选定能力形态
> 「建时 clone + run 前自动拉」与首个真实绑定「OS 仓库自身 dogfood」)。承 [declarative-pipelines.md](declarative-pipelines.md)
> 方向定稿 D7 案 A(项目 = company 下新一等目录容器实体;channel B repo 收敛为 project 的 **code 源绑定**);
> 本契约把 D7 的「反向派生 code 源」补成**实取代码的完整半环**:项目绑一个 GitHub 地址,本地目录**可为空/不存在**,
> OS 建项目时自动 clone 代码进来,并在每次工程 run 前自动拉到上游最新。
>
> 用户需求原话(逐条保留):「项目应该和 github 的项目进行绑定,bugfix 和 feature 一般都是来源对应
> github 的 issue,所以要定时同步,或者通过 webhook」「这个需求已经有了吗,不能支持自动获取代码?」
> 「我希望一个项目能绑定一个 guthub 地址,有可能是个空目录」。

## 一、范围与边界

### 做
1. **建时 clone**:`project create --repo-url <GitHub URL>` —— root 已存在且 git → 复用(不覆盖本地 origin/内容);
   空目录或不存在 → OS 自动 `git clone` 把代码拉进来;非空非 git → 既有 400(指引不变)。clone 完成后
   origin 即代码源单一来源,D7 `ensureCodeSource` 自动认领(repo 绑定行 + CODE_SOURCE 展示),零新增列。
2. **run 前自动拉**:工程 run 的**首个 fresh 认领(基线未钉)**前 best-effort `git pull --ff-only`;
   失败/离线 → 审计 `eng_sync` 一条、不阻塞 run(离线开发不因断网卡死)。
3. 四表面 `repo_url` 透传:CLI `--repo-url`、HTTP POST /companies/{id}/projects `repo_url`(可空)、
   service `CreateProjectAs` 形参、Web 新建项目 Modal 输入。

### 不做(边界)
- 不新增 DB 列/迁移:GitHub 地址单一来源 = 克隆后 `origin` → `osrepo.Repo.RepoURL`(建时 URL 仅驱动 clone)。
- 不做 GitHub 定时同步/常驻 webhook 改表:issue→任务通道 B(Phase 6.3)已存在;本契约只补**代码**获取。
- 不改 `internal/tool/git.go` agent 网络 git 拒绝墙(keep:宿主侧 clone/pull 不经此墙,agent 仍不能联网 git)。
- 不引入私有仓库 token 注入(clone/pull 凭据):**预留**,首期 dogfood 用公开仓库匿名 clone。
- run 中任何时刻不拉(基线已钉 / 脏 / 续跑返工 / patrol 一律跳过,防污染相位净 diff 锚点)。

## 二、现状缺口核实(立足 2026-09-09,Phase 10.4 完结态;行号以当前工作树为准)

- `internal/service/project.go:40` `rootReady(ctx, rootPath)` 只做「已 git 复用 / 空目录或不存在 `git init` /
  非空非 git → ErrInvalid」,**从不 clone**(原契约 project-pipeline-foundation.md §一.1 明确「不 clone」)。
- 代码源 = D7 **反向派生**(`ensureCodeSource`,CreateProjectAs 尾):读本地已有 `origin`;空 git init 后无
  remote → 无 repo 绑定 → 无代码可跑。
- run 认领唯一入口 `delegateBaseline`(delegate.go):只校验「git + (首次)clean」,无任何拉取;每相位 diff
  锚定基线 `refs/os/tasks/<id>`,**中途拉会污染净 diff** → 拉取只能放在「基线未钉的首个 fresh claim」。
- 宿主 git helper(`gitDirCmd`/`wsIsGit`/`wsPorcelainClean`/`gitRemoteOrigin`)在 delegate.go 已齐,无 env/token 参数。

## 三、决策与默认(定稿门,本会话计划通过 = 批准)

1. **URL 持久化位置 = 不新增列**:克隆后 `origin` 即代码源单一来源(D7 `ensureCodeSource` 派生),避免迁移 + sqlc 重生成。
2. **clone 失败 → 建项目失败**(rootReady 在落库前,错误清晰回退),不留半绑定项目。
3. **run 前拉取 = best-effort 首认领一次**:非 scripted + 基线未钉 + porcelain clean + 有 origin →
   `git pull --ff-only`(20s 超时);其余(续跑/返工已钉、脏、scripted、无 origin)一律不拉;失败审计不阻塞。
4. **已 git 项目 + repo_url → 复用本地**(不覆盖 origin/内容);空/不存在目录 + repo_url → clone。

## 四、实施改动

### A. 建时 clone(service 层)
- `internal/service/project.go`:`rootReady(ctx, rootPath)` → `rootReady(ctx, rootPath, repoURL string)`,
  clone 分支 —— `repoURL==""` → 现行为(mkdir + git init);已存在空目录 + URL → `git clone <url> <abs>`;
  不存在路径 → `MkdirAll` 后 clone;已存在 git → 直接复用不 clone;非空非 git → 既有 ErrInvalid。契约注释同步更新。
- `CreateProjectAs(ctx, companyID, name, rootPath, description, repoURL, actor)` / `CreateProject(... , repoURL)`(CLI wrapper):
  显式加 repoURL 形参(全部直调方更新);rootReady 之后落库 + audit + `ensureCodeSource`(clone 后 origin 在 → 自动认领,零改动)。
- 新宿主 helper `internal/service/delegate.go:175` `gitCloneURL(ctx, repoURL, dest)`(宿主 `git clone`,
  失败带合并输出回显;token 注入预留,永不写 token 进 origin)。

### B. run 前自动拉
- 新方法 `internal/service/delegate.go:396` `pullRunFresh(ctx, t)`:
  guards = 非 scripted、ws 是 git、基线未钉(`rev-parse --verify --quiet refs/os/tasks/<id>` 失败)、
  porcelain clean、有 origin → 20s 超时 `git pull --ff-only`;错误 `audit eng_sync` 一条 + 静默返回(不 error)。
- `delegateBaseline`(delegate.go:368,scripted 早退后)注入 `s.pullRunFresh(ctx, t)` —— 单点覆盖
  runEngineering(driver.go)与 runSynthesized(synthesize.go),且两者都只在 `!humanOverride` 下调用 → 天然跳过
  熔断续跑与 scripted;runPatrol 不调它 → patrol 不拉(本期不覆盖)。

### C. `repo_url` 四处表面(可空)
- CLI `internal/cli/project.go:102/117/118`:`project create --repo-url` + Short/Flag 文案(空/不存在 + URL → 自动 clone)。
- HTTP `internal/server/projects_api.go:74/83`:`apiCreateProject` DTO 加 `RepoURL string json:"repo_url"`(可空)→ `CreateProjectAs(..., req.RepoURL, consoleActor)`。
- Web `web/src/components/modals.tsx:269-311`(ProjectCreateModal GitHub URL 输入 + 工具提示 +
  文案「填 GitHub URL 则 OS 自动 clone;留空 = 本地 git init」)+ `web/src/api/types.ts:357` `CreateProjectReq.repo_url?: string`(endpoints 透传,无改)。

### 文件落地清单
(改)`internal/service/project.go`(rootReady/CreateProjectAs/CreateProject + 注释)、
`internal/service/delegate.go`(gitCloneURL + pullRunFresh + delegateBaseline 注入)、`internal/cli/project.go`、
`internal/server/projects_api.go`、`web/src/components/modals.tsx`、`web/src/api/types.ts`;
测试直调方补 `""` 形参(service project_test/codesource_test + server plan_api_test/pipelines_schedule_test);
(新)`internal/service/gitcode_test.go`(GC1-GC7)。**零迁移**。

## 五、验证

- `go vet ./...` / `go build ./...` 干净;**`go test -count=1 ./...` 全绿**(既有 600+ 用例零 body 改)+
  `-race`(service/server)绿;`npx tsc --noEmit` 干净。
- **GC1-GC7(离线本地 remote 免网络)**:
  - GC1/GC2 空目录 / 不存在路径 + repoURL → clone 落盘(base.txt 在)+ git + origin=源;
  - GC3 已 git 项目 + repoURL → 复用(origin/内容不被覆盖);GC4 非空非 git + repoURL → ErrInvalid 且不落库;
    GC5 clone 源不可达 → 建项目失败不落库(rootReady 先于 DB insert);
  - GC6 首个 fresh 认领(基线未钉 + clean + 有 origin)→ 自动拉,HEAD 前移拿到新提交(feature.txt 在);
  - GC7 基线已钉 / 工作区脏 / scripted / 无 origin → 一律不拉(HEAD 不动,脏文件保留)。
- **live 冒烟覆盖边界**:run 前自动拉需 live endpoint + worker 真实认领才能端到端;无 live 引擎时以
  GC6/GC7(真实 git binary + 本地 remote)+ 建时 clone 二进制冒烟(dogfood 仓库 clone 落盘 + CODE_SOURCE 展示)等价覆盖,
  如实标注边界(见 stage 5 归档)。

## 六、后续边界

- 私有仓库 clone/pull:凭据注入方案预留(将来经 secret `github_token` 按 URL 主机注入 env,不写 origin;定稿门前不实现)。
- GitHub issue 定时同步 / webhook → 流水线链式触发(通道 B 常态化)仍留后续(本契约只取代码)。
- patrol run 前不拉(巡检读本地状态,防 repo 移动导致报告抖动)——本期边界,后续可评估加只读 fetch。

## 七、登记

- 登记 phase10/design/README.md;进度总表 Phase 10 行续记;归档收尾写 stages/5.md。
- 定稿门:本会话 ExitPlanMode 计划通过(2026-09-09)。方向文档 declarative-pipelines.md **正文未改**(记忆偏好:方向文档最终版)。
