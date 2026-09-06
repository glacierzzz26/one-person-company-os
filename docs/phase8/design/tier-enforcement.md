# Phase 8.4 — 档位分层强制(endpoint.tier + 角色默认解析 + CLI/控制台可见)(实施契约)

> Phase 8 子阶段实施契约,承接方向设计 [model-runtime.md](model-runtime.md) §六 8.4 行
> (endpoint.tier(迁移 0010)+ 角色默认解析 + CLI/控制台可见;建单不指定端点 → 按默认档落到对应端点,显式覆盖仍生效)
> 与 §十 定稿记录 1/2(档位三档 + 角色默认映射已认可)、§八 验收 2;以及 8.3 契约
> [delegation-and-judging.md](delegation-and-judging.md) §六风险与 8.3 归档 [stage 3](../stages/3.md)「留给 8.4」行。
> **定稿门决策(2026-09-06,三项,见 §八 定稿决策)**:① test 分槽 = 新增 `task.test_endpoint_id`(0011);
> ② writer 默认只从 cheap 且可作 claude 后端(proto=anthropic)解析,无则留空;③ 判读槽默认档无 active 端点 → **建单即硬失败**。
> 已定稿(2026-09-06,定稿门通过)。定稿冻结,按它写代码;落地后归档 `stages/4.md` 并在进度总表登记。
> 本文件是 8.4 的执行蓝本:**代码现状全部经仓库核实,类型/签名照抄真实代码,不凭想象**。

## 一、范围与边界

**做(8.4 交付物):**

1. **档位表达(A)**:endpoint 增 `tier` 三档 `frontier|standard|cheap`(UI 中文「高智/均衡/经济」),
   与既有 `role`(pool/planner/standby)正交;**迁移 0010** 只增不改加列,存量行回填默认档。
2. **test 判读与 review 把关分槽(B)**:task 增 `test_endpoint_id`(**迁移 0011**),test 默认落 standard、
   review 默认落 frontier(方向 §三表定稿)不再挤同一 reviewer 槽;`engEndpointFor` test 分支先看 test 槽。
3. **角色 → 档默认解析落建单(C)**:engineering 任务建单(live 语义)未显式给某角色端点时,按方向 §三表默认映射
   从公司端点池确定性解析落槽;**显式端点永远覆盖默认**;解析不到匹配档 active 端点 → **建单即硬失败**(判读槽,
   提示补档或显式端点);writer 因 B 决策「空 = claude 自带」为合法态不硬失败。
4. **CLI/控制台可见(D)**:`os endpoint select --tier` 指派/改档、list/show 显 tier;控制台端点池页增 tier 列(中文 tag),
   类型/API 对齐;建单默认落档在 task create 审计里可见所选模型(方向 §八 验收 2)。**不做新 UI 页面**。

**不做(留给后续/live,防越界):**

- **不改 role 语义与 runtime planner 解析**:planner 拆解仍走 `intakeEndpoint`(role=planner active 优先,否则 pool)。
  「planner=frontier」由运营把 role=planner 指到 frontier 档端点来表达(控制台/CLI 显 tier 辅助选择),8.4 不做
  运行时按档自动换 planner 端点。intake triage 端点同理不动(triage 不在 §三 映射表内)。
- **不做预算上限/成本仪表盘**:方向 §七「需要时后续立项」;8.4 只落默认分层。
- **不改变 scripted 离线红线**:scripted 下建 engineering 任务沿用现语义(端点可空,离线可复现冒烟不破坏)。
- **不改已冻结 Phase 0–7 契约与 8.1/8.2/8.3 语义**:回合机/熔断/审批/委派/审计字段逐字保留;迁移只增不改。
  存量端点(0007 建,无 tier)回填 `standard`,不因未标档而失效。

## 二、代码现状核实(8.4 立足点,全部经仓库核实)

| # | 事实 | 位置 |
|---|---|---|
| 1 | endpoint 表**无 tier 列**(role/status/models_cache…);任务表**无 test_endpoint_id**,只有 `writer_endpoint_id`/`reviewer_endpoint_id`(0008 加,均 `REFERENCES endpoint(id)` 可空) | `db/schema.sql` task L60-95、endpoint L161-174;`0008_task_round.up.sql` |
| 2 | 最新迁移版本 = 9(文件 `0009_repo.up.sql`,名称 repo);0010/0011 是下一个连续版本;runner `//go:embed migrations/*.sql`,按整数前缀升序应用未 applied,只增不改 | `internal/storage/migrate.go`、`internal/storage/migrations/` |
| 3 | sqlc v1.31.1 独立二进制 `/home/rguo/go_workspace/bin/sqlc`;`sqlc.yaml` schema=`db/schema.sql` queries=`db/queries` gen→`internal/storage/query`(**schema.sql 须与迁移终态一致**,迁移是运行时 delta) | `sqlc.yaml` |
| 4 | sqlc 查询几乎全 `SELECT *` / `RETURNING *` → 加列后生成的 struct 自动带新列;**例外是 INSERT 显式列清单**,CreateEndpoint / CreateTask 的 INSERT 必须加列(sqlc 参数随之) | `db/queries/endpoint.sql`、`db/queries/task.sql` |
| 5 | domain 模型:`endpoint.Endpoint` 无 Tier;`task.Task` 有 `WriterEndpointID/ReviewerEndpointID *string`(json snake_case),无 TestEndpointID | `internal/endpoint/model.go`、`internal/task/model.go:28-29` |
| 6 | repo 层:CreateEndpoint/CreateTask 把 model 逐字段映射进 sqlc params(`ptrToNull` 处理可空),回读 `toEndpoint`/`toTask`(`nullToPtr`);加列要动 mapper 两处各 +1 | `internal/storage/repository/endpoint.go`、`task.go` |
| 7 | 判读端点解析 `engEndpointFor(t, role)`:**test 与 review 都回退 `ReviewerEndpointID` → 否则 `WriterEndpointID`** —— test=standard 与 review=frontier 无法用同一槽同时表达(本契约 fork ①根治点) | `internal/service/engine.go:101-114` |
| 8 | engCall live:writer → delegateWriter(委派);test/review → `engEndpointFor` 拿端点 → `GetEndpoint`(非 active 报错)→ `modelCall`(判读必须 proto=openai 网关,proto 门在 modelCall) | `internal/service/engine.go:47-99` |
| 9 | **writer 端点是 claude 委派 env 源**:`delegateEnv` 把 `WriterEndpointID` 端点读成 `ANTHROPIC_BASE_URL/AUTH_TOKEN/MODEL` 喂 claude;头注明确「OpenAI 方言网关不能作 claude 后端」→ writer 默认落档**只该在 proto=anthropic(可当 claude 后端)的端点里挑**,随手塞 openai cheap 会弄坏委派(fork ②依据) | `internal/service/delegate.go:341-367` |
| 10 | 建单漏斗:CLI `os task create` / intake `createIssueTask` / planner `createPlannedSubtask` / 通道 A workflow 工程节点 **全部经 `createTask`**(TaskParams.WriterEndpointID/ReviewerEndpointID *string,空 → nil);子任务继承父端点 | `internal/service/task.go:39-68`、`plan_driver.go:113-122`、`intake.go:204-208` |
| 11 | engineering 判定/端点目前**不在建单校验**:没端点建单成功,到 live 阶段才 fail-closed(`no <role> endpoint set (use --writer-endpoint/--reviewer-endpoint…)`);scripted 永不查端点 | `internal/service/engine.go:58-66`、driver 冒烟 |
| 12 | CLI endpoint 面:add(company/name/base-url/token/proto)、list(列含 ROLE)、show、models、select(`--model` 必填 + 可选 `--role pool|planner|standby`)—— tier 照 role 先例走 select 指派;CLI task create 有 `--writer-endpoint/--reviewer-endpoint`,show 行含两端点 | `internal/cli/endpoint.go`、`task.go:83-84,131` |
| 13 | server/console:create-task req `{…, writer_endpoint_id, reviewer_endpoint_id}` → `strPtr`;endpoints select handler body `{model, role}`;TS `Endpoint` type 无 tier、AddEndpointReq/CreateTaskReq 无 test | `internal/server/api.go`、`web/src/api/types.ts`、`api/endpoints.ts`、`pages/Endpoints.tsx` |
| 14 | audit:task create 现为 detail 空;endpoint select audit detail `model+" role="+e.Role`。默认落档要在 create audit 里可见所选模型(§八 验收 2) | `internal/service/task.go:64-66`、`endpoint.go:116-127` |

## 三、契约设计

### A. 档位表达 + 迁移(0010 / 0011,只增不改)

- 迁移 **0010_endpoint_tier**:`ALTER TABLE endpoint ADD COLUMN tier TEXT NOT NULL DEFAULT 'standard';`
  (存量 0007 端点回填 `standard` —— 不因未标档失效);down = `DROP COLUMN tier`。
- 迁移 **0011_task_test_endpoint**:`ALTER TABLE task ADD COLUMN test_endpoint_id TEXT REFERENCES endpoint(id);`
  (可空,同 writer/reviewer 先例);down = `DROP COLUMN test_endpoint_id`。注释点明:test 判读槽(standard 默认),
  与 review(仍走 reviewer_endpoint_id,frontier 默认)分槽。
- `db/schema.sql` 同步(加 tier 列到 endpoint 表、加 test_endpoint_id 到 task 表,须含 REFERENCES 与可空);
  改 `db/queries/endpoint.sql`(CreateEndpoint INSERT 加 tier;新增 `UpdateEndpointTier`)+ `db/queries/task.sql`
  (CreateTask INSERT 加 test_endpoint_id);跑 `/home/rguo/go_workspace/bin/sqlc generate` 重生成
  `internal/storage/query`(加列自动进 `SELECT *`/`RETURNING *` struct)。**无 go.mod 变更**。
- 档位合法性:service 层校验(同 role 先例,无 DB CHECK)—— `frontier|standard|cheap`,select 指派时校验。
  UI/CLI 中文标注:`frontier=高智、standard=均衡、cheap=经济`(方向 §十 1)。

### B. task.test_endpoint_id 分槽(test=standard ≠ review=frontier)

| 角色 | 槽位(task 列) | 默认档 | 解析约束 |
|---|---|---|---|
| test | `test_endpoint_id`(新,0011) | `standard` | active + proto=openai(判读走网关) |
| review | `reviewer_endpoint_id`(既有) | `frontier` | active + proto=openai |
| writer | `writer_endpoint_id`(既有) | `cheap` | active + **proto=anthropic**(可作 claude 后端);空 = 合法(见 C/§十 ②) |

- `engEndpointFor`(engine.go)test 分支改为:`test_endpoint_id` → `reviewer_endpoint_id` → `writer_endpoint_id`(显式覆盖 + 渐进回退保持);
  review 分支不变(reviewer → writer)。
- TaskParams / task.Task / createTask / plan_driver 子任务继承 / CLI `--test-endpoint` / server create req `test_endpoint_id` 全链新增。
- task create/list/show、console Task 类型按「该字段是否展示」补齐(show 已有 writer/reviewer 行,补 test 行即可)。

### C. 角色 → 档默认解析落建单(createTask 单一钩子)

**触发条件(三者缺一即跳过默认解析):**
1. `p.ToolName == "engineering"`(判读/委派只在 engineering 回合出现);
2. **live 语义**(`OS_ENGINE_MODE != scripted`,与 engCall 同源判据;scripted 建单沿用现语义、离线红线不动);
3. 该角色槽位为空(非空 = 显式覆盖,不解析、不硬失败)。

**解析规则(确定性):** 在 `ListEndpoints(ctx, companyID)` 的 active 端点里,按角色默认档 + proto 约束挑选
(见 B 表);命中多个 → `created_at ASC, id ASC` 取最早(确定性、无随机)。命中即落槽;未命中按角色分:
- **review / test(判读槽)→ 硬失败**:`createTask` 报错,消息含「公司 + 角色 + 默认档 + 无 active 网关端点」指引
  (`os endpoint add --proto openai …` 再 `os endpoint select <id> --tier <tier>`,或本次建单显式 `--reviewer-endpoint/--test-endpoint` 覆盖)。
- **writer(执行槽)→ 不硬失败**:未命中 cheap+anthropic → 留空(claude 委派走自带鉴权/模型,`delegateEnv` 返 nil,现语义)。
  writer=cheap 是「够便宜就便宜配对;没有就自带」,空不是配置错误 —— 与 fork ② 定稿一致。

**落槽后可见性:** createTask 的 audit detail 现为空;当确有默认落档时,detail 写 `writer=…/test=…/review=…`
(格式 `reviewer=<id>:<model>;test=<id>:<model>;writer=<id>:<model>`,只列本次默认解析命中的槽;显式给的槽不重复列),
满足 §八 验收 2「日志/审计可见所选模型」。CLI `os task create` 成功输出可补一行「default endpoints resolved」摘要(可选)。

**硬失败的影响面(如实)**:通道 B intake(live)在未配多档端点的公司上建 engineering 任务会在 createTask 失败 →
该 issue 不入账本,每次 sync 重试并报错(配置指引可见,修好即通);scripted intake 不受影响(触发条件 2 不满足)。
单网关端点公司(只有 1 个 standard openai):显式给了 reviewer 且 test 空 → test 默认解析命中该 standard 端点(等价原回退);
review 显式覆盖跳过;writer 空合法。真正新增的硬失败 = review/test 槽空且公司无对应档网关端点 —— 即 8.4 的「强制」面。

### D. CLI / API / 控制台可见

- **CLI**:`endpoint select <id>` 增可选 `--tier frontier|standard|cheap`(校验;与 `--role` 正交可同给);
  `endpoint list` 加 TIER 列、`endpoint show` 加 TIER 行(中文标注 `高智/均衡/经济`——CLI 通篇中文、循方向 §十 1;原拟「取原值与 ROLE 同风格」实施时改判,见 §七 修订 ②)。
  task create 加 `--test-endpoint`。
- **server**:select 端点 handler body 增可选 `tier`;create-task req 增可选 `test_endpoint_id`。
- **控制台(不新增页面)**:`Endpoint` TS type 加 `tier`;端点池页 Endpoints.tsx 加 TIER 列(中文 tag,仿 ROLE pill);
  select 弹窗(如有)加 tier 可选项;Task 类型按 §B 是否展示端点补 `test_endpoint_id`。

### E. 端点指派语义

role 先例照搬 tier:`os endpoint select <id> --tier frontier` → `SetEndpointRole` 同款 `UpdateEndpointTier`
(repo,`updated_at` 刷新)+ service `SetEndpointTier`(审计 actor 变体)+ audit `endpoint/<id>/tier` detail=tier。
`AddEndpoint` 签名**不动**(tier 由列默认 `standard` 起步,select 改档)—— 避免 AddEndpoint 全调用点(CLI/server/测试)签名涟漪;
fake-gateway live 测试端点自然是 standard,与 §C 硬失败判定自洽。

## 四、文件落地清单(8.4)

| 文件 | 内容要点 |
|---|---|
| `internal/storage/migrations/0010_endpoint_tier.up/down.sql`(新) | endpoint 加 `tier TEXT NOT NULL DEFAULT 'standard'`;down 删列 |
| `internal/storage/migrations/0011_task_test_endpoint.up/down.sql`(新) | task 加 `test_endpoint_id TEXT REFERENCES endpoint(id)`;down 删列 |
| `db/schema.sql`(改) | 两表补列(与迁移终态一致,sqlc schema 源) |
| `db/queries/endpoint.sql`(改) | CreateEndpoint INSERT 加 tier;新增 `UpdateEndpointTier`(RETURNING *) |
| `db/queries/task.sql`(改) | CreateTask INSERT 加 test_endpoint_id |
| `internal/storage/query/*`(再生成) | `/home/rguo/go_workspace/bin/sqlc generate`;diff 应仅限新列与新查询 |
| `internal/endpoint/model.go`(改) | `Tier string json:"tier"`(头注补三档说明) |
| `internal/task/model.go`(改) | `TestEndpointID *string json:"test_endpoint_id"` |
| `internal/storage/repository/endpoint.go`(改) | CreateEndpoint params 加 Tier 且**空档归一 standard**(INSERT 现显式列 tier,空串会绕过 DB DEFAULT;见 §七 修订 ①);`toEndpoint` 加 Tier;新 `SetEndpointTier` |
| `internal/storage/repository/task.go`(改) | CreateTask 映射加 TestEndpointID;`toTask` 加 TestEndpointID |
| `internal/service/endpoint.go`(改) | `SelectEndpointModelAs` 增可选 tier 参数(或新 `SetEndpointTierAs`),audit detail 带 tier;校验三档 |
| `internal/service/tier.go`(新) | tier 常量(frontier\|standard\|cheap)+ `isScriptedEngine`(live 门)+ `applyEndpointDefaults(ctx, companyID, writer, reviewer, test)`(三槽解析/硬失败/writer 宽容,返回落槽指针 + create audit 命中明细)+ `isNilOrEmpty`;中文标注放展示层(CLI `tierCN` / console `tierLabel`),service 不持(见 §七 修订 ②) |
| `internal/service/task.go`(改) | TaskParams.TestEndpointID;createTask:engineering+live → 默认解析落槽 + 硬失败报错 + create audit detail 列默认落档;构造 t.TestEndpointID |
| `internal/service/engine.go`(改) | `engEndpointFor` test 分支先看 TestEndpointID |
| `internal/service/plan_driver.go`(改) | 子任务继承 TestEndpointID |
| `internal/cli/endpoint.go`(改) | select `--tier` 校验;list 加 TIER 列;show 加 TIER 行 |
| `internal/cli/task.go`(改) | create `--test-endpoint`;show 补 test 端点行 |
| `internal/server/api.go`(改) | create-task req `test_endpoint_id`;endpoint select body `tier`(均 strPtr/可选) |
| `web/src/api/types.ts`(改) | `Endpoint.tier`;`Task.test_endpoint_id`(若展示);req types 对齐 |
| `web/src/api/endpoints.ts`(改) | `selectEndpointModel(id, model, role?, tier?)`;addEndpoint 不变 |
| `web/src/pages/Endpoints.tsx`(改) | TIER 列 + tier 中文 label(仿 ROLE pill);select 弹窗 tier 可选 |
| 测试(见 §五)+ `docs/phase8/stages/4.md`(落地后归档)+ `design/README.md` + `docs/进度总表.md` | 收口凭证 |

依赖:全 stdlib + 既有 sqlc;两迁移;无 go.mod 变更。

## 五、用例清单(验收凭证)

harness 沿用 service 既有(company + endpoint seeding + 假网关 + seedGitWorkspace)。**8.4 对既有 live 用例的影响**:
建单默认解析只在「engineering + live + 槽位空」触发;既有多数 live 用例显式给 writer/reviewer、且已 seed 端点
(迁移后默认 standard)→ test 槽自动解析到同一端点(与原 reviewer 回退同效),review 显式覆盖跳过 → 影响面小;
个别全空端点用例按新契约适配(补端点或 scripted)。

| # | 用例 | 断言 |
|---|---|---|
| A1 | 迁移回填 | `storage.Open(tmp)` 应用 0010/0011;老端点 tier='standard'、task 表有 test_endpoint_id;down 可逆(0009 旧库升级路径) |
| A2 | sqlc/repo 全链 | CreateEndpoint/GetEndpoint/ListEndpoints 回读带 tier;CreateTask/GetTask 回读带 test_endpoint_id(含 nil 态 ptrToNull 往返) |
| B1 | engEndpointFor 分槽 | test: test_endpoint_id 命中 → 用它;空 → 回退 reviewer → writer(既有语义保持);review 不变 |
| C1 | 默认解析确定性 | company 多 active 端点:按 tier+proto 过滤、created_at ASC 取最早;review 取 frontier+openai、test 取 standard+openai、writer 取 cheap+anthropic |
| C2 | 建单落档 + audit | engineering + live + 全空 → 三槽自动落位(公司配齐三档);create audit detail 列 `reviewer=…;test=…;writer=…`(含 model) |
| C3 | 显式覆盖优先 | 显式给 writer/reviewer → 不解析不改;test 显式给 → 落显式值 |
| C4 | 判读槽硬失败 | engineering + live + test 空且公司无 standard+openai → createTask 报错(消息含默认档 + 指引);review 空且无 frontier 同理 |
| C5 | writer 宽容 | 无 cheap+anthropic → writer 留空、建单成功(scripted 同);有 cheap+anthropic → 落槽且 delegateEnv 产出 ANTHROPIC_* env |
| C6 | scripted 红线 | OS_ENGINE_MODE=scripted 建 engineering(全空端点)成功、不走解析;冒烟回归零改动 |
| C7 | 端到端闭环(live 假网关) | 公司配 standard(test)+ frontier(review)+ cheap+anthropic(writer)→ 建单无端点全落位;执行:writer 委派(fake)、test 判读**走 test 端点**、review 走 review 端点(假网关按端点分回,断言各自命中);task 完成 |
| C8 | CLI/API/控制台 | `endpoint select --tier` 校验+落库;list/show 显 tier;create-task payload `test_endpoint_id` → 200 回读;httptest select body tier;web tsc --noEmit 干净 |

验收口径:
1. `gofmt` 本次改动干净;`go vet ./...`、`go build ./...`、`go test -count=1 ./...` 全绿;**0–7 与 8.1 既有测试零改动**;
   8.2/8.3 测试仅按新契约适配(live 建单默认解析 + 端点 tier seeding);`-race ./internal/service/` 亦绿。
2. `sqlc generate` 后 diff 仅新增列/新查询(不漂移);两迁移上/下可逆;无 go.mod 变更。
3. 离线冒烟(§八 验收 2):`os server`/CLI 端点池可给多档端点标 tier;engineering 建单不显式给端点 → 审计可见默认落档所选端点/model;
   显式覆盖仍生效。scripted 分界与确定性保持。
4. **live 验收归用户**:真实网关多档端点 → 建单默认落档跑通 test=standard/review=frontier 真实判读;cheap claude 委派配对 —— 同 6.x/8.2 惯例。

## 六、风险与取舍

- **硬失败是行为变更**:未配多档端点的公司,live engineering 建单(judging 槽空)从「建单成功、运行期报错」改为「建单即报错」;
  单网关公司因默认回填 standard 且判读槽常显式给,实际新失败面 = 「槽空 + 无对应档网关端点」,属 8.4 强制目标本身。
- **intake live 依赖配置**:未配档公司 channel B 每 issue 建单失败、不入账本、每次 sync 重报 —— 配置指引在错误消息内,
  属「强制」副作用(scripted intake 不受影响);备选「intake 跳过默认解析」不做(会静默退回运行期失败、失去强制意义)。
- **writer=cheap 仅指 anthropic 方言**:把 openai 网关 cheap 端点塞进 `ANTHROPIC_BASE_URL` 会弄坏委派 —— 契约已按
  proto=anthropic 收窄默认解析(fork ②);显式 `--writer-endpoint` 若指 openai 方言端点,行为与现状一致(运行时委派报错,用户自负)。
- **建单时引擎模式即进程模式**:live 门取建单进程 `OS_ENGINE_MODE`(与 engCall 同源);若建单(scripted)与执行(live)跨进程换模式,
  解析跳过 → 运行期回到既有 fail-closed,不新增坏路径。
- **tier 缺省 `standard` 的语义**:存量端点全部落中档,最不激进;真实档位由 select 指派(方向 §八 验收 2 建端点时标注)。
- **sqlc regen 噪音**:schema/queries 变更必然重生成 query 包 —— 验收 2 以 git diff 限定在新增列/查询,不漂移既有方法。

## 七、实施修订记录

(2026-09-06 按 §四 落地;live 验收归用户。与契约的偏差逐条记明:)

① **repo CreateEndpoint 空档归一 `standard`**:sqlc `CreateEndpoint` INSERT 显式列出 tier 列后,Go 侧不传 tier(AddEndpoint 无 tier 参数、测试 seed 未给)会以空串绕过 DB `DEFAULT 'standard'`(§六「未标档端点落中档」落空)→ mapper 层把空档归 `standard`,契约意图(全调用点零签名涟漪 + 默认落中档)保持。
② **中文标注归展示层**:方向 §十 1 的 `frontier=高智/standard=均衡/cheap=经济` 只在 CLI/控制台显示时需要,console(JS)本就不能 import Go;service 不持 CN map(避免单消费者死码)。CLI 自持 `tierCN`、console 自持 `tierLabel`。
③ **CLI list/show TIER 用中文标注**(原 §三 D「取原值、与 ROLE 同风格」改判:CLI 通篇中文、循方向 §十 1);console 端点池以独立「档位」列(pill-muted)呈现 + select 弹窗加 tier 可选项(与 role 正交可同给)。
④ 空槽默认解析 + 判读槽硬失败 + create audit 命中明细,全部收敛在 `createTask` 单钩子内完成(触发门 engineering + `!isScriptedEngine()`;显式槽不解析不硬失败);硬失败消息带公司 + 档位 + add/select/显式 flag 指引。
⑤ sqlc regen diff 仅新增列 + `UpdateEndpointTier`,不漂移既有方法(§六风险项过)。
⑥ **既有测试按新契约适配**(§五):seedEndpoint 落 standard → 多数「显式 reviewer + test 槽自动解析到同端点(与原回退同效)」用例零改动;纯 writer 路径(无判读端点)用例补 `seedJudgeDefaults`(frontier+standard,baseURL 假地址不触网);其余全绿。零改动面:0–7/8.1 全量。
⑦ 迁移 0010/0011 于每次 `storage.Open`(每个测试库)自动应用;task/endpoint 新列经 sqlc 全链路回读(A2)、new contract 用例(tier_test.go / server tier_api_test.go)与既有 service/server/console 测试共同验证。


## 八、登记

- 定稿后登记 `docs/phase8/design/README.md`(8.4 row ✅)+ `docs/进度总表.md`(子阶段明细 + 总览 Phase 8 行 8.4 ✅)
  + 落地后归档 `docs/phase8/stages/4.md`。
- 定稿决策(2026-09-06,AskUserQuestion 三项):① test 分槽 = 新增 `task.test_endpoint_id`(0011);
  ② writer 默认只从 cheap 且可作 claude 后端(proto=anthropic)解析,无则留空;③ 判读槽默认档无 active 端点 → **建单即硬失败**。
  (注解:C 硬失败范围 = 判读槽 review/test;writer 空为合法态随 ② 不硬失败;解析与硬失败均以 live 语义为门,保 scripted 红线。)
