# Phase 10.3 — 阶段计划契约通用化(run 计划账本 + 计划审批 + 逐阶段 I/O 验收)(实施契约)

> 实施契约(方向:declarative-pipelines.md 定稿 §八 10.3;定稿门三项决策,见 §七)。
> 目标:**把 run 的隐式阶段变成一等可观察物 —— 每条流水线 run 带一份持久化「计划账本」
> (有序阶段:目标/分配/状态/证据),Web 可看、可审;高风险 run 的计划先于人批(先审后干);
> 每阶段只在其 I/O 验收过门后 advance;D4「机械验证优先」在此拿到第一个 OS 直跑真相位。**
> 不含高智合成(10.4)。不含通用计划解释器(见 §一 边界)。

## 一、范围与边界

### 做(10.3 交付)
1. 新表 **task_plan / task_plan_phase**(迁移 0016,只增不改):流水线 run 的任务挂一份计划账本
   —— plan 行(kind/materized=计划形态)+ 有序 phase 行(seq/kind do|accept|dispose/标题/分配器/状态/
   evidence/note/起止)。只写读,驱动内部翻状态,无外部写口。
2. **双形态共一账本(定稿门①)**:
   - **ops_patrol → upfront 预铺**:run 建单时即铺齐全部阶段(pending),计划先于执行存在 →
     高风险先审时可看到「将要做什么」;runPatrol 在真实边界逐段翻状态、填证据。
   - **bugfix/develop(engineering)→ grow 记账**:建单只建 plan 行(materialized=grow),
     驱动在真实回合边界(writer→test→review / 熔断 / 拆解)执行时逐段 append do/accept 行。
     自适应回合语义不动、控制流不动,只插桩。
3. **计划审批 = 增强既有单门(定稿门②)**:不新起审批状态机、不加 pipeline 字段。risk=high 或
   审批策略命中 → 走既有 waiting_approval;因 patrol 计划已 upfront 预铺,Web 审批页随单展示该 run
   计划(先审后干 = 看清阶段再批准)。批准 = 既有 ApproveTask requeue → runPatrol 照计划执行翻状态;
   拒绝 = reject→failed。engineering(grow)无预铺计划,审批页标注「计划将在执行中生成」。
4. **逐阶段 I/O 验收 + D4 机械优先(定稿门③)**:patrol 判读前新增 **OS 自跑机械卫生预检层**
   (确定性、零网关):委派前对工作树 porcelain 快照 → 委派产出并 OS 提交报告后复检 —— 报告存在非空 +
   除「快照已有脏项」与「本次报告文件」外无任何新增/改动(委派越界残留 → fail)。机械层不过 → accept
   fail / engFail 不经判读;通过才进 frontier/scripted 判读(frontier 只评机械未覆盖域)。engineering
   的 test/review 仍是模型判读(OS 直跑 go test/build 属未来 build/test 型 run,不在本子阶段)。
5. **Web 全程跟上**:任务详情抽屉「计划」段(时间线:阶段 + 状态 Tag + 分配器 + 证据)、ProjectDetail
   runs 表计划入口、Approvals 决策弹窗随单展示预铺计划。

### 不做(边界)
- **不做通用计划解释器**:10.3 不给「任意声明的阶段序列」造一个 OS 通用驱动引擎;两驱动各自控制流
  不变,账本只如实记录 + 在真实边界翻状态。真正的「frontier 合成计划 → OS 逐阶段通用驱动」= 10.4
  合成进来时(方向 §三/§八)。本子阶段账本是它的落库/观察/审批底座。
- 不给 planner 拆解出的子任务(split,挂 parent_task_id)单建计划;账本挂顶层流水线 run 任务
  (子任务非流水线产物,天然无 plan)。
- 不做审批状态机/不新 pipeline 字段(定稿门②);不做 OS 直跑 go test/build 的机械验收(留未来形态)。
- 不新增 CLI(无新作者面;计划观察走 Web + 既有 audit;CLI 只读族不扩)。非流水线任务/0016 前历史 run
  无计划(GET 返回 plan:null)。
- 不改 0-10.2 冻结契约/表/语义;迁移只增不改;产品零 env(9.4 反向门控沿用);机械层纯 exec git
  (沿 delegate.go 既有 exec 用法,无新依赖)。

## 二、代码现状核实(立足 2026-09-08,10.2 完结态;行号以当前工作树为准)

- **迁移/数据管线**:`internal/storage/migrations/` embed FS 自动扫描 `NNN_*.up/down.sql`;当前上限
  **0015_schedule_poll** → 新迁移 = **0016_task_plan**。FK ON(`internal/storage/db.go:19`)。
  schema 权威 = `db/schema.sql`(sqlc);sqlc → `internal/storage/query`(入库)→ `internal/storage/repository`
  (Store 方法;范式 workflow.go/repo.go)。
- **task 模型**:`internal/task/model.go` Task{... Status(业务:pending|running|waiting_approval|completed|
  failed,model.go:12)、QStatus、Risk、RoundNo/ConflictCount(model.go:27,回合状态驱动内)、
  ProjectID/PipelineID(model.go:31-32)、WorkspacePath、Writer/Reviewer/TestEndpointID}。回合状态**在驱动
  内部,不另建状态机**(model.go:24 注)。
- **engineering 驱动**:`internal/service/driver.go:34 runEngineering` 单次认领同步跑完;前置
  needsApproval(driver.go:36)→ MarkTaskRunning+eng_start 审计 → planner 序曲 planEngineering
  (direct/split≤8 子任务/ask→审批,`internal/service/plan_driver.go:37`;split 子任务由 driveChildrenToDone
  同步驱动聚合)→ delegateBaseline(起点 git+clean)→ 回合 `for{...}`(driver.go:93):runEngPhase(writer)→
  runEngPhase(test;parseTest;免费返工 ≤engMaxFreeRework=3)→ runEngPhase(review;parseReview verdict
  approve/needs_changes)→ approve:CompleteTask(result=diff 文本)+eng_complete 审计;needs_changes→
  conflict+1→ round 推进重写;conflict≥engFuseMax=3 → 熔断 requestApproval(driver.go:162)。runEngPhase
  (driver.go:180)每次调用即建一条 execution 行(execution.go 实体,含 status/output)—— 工程每"子调用"
  已有执行记录,计划账本取其边界做 do/accept 聚合,不重复造。
- **审批机**:`internal/service/approval.go` needsApproval(task risk=high 或 company enabled approval
  policy;approval.go:15)/ requestApproval(建 approval 行 + RequestApprovalTask→waiting_approval + audit
  request + notify;approval.go:39)/ DecideApprovalAs(approve→ApproveTask(requeue)/reject|changes→FailTask;
  approval.go:78)。runClaimed(execution.go:75)与 runEngineering/runPatrol 各自入口先查 needsApproval。
- **runPatrol(10.2)**:`internal/service/patrol.go:37` 前置审批 → patrol_start 审计 → scripted fixture 写报告
  / live delegatePatrol(委派 claude 只读,报告由 agent 写 patrol/<id>.md,委派结束 os.Stat 校验存在) →
  commitPatrolReport(`git add -- patrol/<rel>` + commitDelegation;patrol.go:155)→ patrol_delegate 审计 →
  readReportCapped(64KiB;patrol.go:289)→ patrolJudge(frontier reviewer 端,patrol.go:221)/scripted
  OS_SCRIPT_PATROL 桩 → parsePatrolVerdict → CompleteTask(result=`patrol:<rel>|ok=..|severity=..|action=..|
  <summary首行>`,patrol.go:103)→ patrol_complete 审计 → !ok disposePatrolFindings(notify best-effort +
  fix&high&同项目 active bugfix 链拉 RunPipelineAs(actor=system:patrol_chain)/ 否则 patrol_manual;patrol.go:168)。
  报告提交仅 add 单文件 → 现有 patrol 无"委派越界残留"检查(残留会留在 porcelain 上不被发现)。
- **路由/api.go**:`GET /tasks/{id}`(apiGetTask)、`GET /tasks/{id}/executions`、`/approvals` +
  `POST /approvals/{id}/decision` 已存在(api.go:101-107);项目族 10.1/10.2(api.go:120-131)。
- **Web**:页面 pages/{Tasks,Approvals,ProjectDetail,Projects}.tsx 等;路由 App.tsx:75-87;组件
  TaskDrawer.tsx(任务详情抽屉,承接计划段)、DecideModal.tsx(审批决策弹窗,承接计划展示)、
  ProjectDetail.tsx(runs 表:形态列/裁决列/查看报告)。api/types.ts Task 含 pipeline_id(88);endpoints.ts
  逐端点薄 helper。make ui(web→internal/console/ui go:embed)+ tsc --noEmit。

## 三、契约设计

### 3.1 实体与迁移(0016_task_plan;只增不改;含 down)

```sql
CREATE TABLE task_plan (
    id           TEXT PRIMARY KEY,
    task_id      TEXT NOT NULL UNIQUE REFERENCES task(id),  -- 一条流水线 run 一份计划
    kind         TEXT NOT NULL,      -- patrol | engineering(计划形态;与驱动分流同源,pipeline.kind)
    materialized TEXT NOT NULL DEFAULT 'grow',  -- upfront(建单即铺全,可先审) | grow(执行中 append)
    created_at   INTEGER NOT NULL,
    updated_at   INTEGER NOT NULL
);
CREATE INDEX idx_task_plan_task ON task_plan (task_id);

CREATE TABLE task_plan_phase (
    id          TEXT PRIMARY KEY,
    plan_id     TEXT NOT NULL REFERENCES task_plan(id),
    seq         INTEGER NOT NULL,    -- 阶段序;grow 计划 append 递增
    kind        TEXT NOT NULL,       -- do | accept | dispose
    title       TEXT NOT NULL,       -- 人类可读阶段目标(含 round/报告名等)
    allocator   TEXT NOT NULL,       -- delegate | os | judge | planner | manual
    status      TEXT NOT NULL DEFAULT 'pending',  -- pending | running | ok | fail | skipped
    evidence    TEXT NOT NULL DEFAULT '',  -- 产出物引用/摘要:报告 rel / verdict 行 / execution id /
                                           -- diff 首行+行数 / commit —— 不整存大产出(execution/报告已存正文)
    note        TEXT NOT NULL DEFAULT '',
    started_at  INTEGER,
    finished_at INTEGER,
    UNIQUE (plan_id, seq)
);
CREATE INDEX idx_task_plan_phase_plan ON task_plan_phase (plan_id);
```

**最小外键**:plan 只 `REFERENCES task(id)`(project/pipeline/company 均经 task 联到)→ 项目/流水线删除
  断引用与历史保留语义完全沿用现有 ClearTaskProject/ClearTaskPipeline(task 本体不动,plan 随 task 留)。
  phase 不另建序号状态机;计划整体状态 = task.status 单一来源,plan 表不重复存总状态。

领域 struct(仿 workflow/model.go):`internal/plan/model.go` RunPlan{ID, TaskID, Kind, Materialized,
  CreatedAt, UpdatedAt};RunPlanPhase{ID, PlanID, Seq, Kind, Title, Allocator, Status, Evidence, Note,
  StartedAt, FinishedAt};常量 PlanKindPatrol/PlanKindEngineering、MaterializedUpfront/MaterializedGrow、
  PhaseKindDo/PhaseKindAccept/PhaseKindDispose、PhaseStatus 五态、Allocator 五值(与表注同串)。

### 3.2 repository(plan.go;仿 repo.go/workflow.go)
- GetTaskPlanByTask(ctx, taskID)(RunPlan, 不存在 ok=false)/ CreateTaskPlan(ctx, RunPlan)
- ListPhasesByPlan(ctx, planID)[]RunPlanPhase(按 seq)/ CreatePlanPhase(ctx, RunPlanPhase)
- UpdatePlanPhase(ctx, id, status, evidence, note, startedAt, finishedAt)(列指针,只更变列)
- SetPlanUpdated(ctx, planID, ts) — plan.updated_at 跟随最近 phase 变更

### 3.3 service(internal/service/plan.go 新;runPatrol/driver/runClaimed 插桩)
- `ensurePlan(ctx, t, kind, materialized)`(GetTaskPlanByTask→无则 Create):幂等,重复认领不重铺。
- `appendPhase / advancePhase(ctx, taskID, phaseRef, status, evidence, note)` 小助手:advance 首次进
  running 记 started_at,终态(ok|fail|skipped)记 finished_at + 刷 plan.updated_at。
- **RunPipelineAs(pipeline.go)增**:CreateTaskAs 成功后 `ensurePlan(t, planKindByPipelineKind, mat)`——
  kind==ops_patrol → **upfront** 并按 patrolPlanTemplate 一次铺 5 条 pending 行;bugfix/develop →
  grow(只建 plan 行)。(计划 pre-materialize 与建单同事务/同步,先于任何 worker 认领;8.4 落槽失败仍
  在建单即 400,不产生 plan。)
- **runPatrol(patrol.go)记账 + 机械层**(改,控制流仅加机械步与翻状态):
  - 进入:取 plan;phase[1].do → running。
  - **机械预检 pre**:委派/写报告前 `git status --porcelain` 快照脏项集合(与 delegate 前已有用户脏项
    区分)。
  - 产出报告(scripted fixture / live delegate)→ phase[1].do ok,evidence=`patrol/<id>.md`(字节数)。
  - phase[2].do(running→ commitPatrolReport)→ ok,evidence=commit 摘要/报告 rel。
  - **机械预检 post(new)**= phase[3].accept:报告存在非空(readReportCapped 已证)+ porcelain 复检 =
    {post 集合 - pre 集合} 仅允许空(报告文件已被 add+commit,不在 porcelain)→ 通过 ok,evidence=
    "mechanical: report N bytes; worktree residue=0";有越界残留 → phase fail + engFail("patrol",
    "inspector residue in worktree: …")(requeue/fail 按 attempt 既有语义)。
  - phase[4].accept(running→ judge 产出/scripted)→ parsePatrolVerdict;不可解析 → phase fail +
    engFail 既有文案;ok → phase ok evidence=verdict 审计行(ok/severity/action + summary 首行)。
  - CompleteTask + patrol_complete 审计(不变)→ 处置:ok → phase[5].dispose=skipped,note="ok — no
    disposition";!ok → dispose running → disposePatrolFindings → ok,evidence=chain/manual 结果。
- **runEngineering(driver.go)记账**(grow;插桩不改语义):RunPipelineAs 已建 grow plan;driver 内
  - planner split → append do "planner 拆解 N 子任务" allocator=planner;ask → 该行 note="ask → 人工审批"。
  - 每回合 R:writer 产出 diff 后 append do "写 round R" allocator=delegate evidence=exec id+diff 摘要
    (返工 #k 并进该行 note);test 结果 append accept "测试 round R" allocator=judge(ok=pass/免费返工 →
    note;engFail 路径 → fail);review verdict append accept "评审 round R"(approve→ok + 完成后 dispose?
    不,complete 由任务态表达 / needs_changes→fail note → 下一 R)。
  - 熔断 → 当行 fail + note "fuse → waiting_approval";approve 续跑(resume)后按新 R 继续 append。
- **审计策略**:计划行证据自身即账本,不为每 phase 翻状态另发审计(避免噪声;run/patrol_complete 等既有
  审计照旧)—— 决策记录于本契约,review 对照。
- 错误映射:无新 sentinel(计划失败随任务既有 requeue/fail 语义;engFail 复用)。

### 3.4 HTTP /api/v1(api.go 追加一条只读)
- `GET /tasks/{id}/plan` → apiGetTaskPlan:GetTask→404(未知);GetTaskPlanByTask → 无 → data
  `{task_id, plan:null}`;有 → `{task_id, plan:{kind,materialized,created_at,updated_at,
  phases:[{seq,kind,title,allocator,status,evidence,note,started_at,finished_at}]}}`(ListPhasesByPlan)。
  鉴权/信封沿用(console Bearer)。**无写口**。

### 3.5 Web(观察面 = 验收核心)
- types.ts + endpoints.ts:TaskPlan/PlanPhase 类型 + `getTaskPlan(taskID)`(路径逐字对 §3.4;plan 可 null)。
- TaskDrawer.tsx(任务详情抽屉;Tasks 页与 ProjectDetail 复用同一点):加「计划」区 —— plan:null →
  “非流水线 run / 历史 run 无计划”;有 → 阶段时间线(序号 + kind 徽标 do/accept/dispose + 标题 +
  分配器 + 状态 Tag + evidence 等宽截断 + note;grow 计划显示「执行中生成」注记)。
- ProjectDetail.tsx:runs 表行加「计划」动作 → 复用 TaskDrawer(与该行既有查看报告并存)。
- Approvals.tsx / DecideModal.tsx:对每条 pending 审批按 approval.task_id 拉 getTaskPlan ——
  upfront(patrol)→ 决策按钮上方渲染计划阶段列表 + 提示「批准 = 按上述计划执行」;grow/null →
  提示「计划将在执行中生成(自适应)」。先审后干在审批页落地。
- utils/dicts.ts:plan kind/phase kind/status/allocator 中文标签。

### 3.6 边界裁定说明(供实施/review 对照)
- 计划账本只服务流水线 run(RunPipelineAs 建单的任务);手动 createTask/workflow/拆解子任务不建 plan。
- grow 计划的 plan 行在建单即存在(空 phases),供 Web 区分「执行中生成」;patrol upfront 是 10.3
  「计划可先于执行存在」的唯一形态 —— 审批增强因此只对 patrol(及未来线性形态)展示完整计划。
- 机械预检只在 patrol(接受 run 的线性形态);engineering 无 OS 机械层(其 test/review 模型判读照旧)。
- 项目/流水线删除不新增 store 步骤(plan 最小 FK task;任务历史保留 → plan 随任务保留;删除永不碰磁盘)。
- Web = 计划观察/审批唯一作者面;无新 CLI;报告/执行正文仍存既有(execution/patrol 文件),evidence 只存引用/摘要。

## 四、文件落地清单
(新)`migrations/0016_task_plan.up.sql/.down.sql`;(改)`db/schema.sql` + `db/queries/plan.sql`(新)+
`sqlc generate`;(新)`internal/plan/model.go`;(新)`internal/storage/repository/plan.go`(+store.go 挂);
(改)`internal/storage/repository/task.go`?——无需,plan 独立实体;(新)`internal/service/plan.go`(助手+
模板);(改)`internal/service/pipeline.go`(RunPipelineAs ensurePlan+patrol 预铺)、`patrol.go`(机械层+翻
状态)、`driver.go`(grow 插桩);(改)`internal/server/api.go` + 新 handler(apiGetTaskPlan);(改)
`web/src/api/types.ts`、`endpoints.ts`、`components/TaskDrawer.tsx`、`components/DecideModal.tsx`、
`pages/Approvals.tsx`、`pages/ProjectDetail.tsx`、`utils/dicts.ts`;(新)test 文件(§五)。

## 五、用例清单(定稿后逐条落地;命名 T* / Tsrv-*;service 全在 scripted 公司)
- **T1** RunPipelineAs ops_patrol 建单 → task + task_plan(kind=patrol,materialized=upfront,5 条 pending
  行 seq1..5);grow(develop/bugfix)建单 → plan 行(materialized=grow)无 phase;非流水线 createTask 无 plan。
- **T2** patrol 全流程后账本镜像(scripted,真 git 项目):seq1/2/3/4 → ok + evidence(报告 rel / 提交 /
  机械行 / verdict 行);ok 巡检 seq5.dispose=skipped;OS_SCRIPT_PATROL=finding-high(fix&high+active
  bugfix)→ seq5.dispose ok + note 链拉。**既有 S5-S11 语义零 body 改**(仅新增翻状态副作用)。
- **T3** 机械层:报告写好后、pre 快照后塞多余残留文件 → seq3.accept fail → task engFail 不判读(OS_SCRIPT_
  PATROL 未消费);报告缺/空 → fail;报告正常且无残留 → 通过进判读。委派前已存在用户脏项(pre 快照含)
  → 不误伤。
- **T4** engineering direct 跑通 → 账本 append 形状(do 写 R → accept 测试 R → accept 评审 R,ok 链);
  needs_changes 一轮 → 下一 R 行追加;熔断 → 尾行 fail + note fuse → waiting_approval;approve 续跑 →
  新 R 行。split → planner do 行 + N 子任务(子任务无 plan)。ask → 行 note。
- **T5** 计划审批:risk=high ops_patrol 建单 → waiting_approval 且 plan 5 行全 pending(GET 可见);
  Decide approve → requeue → 执行后 phases 推进(completed 时 1-4 ok + 5 依 ok/finding);reject →
  task failed。risk=medium patrol 建单 → 不 waiting,直接可执行。
- **T6** project delete(含计划历史任务)→ 任务 + task_plan/phase 保留、task.project_id 置空、pipelines/
  project 行删、磁盘目录仍在;pipeline delete 同理(plan 最小 FK task 无级联)。
- **Tsrv-1** GET /tasks/{id}/plan:patrol 预铺完成 run → 200 形状(phases 状态/evidence);grow 未执行 →
  200 phases 空;非流水线任务 → plan:null;未知 task → 404;无令牌 → 401。信封/鉴权沿用。
- **Tsrv-2** approvals 决策弹窗计划展示 = 前端(Web 验证)承接;server 侧由 Tsrv-1 + T5 覆盖,无新端点。
- 回归:既有 600+ 用例零 body 改优先(grow/机械层为纯新增副作用);`gofmt/vet/test ./...` + `-race`
  (service/server)+ `tsc --noEmit` + `vite build` + `make ui`;二进制冒烟(建 company→project→ops_patrol
  → 控制台加判读端点 → run 201 → **GET /tasks/{id}/plan 见 5 条 pending 计划**)→ 归档 stages/3.md。

## 六、风险与取舍
- **插桩 vs 驱动稳定**:runEngineering 是核心驱动 —— 只加记账副作用,控制流/判断零改动;600+ 用例 +
  race 兜底;若某步插桩失败只应 fail 该 task(不吞驱动错误),不引入新状态路径。
- **机械层行为收紧**:patrol 从「残留静默」变「残留 = 委派越界 → fail」。pre 快照剔除用户既有脏项,
  只追究本次委派新增 → 不误伤;对 live agent 是 fail-closed 收紧(方向一致的委派边界),S5(scripted
  只写报告)→ 不受影响。
- **grow 计划对审批的局限**:engineering 无预铺 → 高风险开发 run 审批仍只见任务描述不见完整计划;
  这是自适应本性的诚实边界(完整计划要等 10.4 合成),不在 10.3 造伪预铺。
- **不做通用解释器防膨胀**:账本不与抽象执行引擎绑定;10.4 合成来时可把合成计划直接落进同一账本表。
- 零 env、零新依赖、删除语义沿用;Web 无暗角(计划观察/审批展示全部落页)。

## 七、登记
- 契约登记 phase10/design/README.md;进度总表挂 10.3 实施契约 ✅(🔶 实施中)。归档阶段收尾写 stages/3.md。
- **定稿门(2026-09-08,AskUserQuestion 三项,全部按推荐取向采纳)**:
  ① 覆盖面 = **双形态共一账本**(patrol upfront 预铺可先审 + engineering grow 执行记账;表/门同一套);
  ② 计划审批 = **增强既有单门**(无新状态机/无 pipeline 字段;waiting_approval 时 Web 审批页随单展示
  预铺计划,批准照计划执行);
  ③ D4 机械优先 = **patrol 判读前 OS 自跑机械卫生预检层**(pre/post porcelain 快照对账,残留 → accept
  fail;通过才进判读)—— 本子阶段首个 OS 直跑真相位,engineering OS 直跑 build/test 留未来形态。
