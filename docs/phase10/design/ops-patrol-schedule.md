# Phase 10.2 — schedule 调度 + ops_patrol 巡检真形态(实施契约·定稿)

> 实施契约(方向:declarative-pipelines.md 定稿 §八 10.2 行「schedule 调度 + 只读凭据引用 + 委派 claude 巡检
> + 报告/证据写进项目目录 + 发现 → 通知/处置」;验收锚「到点拉起 → 报告产出 + 审计;异常触发处置链」)。
> 10.1 冻结边界原句:schedule 字段已落库(默认 '')但**不解析不调度**;kind=ops_patrol 与 bugfix 同一 run 语义,
> 「形态差异 10.2-10.4」。本契约 = 把这两句兑现:**schedule 解析+到点拉起** 与 **ops_patrol 的巡检形态**。
> **契约定稿门(2026-09-08,AskUserQuestion 四项)**:① schedule 语法 = **cron 五段**(`0 9 * * *`);
> ② 只读凭据引用 = **后移**(巡检落项目目录内例行检查,无凭据 fail-closed 不触网;真实生产凭据归 10.4+);
> ③ 处置链 = **fix+high 自动链拉同项目 bugfix**(low/medium/缺流水线仅通知留人);④ 调度 = **独立全局开关
> (schedule_poll_sec,0=关;不绑 queue-work)** + **报告正文控制台内嵌阅读**(受控读端点)。→ 本契约冻结,按 §三 实施。

## 一、范围与边界

### 做(10.2 交付)
1. **schedule 调度(到点拉起)**:pipeline.schedule 从「存而不解」变为可解析可触发。语法 = **cron 五段
   `分 时 日 月 周`**(定稿门①;子集见 3.5)+ `""`/`off` = 不调度。server 增 ScheduleLoop(**独立开关定稿门④** =
   全局 app_setting `schedule_poll_sec`,0=关,默认关):轮询 active+非空 schedule 流水线,命中即**以系统 actor 触发一次
   run**(复用 `RunPipelineAs`,不另起执行体);到点只建单,执行仍受既有 queue-work 总开关(产品「后台执行」语义,Web 文案讲清)。
2. **ops_patrol 巡检真形态(runPatrol)**:认领到 kind=ops_patrol 的 run 走**独立巡检驱动**,不再掉进 bugfix 回合机:
   项目目录内委派 claude 只读巡检 → 报告/原始证据落 **`<root>/patrol/<taskID>.md`** 且 OS git 提交(8.3 提交语义,
   只 add 该报告文件,绝不动无关改动)→ OS 读回报告正文 → 网关模型判读(D4:对**证据文本**做结构化裁决,不信 agent 自述)
   → 裁决 → 通知 / 链式处置。离线:scripted 公司(engine_mode=scripted,DB 生效,产品零 env)走确定性同路径,
   报告文件真落盘真提交 —— **二进制冒烟可离线端到端验「到点拉起 → 报告产出 + 审计」**。
3. **发现处置链(定稿门③)**:判读产出结构化 verdict `{ok, severity, action, summary, findings}`:
   - ok → 绿,完成任务,审计记 report 路径 + verdict,不通知。
   - 非 ok → `notifyCompany` 推飞书(公司无 webhook 机密 → 静默跳过记日志,同 digest best-effort)+ 审计;
     action=fix 且 severity=high 且项目下有 active bugfix 流水线 → **先完成任务、再链式拉起同项目 bugfix 工程任务**
     (request = verdict.summary,actor=`system:patrol_chain`,变更仍过既有审批门);否则仅通知,处置留人(审计记 manual)。
4. **task 加 `pipeline_id`(0014,只增不改)**:run 产物反链流水线 → worker 认领时能判 kind 分流、Web runs 行能显 kind;
   删除流水线先断引用(历史任务保留,同 10.1 project_id 语义);删除项目双清 project_id+pipeline_id。
5. **作者面 + 报告正文内嵌阅读(定稿门④)**:Web 流水线 schedule(cron)建/改;ProjectDetail 流水线表显调度、行内改调度;
   runs 表按 pipeline 显 kind + ops_patrol 行显裁决行(ok/severity/action + 报告路径);「查看报告」→ **受控读端点**
   (`GET /projects/{id}/patrol/{taskID}`,路径由服务端按 task 推导,无客户端路径入参 → 无穿越面;仅已完成且 result 带
   patrol: 标记的 run,读回 `patrol/<taskID>.md` 正文,限长截断)→ Web 抽屉渲染正文(纯文本,不做 markdown 渲染面)。
6. **runPatrol 与既有 driver 分流**:`runClaimed` engineering 家族里,kind==ops_patrol → runPatrol;kind≠patrol 或
   流水线已删 → 落回 runEngineering(默认语义兜底)。

### 不做(边界)
- 不做「只读凭据引用 → 委派进程注入 env / 真打生产」(定稿门②,后移 10.4+ 与高智合成同批)。边界原句(方向 §五):
  OS 不自研逐 call 外部编排;新外部触点前置 = 凭据最小化 + 只读优先 + 主机 allowlist + 变更过人。前置未齐前,巡检落在
  **项目目录内可机械捕获证据的例行检查**(build/依赖/git 卫生/残留/一致性等,检查项 = 流水线 description 即意图)。
  **无凭据配置 = fail-closed:巡检不触网**,承 9.4 fail-closed 产品取向。
- 不做「阶段计划落库/可审」(10.3);不做高智合成/预算(10.4)。D3 例行模板化先行 —— 10.2 巡检 = **OS 固定模板
  (只读 + 报告路径 + 证据格式 + 裁决契约)+ 流水线 description 即检查项清单**,模型只「照着干 + 产出证据」;
  跨天可比性靠同 description 同报告格式。
- 不改 0-9/10.1 冻结契约与语义(回合/端点/审批/审计/API 字段逐字保留);迁移只增不改;产品零 env(9.4 反向门控)
  —— scripted 公司是 DB 配置的合法产品态(离线红线),不读 env。
- 不做 pipeline 其余字段编辑(名称/意图/kind/risk 改 = 删建;10.1 冻结);不做 disable/enable(暂停调度 = schedule 置
  ""/off)。
- 不做 code//knowledge/ 子目录实体化;报告子目录 `patrol/` 是 OS 在项目根下新增的类型化工作目录(委派面内),不挪动用户内容。
- cron 子集外语法(月/星期名 `MON/JAN`、`?`、`L/W/#`、秒/年字段)不做,校验报错给指引(见 3.5)。

## 二、代码现状核实(立足 2026-09-08,10.1 完结态;行号以当前工作树为准)

- **迁移最新 = 0013_project_pipeline**(`internal/storage/migrations/`);FK 强制 `internal/storage/db.go:19` →
  删除先断子行引用(10.1 已验证)。task 列已有 project_id(TEXT REFERENCES projects);本次只增 pipeline_id。
  pipeline.schedule 已落库(默认 ''),`internal/pipeline/model.go:52` 注明「10.2 才解析;本期恒空」。
- **建单链**:`internal/service/pipeline.go` `RunPipelineAs(ctx, pipelineID, request, actor)` → GetPipeline/GetProject/
  CountActiveTasksByProject 串行守卫 → `CreateTaskAs(TaskParams{…, ProjectID:&prj.ID, MaxAttempts:1}, actor)` →
  audit("pipeline", id, "run")。createTask 走 `internal/service/task.go:69`(engineering live 空槽默认落档/硬失败,tier.go;
  scripted 公司跳过)。**改点**:TaskParams 增 PipelineID;CreateTask INSERT 扩 30 列(pipeline_id)。
- **worker 分流**:`internal/service/execution.go` `runClaimed(workerID, t)`: isEngineeringTask(t)(tool=="engineering")
  → `s.runEngineering`(`internal/service/driver.go:34`)。**改点**:t.PipelineID 非空 → 查 pipeline,kind==ops_patrol →
  新 `s.runPatrol`;查不到/≠patrol → runEngineering。driver live 才触网;scripted 公司全角色确定性桩 `engine.go`
  engCall→engScripted;9.4 envSeam 门测试专用(产品恒关)。
- **委派/提交资产(delegate.go)**:git exec(`delegate.go:162`)、`.claude/.codex`→`.git/info/exclude`、
  `commitDelegation(ws,msg)`(`delegate.go:253` 唯一提交者,agent 简报禁自 commit)、captureNet。简报消费 Title/
  Description(`delegate.go:414-416`)。**复用**:runPatrol 报告提交 = `git add -- patrol/<file>` + commitDelegation
  (显式路径,不 add -A → 脏工作树不吞用户改动)。
- **判读资产**:`judge.go` decodeJudgeJSON + parseTest/parseReview(JSON 主契约 + 单行兜底);判读走网关 text Chat
  `engine.go modelCall`(proto=openai);档位 `engEndpointFor(t, role)`(review→reviewer→writer)。**改点**:新增
  `parsePatrolVerdict`(同 decodeJudgeJSON 族)。verdict 不可解析 → 不默认绿(engFail 语义)。
- **通知**:`internal/service/notify.go` `notifyCompany(ctx, companyID, text)`(公司 feishu_webhook 机密;无 → nil 跳过)。
  复用发巡检告警。company engine_mode 覆盖 = DB 生效(9.3):scripted 公司是产品合法态。
- **定时循环范式**:`internal/server/server.go` `SetDigestTime/SetQueueWork/DigestLoop/QueueLoop/queueOnce`;
  `internal/cli/serverconfig.go` `serverConfigFromApp(app settings.AppSetting)` → `os server` 启动把 app_setting 落到
  server 字段。**改点**:AppSetting 增 `SchedulePollSec`(缺省 0=关)→ `srv.SetSchedulePoll(d)`;Server 增
  `ScheduleLoop`(gate>0)+ 可测单 tick `scheduleOnce`。
- **HTTP**:`internal/server/api.go` 路由 + `apiOK/handleServiceErr`(ErrInvalid→400/ErrBusy/Conflict→409/ErrNoRows→404)。
  10.1 无 pipeline 更新路由、无文件读端点 —— 本期加 **PUT /pipelines/{id}**(schedule)与 **GET /projects/{id}/patrol/{taskID}**
  (报告正文,路径服务端推导)。
- **Web**:10.1 已交付 `/projects` `/projects/:id` 两页三 Modal(建项目/建流水线/运行);ProjectDetail 流水线表 + 最近 runs
  表(10s 轮询);`api/types.ts`/`endpoints.ts` 全列 snake_case(Pipeline.schedule 已随 read 返回);设置页 9.3 全量
  (queue/digest 行)。
- **CLI**:`internal/cli/pipeline.go` list 列含 SCHEDULE、show 含 Schedule。作者面(建流水线 + 改调度)留 Web;CLI 读 + run + delete。

## 三、契约设计

### 3.1 实体与迁移(0014_pipeline_run)

`internal/storage/migrations/0014_pipeline_run.up.sql`(只增不改;含 down):

```sql
-- task 反链流水线:run 产物挂 pipeline → worker 认领判 kind / Web runs 显 kind / 审计追到流水线。
ALTER TABLE task ADD COLUMN pipeline_id TEXT REFERENCES pipelines(id);
-- down: ALTER TABLE task DROP COLUMN pipeline_id;
```

删除顺序(承 10.1「先断引用后删」):
- `DeletePipelineAs`:先 `UPDATE task SET pipeline_id=NULL WHERE pipeline_id=?`(ClearTaskPipeline;历史 run 保留)再删 pipeline。
- `DeleteProjectAs`:ClearTaskProject 扩为同条 UPDATE 双清 `project_id=NULL, pipeline_id=NULL WHERE project_id=?`
  (项目 tasks 的 pipeline 必属本项目)再级联删 pipelines。

repository(store)/sqlc(`db/schema.sql` 同步):
- `db/queries/pipeline.sql`:`UpdatePipelineSchedule`(UPDATE schedule RETURNING *)、`ListScheduledPipelines`
  (SELECT id, project_id, name, kind, schedule FROM pipelines WHERE status='active' AND schedule != '' ORDER BY created_at;
  project 行 FK 保证存在)。
- `db/queries/task.sql`:`ClearTaskPipeline`(UPDATE task SET pipeline_id=NULL WHERE pipeline_id=?);`ClearTaskProject` 扩双清;
  `CreateTask` INSERT 扩 30 列含 pipeline_id。→ sqlc regen(可空 FK WHERE 参数 → sql.NullString,ptrToNull 接;
  占位符数对列)。

### 3.2 repository / 领域模型
- `internal/storage/repository/pipeline.go`:UpdatePipelineSchedule / ListScheduledPipelines;断引用编排放 service
  (同 10.1 DeleteProjectAs 编排)。
- `internal/storage/repository/task.go`:CreateTask 接 PipelineID *string;ClearTaskPipeline。
- `internal/task/model.go`:Task 增 `PipelineID *string`(ProjectID 之后)。Pipeline 不变。

### 3.3 新包 `internal/schedule`(cron 五段,纯 Go 无依赖)
- `Parse(s string) (Cron, error)`:`""`/`off` → 不调度(scheduled=false);否则 5 段解析。段界空白(空格/tab)。
- 段子集(逐字段):`*` | `n` | `a-b` | `a-b/n` | `*/n` | 逗号并集;数值界:分 0-59 / 时 0-23 / 日 1-31 / 月 1-12 /
  周 0-6(0=周日,**7 亦按周日接受**,cron 传统)。名字(MON/JAN)、`?`、`L/W/#`、秒或年字段、`@daily` 等 → ErrInvalid
  带指引(「5 段数字 cron;如 0 9 * * * = 每天 9:00」)。dom/dow 语义 = **cron 标准 OR**(两者都受限时任一命中即匹配)。
- `func (c Cron) Next(now time.Time) time.Time`:严格晚于 now 的下一次命中;从 now+1min 起按「分→时→日→月/周」逐层
  进位搜索(最坏一个月内收敛,每次命中仅算一次,开销可忽略)。纯函数,全量单测。
- `func (c Cron) NextAfter(now time.Time) time.Time` 别名不需要 —— Server 只用 Next。

### 3.4 service(pipeline/schedule/task)
`internal/service/pipeline.go`:
- `CreatePipelineAs(...)` 增尾部参 schedule(cron 校验:空/off 合法=不调度,非法 → ErrInvalid 带指引);`CreatePipeline`
  同步扩参(既有调用方补 "");CreatePipelineAs 落库 schedule 原文。
- 新 `UpdatePipelineScheduleAs(ctx, id, schedule, actor)`(校验 → store.UpdatePipelineSchedule → audit("pipeline", id,
  "edit", "schedule "+old+"→"+new))。
- `RunPipelineAs`:TaskParams 增 `PipelineID: &pl.ID`。actor 常量:`actorSched="system:schedule"`、`actorChain="system:patrol_chain"`。
- `ListScheduledPipelines(ctx)` 薄 wrapper(store 查询结果,供 server 调度)。
- `DeletePipelineAs`:前置 ClearTaskPipeline(断引用再删)。

`internal/service/execution.go`(分流):
```go
if isEngineeringTask(t) {
    if t.PipelineID != nil {
        if pl, err := s.store.GetPipeline(ctx, *t.PipelineID); err == nil && pl.Kind == pipeline.KindOpsPatrol {
            return s.runPatrol(ctx, workerID, t, pl)
        } // 流水线已删(sql.ErrNoRows)/kind≠ops_patrol → 落回 runEngineering(默认语义兜底)
    }
    return s.runEngineering(ctx, workerID, t)
}
```

`internal/service/patrol.go`(新,巡检驱动):
```
runPatrol(ctx, workerID, t, pl):
  前置 = runEngineering 同款:needsApproval→requestApproval;MarkTaskRunning;audit "patrol_start"
  runCtx(TimeoutSec)
  reportRel = "patrol/<t.ID>.md";abs = filepath.Join(t.WorkspacePath, reportRel)
  mkdir patrol/;产出报告(3.5 委派/scripted)
  git add -- <reportRel> + commitDelegation(msg 含 pl.Name/t.ID)→ audit "patrol_delegate"(失败 → engFail)
  OS 读回报告正文(限长)→ 判读 verdict(3.5)→ parsePatrolVerdict
  CompleteTask(result = "patrol:<reportRel>|ok=<ok>|severity=<..>|action=<..>|" + summary 首行) → audit "patrol_complete"
  非 ok 处置(3.6):notifyCompany(best-effort)+ fix&high&同项目 active bugfix → RunPipelineAs(bugfixID, summary, actorChain)
    → audit "patrol_chain";链缺/忙/非 fix → audit "patrol_manual" 留人
  失败 = engFail("patrol",…):无报告文件/verdict 不可解析/超时 → requeue 或 fail,不默认绿
```

`internal/service/task.go`:`TaskParams` 增 PipelineID *string → createTask 落列。

### 3.5 runPatrol 的委派 / 判读
- **只读简报(delegatePatrol,模板)**:cwd=项目根;允许读全部 + 只写 `patrol/<t.ID>.md`;禁止改 patrol/ 外任何文件、
  禁 git commit/push、禁装依赖等副作用;检查项 = pl.Description;产出物每发现带**原始证据**(命令输出/返回码/文件摘录,
  不写 claude 自述总结)。live → claudeDelegator(与 writer 同族,agentCLIRegistry);scripted 公司 → OS 写确定性
  fixture 报告(内容固定含证据行,离线红线)。
- **判读(仅对证据)**:OS 提交后读回报告正文,以 patrol 判读 prompt(固定模板,含 report 正文 + 检查项)喂
  `engEndpointFor(t, engRoleReview)`(frontier;reviewer→writer 兜底);scripted → 确定性 verdict(envSeam 门内
  OS_SCRIPT_PATROL:ok 默认 / finding-high / finding-medium / finding-low / malformed;产品不可达)。
  输出 JSON 契约 `{"ok":bool,"severity":"low|medium|high","action":"none"|"fix","summary":"..","findings":[…]}`;
  `judge.go parsePatrolVerdict`(decodeJudgeJSON 同族)。**verdict 缺失/不可解析 → 任务失败(engFail),不自动绿**。

### 3.6 处置链(定稿门③)
| verdict | 动作 |
|---|---|
| ok | 完成任务;审计记 report 路径 + verdict;不通知 |
| 非 ok & fix & high & 项目下有 active bugfix | 完成任务 → notifyCompany → **链式拉起**同项目 bugfix 工程 run(actor=system:patrol_chain;request=summary) |
| 其余非 ok(low/medium/缺 active bugfix/非 fix) | 完成任务 → notifyCompany(无 webhook 静默跳过)→ 审计记 manual,处置留人 |

chain 拉起的 bugfix run 落回 runEngineering → 既有审批门/熔断/审计全覆盖(变更过人)。

### 3.7 schedule 调度循环(server;独立开关定稿门④)
- `settings.AppSetting.SchedulePollSec`(缺省 0=关)→ `serverConfigFromApp` → `os server` `srv.SetSchedulePoll(d)`;
  Server 持 `scheduleNext map[string]time.Time`(pipelineID → 下次命中)。
- `scheduleOnce(ctx)`(可测单 tick,范式同 queueOnce):
  1. 刷新 ListScheduledPipelines;
  2. 对每条 schedule!=off:无 map 项 → 初始化 `Next(now)`(**重启不补历史命中**,见风险);有项且 `now >= next` → 触发
     `RunPipelineAs(id, "", actorSched)`:ErrPipelineBusy/Conflict(同项目他跑在飞)→ 记日志 skip;其他硬错记日志不断流
     → **无论成败 advance** `next = Next(next)`(单次命中至多触发一次,天然去重);
  3. 删掉的流水线从 map 清理。
- `ScheduleLoop(ctx)`(gate=schedulePollSec>0;ticker d;每 tick scheduleOnce;异常记日志不退出,同既有循环)。
- 触发与执行解耦:scheduleOnce 只建单;任务执行仍由 queue-work 消费(文案在 Web 讲清)。

### 3.8 HTTP `/api/v1`(api.go 追加;信封/鉴权沿用;写 actor=human:console)
- **PUT `/pipelines/{id}`** body `{"schedule":"0 9 * * *"|""}` → UpdatePipelineScheduleAs;非法 → 400 bad_request;不存在 → 404。
- **GET `/projects/{projectID}/patrol/{taskID}`** → 报告正文读端点(定稿门④):
  - 服务端按 taskID 推导路径 `patrol/<taskID>.md`(join task.WorkspacePath),**无客户端路径入参**;
  - 校验:task 存在且 task.ProjectID==projectID;task 已完成且 `result` 以 `patrol:` 前缀开头(确系巡检产物);
  - 读文件(存在性 + 限长 64KiB 截断带 truncated 标记)→ `{ok, data:{task_id, path, truncated, content}}`;
    越权/缺失/非巡检 run → 404/400,防穿越由「服务端推导 + 白名单目录」双保险。
- 其余路由不动。

### 3.9 CLI(internal/cli/pipeline.go 微调)
- 不改命令组;list 已列 SCHEDULE、show 已含 Schedule。run/delete 不变。注释注明 schedule 作者面 = Web。

### 3.10 Web(作者面 = 验收核心;功能必须跟上)
- `api/types.ts`/`endpoints.ts`:schedule 字段(已在)+ `updatePipelineSchedule(id, cron)` + `getPatrolReport(projectID,
  taskID)` helper。
- `components/modals.tsx` PipelineCreateModal:加 **Schedule(cron)** 输入(占位 `0 9 * * *`;前端 cron 校验 helper 与后端
  同语法;Alert:5 段 分 时 日 月 周,留空=不调度;每天9点= `0 9 * * *`、工作日9点=`0 9 * * 1-5`、每30分=`*/30 * * * *`;
  提示「到点自动拉起需设置页开 定时调度 + 后台执行」)。
- `pages/ProjectDetail.tsx`:
  - 流水线表加列 **调度**(cron 或 '—');行操作「改调度」→ 小 Modal(单输入 cron/清空)→ PUT → bump。
  - 最近 runs 表加 **形态** 列:run.pipeline_id 对已载流水线 map 显 kind 徽标(已删流水线 → 兜底文案);ops_patrol 行显
    **裁决**(result 首行解析 ok/severity/action + 报告路径摘要)+ 「查看报告」按钮 → getPatrolReport → Modal/抽屉纯文本
    渲染正文(无 markdown 库;长文截断提示)。
- `pages/Settings.tsx`(9.3 全量设置页):加一行 **定时调度轮询(秒,0=关)** 说明调度总开关(全量 PUT 天然兼容新字段;
  核实设置页字段枚举补 label)。

### 3.11 边界裁定说明(供实施/review 对照)
- 调度/链 actor = `system:schedule`/`system:patrol_chain`:审计 actor 无格式锁(10.1 human:* 之外新增 system:* 合法);
  Web 鉴权/console 白名单按 path,不含内部 actor 判定,不受影响。
- ops_patrol 可手动 run(不设 schedule 也 run)→ 同入口 RunPipelineAs,认领分流。
- schedule 对三种 kind 都合法(到点 = RunPipelineAs;bugfix/develop 到点 = 「定时跑一遍意图」,不新增语义)。
- 报告 `patrol/` 属项目目录内:项目 delete 不动磁盘(10.1 红线);patrol/ 随项目目录自然保留。
- 巡检完成态 = status/qstatus=completed + result 文本一体可查;裁决错误不回传通知链;报告正文读端点只服务已完成的
  巡检 run。

## 四、文件落地清单

| 文件 | 内容 | 说明 |
|---|---|---|
| `internal/schedule/cron.go` + `cron_test.go`(新) | cron 五段子集解析 + Next(now) | 纯 Go 无依赖 |
| `internal/storage/migrations/0014_pipeline_run.up/down.sql`(新) | task.pipeline_id 只增列 | down 删列 |
| `db/schema.sql` + `db/queries/pipeline.sql` / `task.sql`(改) | schema 补列;UpdatePipelineSchedule / ListScheduledPipelines / ClearTaskPipeline / CreateTask+列 / ClearTaskProject 双清 | sqlc 源;regen |
| `internal/storage/query/*`(regen)+ repository/pipeline.go / task.go(改) | 上述 Store 方法;CreateTask 接 PipelineID | sql.NullString/ptrToNull 沿用 |
| `internal/task/model.go`(改) | Task 增 PipelineID *string | 只增 |
| `internal/service/pipeline.go` / `task.go` / `settings.go`(改) | cron 校验 + CreatePipelineAs 扩参 + UpdatePipelineScheduleAs + RunPipelineAs 挂 PipelineID + ListScheduledPipelines + 删除断引用 + AppSetting.SchedulePollSec | 全走 As+audit |
| `internal/service/patrol.go`(新) | runPatrol(委派/报告提交/判读/处置)+ delegatePatrol 简报 + notify/chain | scripted/live 同路径 |
| `internal/service/judge.go`(改) | parsePatrolVerdict | 结构化契约 |
| `internal/service/notify.go`(改) | patrol 告警文案(复用 notifyCompany) | best-effort |
| `internal/service/execution.go`(改) | runClaimed 分流(kind==ops_patrol → runPatrol) | 默认语义兜底 |
| `internal/server/server.go`(改)+ cli/serverconfig.go / server.go(改) | SetSchedulePoll + ScheduleLoop/scheduleOnce + serverConfigFromApp 增字段 | 循环范式同 queue/digest |
| `internal/server/pipelines_api.go`(新) | PUT /pipelines/{id}(schedule)+ GET /projects/{id}/patrol/{taskID}(受控读正文) | 信封/鉴权沿用 |
| `internal/service/*_test.go` + `internal/server/*_test.go`(新) | §五 S* 用例 | scripted 公司全链;server 触发+作者面+读端点 |
| `web/src/…`(types/endpoints/modals/ProjectDetail/Settings/dicts) | cron 作者面 + runs 形态/裁决/报告抽屉 + 设置行 | tsc/vite 绿 |
| `docs/phase10/stages/2.md`(新)+ `docs/进度总表.md`(改)+ `docs/phase10/design/README.md`(改) | 归档/登记凭证 | 收口 |

## 五、用例清单(定稿后逐条落地;命名 S*)

scripted 公司(engine_mode=scripted,DB 生效;product 零 env;OS_SCRIPT_PATROL 仅测试 seam 内可读):
- **S1** cron 校验(service):`0 9 * * *` / `*/30 * * * *` / `0 9 * * 1-5` / `0 9 1 * *` / `0 0 * * 7`(=周日)合法;
  `""`/`off` = 不调度;4 段 / `60 * * * *` / `* 24 * * *` / `0 9 * * MON`(无名字)/ `@daily` / `abc` → ErrInvalid。
- **S2** Next 纯函数:从 `2026-09-08 09:00:30` 起 `0 9 * * *` → 次日 09:00;跨月/跨年;dom/dow OR(如 `0 0 13 * 5` 逢
  13 号或周五);月末不越界。
- **S3** UpdatePipelineScheduleAs:改调度 + 审计(human:cli 与 human:console 各一,detail 含 old→new);非法 → 400。
- **S4** RunPipelineAs(actor=system:schedule)→ pipeline 审计 actor=system:schedule;建出 task.pipeline_id=流水线 id。
- **S5** scripted 巡检 run(runPatrol):`patrol/<taskID>.md` **真落盘且 OS git commit**(工作树归 clean;断言 git log 有该
  提交、无未提交 diff)→ 完成;result 以 `patrol:` 开头含路径+ok;审计 patrol_start/patrol_delegate/patrol_complete 齐。
- **S6** verdict ok → 无通知、无链;审计记绿。
- **S7** verdict finding-high + fix + 同项目 active bugfix → 先完成 patrol → 链拉起 bugfix 工程任务(desc=summary、同
  project、actor=system:patrol_chain);两 run 齐。
- **S8** verdict finding-high + 无 active bugfix / 非 fix / low-med → 仅 notify(best-effort)+ 审计 manual,不建任务。
- **S9** verdict malformed(不可解析)/报告缺失/超时 → 任务失败(engFail),不默认绿。
- **S10** 分流兜底:pipeline 已删(sql.ErrNoRows)/kind≠ops_patrol → runClaimed 落回 runEngineering。
- **S11** 删除断引用:DeletePipeline → run 历史 task 保留且 pipeline_id 置空;DeleteProject → 双清、任务保留、磁盘仍在。

server 层(无 env seam;建单不执行;执行验证在 service/scripted):
- **Srv-1** PUT /pipelines/{id} schedule 200 + read-back + audit human:console;非法 → 400;不存在 → 404。
- **Srv-2** 建 ops_patrol(带 cron)+ run(判读档端点配齐,承 10.1)→ 信封 task.pipeline_id=流水线 id;活跃再 run → 409。
- **Srv-3** scheduleOnce 单 tick:seed 到点 cron(`* * * * *`,scheduleNext 预置过去)→ 触发建单 + audit system:schedule;
  QueueWork 关不影响触发(SchedulePoll 独立);ScheduleLoop 在 SchedulePollSec=0 → 即返;同一次命中不双发(advance 后 next 未来)。
- **Srv-4** GET /projects/{id}/patrol/{taskID}:scripted 完成的巡检 run(报告已落盘)→ 200 正文;非巡检 run / 未完成 /
  project 不匹配 → 404/400;超大报告截断标记。

Web:tsc --noEmit + vite build 绿;cron 建/改校验、runs 形态/裁决列、报告抽屉人工过一遍。
二进制冒烟(scripted 公司离线端到端):init → server → setup → 建 scripted 公司 + project + ops_patrol(cron=每分
`* * * * *`)→ 设置 PUT schedule_poll_sec + queue_work 开 → 等到点 → 断言 report 落盘、git log 有 OS patrol 提交、
task completed、CLI/HTTP 读回可见、GET patrol 读端点返回正文;PUT schedule "off" 停调度;GET / 控制台可服务。

## 六、风险与取舍
- **调度触发 vs 执行脱钩**:到点 = 建单(便宜,秒级轮询),执行 = queue-work(后台总开关)。只开调度不开 queue → 任务堆
  pending —— 产品语义「自动拉单、不自动干」,设置页/Modal 文案讲清;Web 每流水线可见状态。
- **cron 子集宽度**:数字 5 段 + 范围/步进/并集,无名字/`?`/`L/W/#`/秒年 —— 校验报错给指引,不静默接受;当前真用例
  只有每日/每分。dom/dow 按 cron 标准 OR 语义(两受限任一命中)。
- **重启丢一次命中**:scheduleNext 内存态,重启后 `Next(now)` 从未来算起,漏过的本次命中不补(记日志说明);单 operator
  可接受。宽限缺失由轮询 ≤ 秒级 + advance 去重兜底(每 tick ≤ pollSec 延迟触发,不双发)。
- **scripted 公司巡检是真实落盘提交**(不是假 diff):离线红线(不触网、DB 生效)与 D4(证据真文件)一致;live 差异只在
  报告内容来源(claude vs fixture)。
- **脏工作树**:报告提交只 `git add -- patrol/<file>`,不吞用户未提交改动;claude 简报禁写 patrol/ 外。
- **verdict 可信度**:只对 OS 读回的**报告证据文本**判读,不信 agent 自述;不可解析 → fail 不绿;链拉仅在 fix+high 门,
  low/med 只通知;自动拉起的 bugfix 仍过审批门(变更过人)。
- **报告读端点越权面**:路径服务端推导(无客户端路径入参)+ 白名单 `patrol/<taskID>.md` + 限已完成且 result 带 patrol:
  标记;限长截断。最大暴露 = 用户读自家项目内自己可写的文件,不构成越权;不做通用文件浏览。
- **pipeline_id FK**:删除流水线/项目都先断引用(10.1 已验证删除顺序模式);迁移只增不改。
- **task 增列回归面**:CreateTask INSERT 30 列 + sqlc 生成多处 task 列清单同步;10.1 已吃过 30vs29 占位符,实施时对列数。

## 七、登记
- 关联:declarative-pipelines.md §八 10.2(验收锚);10.1 契约/归档 stages/1.md(冻结边界原句兑现);config-governance.md
  (9.3 app_setting 全量 PUT 兼容新字段);direction 开放点 2/3(凭据注入、闭环到真实世界 —— 本期取 后移+链拉 姿态)。
- **定稿门(2026-09-08)四项**已记录于文件头;本契约冻结 → 登记 design/README.md → 实施 → 验证 → 归档 stages/2.md +
  进度总表 10.2 ✅ → 一次 commit。
