# Phase 10.4 实施契约 — 高智运行时合成(frontier 合成计划 → 先审后干 → 通用逐阶段驱动)

> 方向见 [declarative-pipelines.md](declarative-pipelines.md)(定稿):§八 10.4 行「高智运行时合成 + 预算 | frontier 对意图合成计划(承 8.4 档位)+ 例行模板化(D3)+ 阶段/全局预算 | 例行多跑可重现;非例行可合成」;§三 中间层「frontier 运行时合成(对 意图+项目可用资源 → 阶段计划 plan={目标|输入|输出契约|分配|验收方式})→ 计划可审(高风险先 approve)→ OS 逐阶段驱动(do→委派;验收→能机械验证 OS 直接执行 / 无机械判据→模型判读)→ 过契约才 advance」。10.3 建账本地基([phase-plan-contract.md](phase-plan-contract.md) 定稿):task_plan/phase + upfront/grow 双形态 + 计划审批增强 + patrol OS 机械预检(D4 首落点)。**本子阶段 = 把方向 §三 的「合成 + 逐阶段驱动」运行时落地为工程 run(develop/bugfix)的一条新分流,10.3 的 grow 默认语义与 600+ 用例零 body 改。**
>
> 契约定稿门四项决策通过(2026-09-08):① **预算不做**——预算/成本治理归外部系统控制,本产品不内建计量/成本模型(零成本原语保持零);② **触发 = pipeline 列 plan_policy**(默认 adaptive=现状 grow,作者 Web 表单显式选 synthesize;零漂移);③ **OS 机械 accept = 只读确定性允许清单**(净残留对账/产出文件存在/报告非空/git diff --check;不执行任意项目代码,go test/build 型 run 留未来);④ **新平行驱动 runSynthesized**(runEngineering/runPatrol/grow 零 body 改,共享仅账本 helper + 既有原语)。按此实施。

## 一、范围与边界

### 1.1 做什么

1. **合成分流(opt-in)**:pipeline 增 `plan_policy` 列(`adaptive`=现状 grow 记账 / `synthesize`=frontier 合成),**默认 adaptive**。认领分流:engineering run + 流水线 plan_policy==synthesize → 新驱动 `runSynthesized`;其余路径(runPatrol / runEngineering)原样。
2. **frontier 对意图合成**(承 8.4 档位):首次认领且计划未合成时,OS 用 **review 槽(frontier)** 调模型 → 得 JSON 阶段计划 → 校验 → 落**同一 task_plan/phase 账本**(materialized 置 upfront,phases 全铺 pending)。
3. **先审后干**:合成先于 needsApproval(requestApproval 时完整计划已落账本 → 审批页 DecideModal 渲染与 patrol 同款);approve → requeue → 复认领时 HasApprovedApproval>0 放行 → 逐阶段执行。
4. **通用逐阶段驱动**:按账本 phase 序执行,do→委派、accept→OS 机械或模型判读、dispose→收尾;过验收才 advance,不过 → 免费返工(≤1 次)→ 再不过 → 阶段 fail → 任务 engFail(evidence 落账本)。
5. **D4 机械优先再落点**:accept 的 OS 机械执行 = **只读/确定性允许清单**(净残留对账、期望产出文件存在、报告非空、git diff --check),**不执行任意项目代码**;go test/build 型 run 仍留后续。
6. **Web 同步**:pipeline 作者面 plan_policy 选择 + runs/计划形态可见 + 未合成空态文案 + 降级提示;无 CLI/API-only 暗角。

### 1.2 不做(边界,留给后续 10.x)

- **预算/成本/失控轴**:**不做**(定稿门①,用户裁)。预算/成本治理归**外部系统**控制,产品不内建计量/单位成本/阶段全局预算/成本可见;零成本原语(全库 grep 为零)保持零。方向 §六 开放点 #4 标注「外部控制」。
- **例行模板化数据化(D3)**:patrol 5 段模板 = 例行模板唯一现例(硬编码 `patrolPlanTemplate`);把「任意工程 run 可登记固定例行模板」做成数据实体/作者面 = 后续子阶段。合成与模板共用「先铺 upfront 账本 + 通用驱动」,模板进来时只加「从模板铺」的源。
- **OS 直跑 go test/build**:方向 §五 不做 CI/CD 张力 + 10.3 冻结边界「工程 build/test 型 run 留未来形态」;本子阶段 OS 机械仍只读(见 §3.5 允许清单)。委派内 agent 自跑测试照旧(不改 8.3 语义)。
- **凭据注入 / 主机 allowlist**:10.2 边界「凭据后移」+ 方向 §六 #2/#5;项目凭据绑定表与 read-only 注入仍不在本子阶段。
- **不做合成计划的编辑/改派 UI**:可审 = approve/reject(既有 Decide 语义);合成不满意 → reject(任务 failed)后改意图重跑。计划人工修编(reorder/改 allocator/补 phase)留后续。
- **非流水线手动任务 / workflow / split 子任务**:仍无 plan(10.3 边界,GET plan null)。

### 1.3 守护(沿既定,逐字保留)

- 产品代码**恒不读 OS_\* env**(9.4 envSeam 反向门控默认关):新合成 seam(OS_SCRIPT_SYNTH 等)仅 `envSeam && engineScripted` 分支可达(TestMain + t.Setenv 开),产品路径不可达。
- 600+ 既有用例**零 body 改**:CreatePipeline 签名不变(用 variadic opts 加 policy,见 §3.2),默认 adaptive 使既有 grow/patrol 断言原样。
- 无新 CLI 写面;Web 是唯一作者面(守 9.4 只读收口)。
- 项目/流水线删除永不碰磁盘;合成 run 的计划随任务历史保留(plan 最小 FK task,无级联,10.3 既定)。
- 一次 commit 收口本子阶段;提交前还原占位 `internal/console/ui/index.html`。

## 二、代码现状核实(2026-09-08 重扫;只读)

- **分流点** `internal/service/execution.go:75` `runClaimed`:engineering 家族 + PipelineID≠nil → `GetPipeline`;kind==ops_patrol → `runPatrol`;否则落 `runEngineering`。GetPipeline 已对每条带 pipeline 的 engineering run 执行 → 三向分流零新增查询。patrol 判 `pl.Kind`(execution.go:79-83)。
- **runEngineering 入口次序** `internal/service/driver.go:34`:needsApproval→requestApproval 置 waiting_approval(释放租约不入队)→ 否则 MarkTaskRunning+eng_start → humanOverride 续跑处理 → planEngineering(direct/split≤8/ask)→ delegateBaseline → 写/测/审 round 机(grow 记账插桩 10.3)。
- **审批机** `internal/service/approval.go`:needsApproval = `HasApprovedApproval>0 → 放行`;否则 risk=high 或公司 enabled approval policy → 需审。requestApproval 建 Approval + RequestApprovalTask + audit request + 通知。DecideApproval approve → ApproveTask(重新入队)→ 复认领 needsApproval 因 approved 放行 → 续跑。→ **合成 run 的「先审后干→批准后续跑」复用同一状态机**,合成只须发生在 needsApproval 判定之前。
- **判读/委派调用链**:engCall(`internal/service/engine.go:48`)= engineScripted ? engScripted :(writer→delegateWriter,judge→engEndpointFor→modelCall)。modelCall(`engine.go:81`)proto=openai 强校验 + OpenToken + provider.NewOpenAI + finish_reason=length→显式截断错误。engEndpointFor(`engine.go:104`)test→test_endpoint_id→reviewer→writer;review→reviewer→writer。**档位解析** `internal/service/tier.go`:review 槽优先 tierFrontier(vendor 规则 frontier=anthropic/openai 判定)、test 槽 tierStandard。scripted seams = OS_SCRIPT_TEST/REVIEW/PATROL/PLAN(envSeam 门内)。
- **planner** `internal/service/planner.go:49` planCall(scripted OS_SCRIPT_PLAN direct/split[:N]/ask[:reason]);planAction=direct|split|ask;engPlanCap=8。plan_driver.go:27 planEngineering:ParentTaskID→no split;children>0→driveChildrenToDone;HasApprovedApproval>0→direct;否则 planCall 三分支(ask→requestApproval+ledgerAppendAsk;split→createPlannedSubtask×N+ledgerAppendSplit+driveChildrenToDone)。
- **账本地基**(10.3):`task_plan`(kind patrol|engineering;materialized upfront|grow)+`task_plan_phase`(seq/kind do|accept|dispose/allocator delegate|os|judge|planner|manual/status pending|running|ok|fail|skipped/evidence/note/started_at/finished_at;UNIQUE(plan_id,seq))。service/plan.go:ensureRunPlan(run-create 时 patrol 预铺 5 行 / engineering 只建 grow 空行)、patrolPlanTemplate、appendPlanPhase(title 去重)、ledgerAppendRound/Split/Ask、loadRunLedger/lgStart/lgFinish(目标态相同 no-op)、GetTaskPlan。patrol.go runPatrol = 现成「upfront 5 相位线性驱动」参考形(runPatrol 真边界翻计划 + 委派前 porcelain pre 快照 + mechanicalPatrolCheck + judge + 处置)。
- **执行原语**:`LeaseAndExecute`/`ExecuteTask`/`ClaimTask`/`RecoverLeasedTasks`(`execution.go`);`runContext(t.TimeoutSec)` 上下文预算;engFail(driver.go:233,按 attempt/max_attempts 重排或终败)。task 五态 pending|running|waiting_approval|completed|failed,**无 cancel/abort 业务状态**;project delete 有活跃 run → ErrProjectBusy(project.go:135)「finish or cancel」但无取消操作(现状诚实边界)。
- **委派**:`Delegator.Delegate(ctx, DelegateSpec{Workspace,Brief,ModelEnv,Timeout,Family})`(`delegate.go:62-75`);claudeDelegator(`delegate.go:97`)= `claude -p spec.Brief --output-format json --permission-mode bypassPermissions --permission-prompts none`,cwd=spec.Workspace。delegateWriter(`delegate.go:293`)= 净 diff 捕获+逐委派 commit+audit eng_delegate;delegateBrief(`delegate.go:428`);gitPorcelainSet(10.3 机械 pre/post 快照底座,`delegate.go`);delegateEnv/delegateTimeout。**os/exec 仅 claude/git/docker 三族**(delegate.go/tool/git.go/tool/shell.go),**无任何 OS 跑 go test/build 的现例**。
- **表**:projects(id/company_id/name/root_path/description/ts;UNIQUE(company_id,name))——**无 secret/凭据列**。pipelines(id/project_id/name/kind bugfix|develop|ops_patrol/description/risk/status/schedule/ts)——**无 plan_policy/template 列**。task 有 project_id/pipeline_id/workspace_path/risk/attempt/max_attempts/timeout_sec/round_no/conflict_count/三端点槽。
- **设置/端点**:settings AppSetting(engine_mode_default/agent_cli_default/…/queue_work/schedule_poll_sec)——无预算字段;CompanySecret(cipher:github_token/feishu_webhook…)。endpoint(model.go:7)= base_url/token_enc/proto/vendor/selected_model/role pool|planner|standby/tier frontier|standard|cheap/status/models_cache——**无单价/成本字段**。迁移最新 0016;sqlc `db/schema.sql`+`db/queries/*.sql`→`internal/storage/query`→repository Store。
- **审计串**:eng_start/resume/complete/fuse/rework/eng_plan/eng_plan_ask/eng_plan_done/eng_delegate/request/decide 等;**无 synthesize 审计**。
- **Web**(`web/src/`):pages = Projects/ProjectDetail/Repos/Tasks/Approvals/Audit/Decisions/Endpoints/Memories/Overview/Settings/SetupWizard;components = PlanBlock(PlanTimeline + mode=decide)/TaskDrawer/DecideModal/StatusTag/modals/common;api = types/endpoints/client。ProjectDetail 已含流水线建(形态/名称/意图/风险)与 runs 行「计划」→TaskDrawer;DecideModal mode=decide 渲染 upfront 计划(10.3)。

## 三、契约设计

### 3.1 实体与迁移 `0017_pipeline_plan_policy`

- `pipelines` **加列** `plan_policy TEXT NOT NULL DEFAULT 'adaptive'`(`adaptive`=现状 grow 记账 / `synthesize`=frontier 合成;service 白名单校验,非法 400)。`db/schema.sql` 同步 + sqlc regen(pipelines 生成 model/query 增该列,既有查询不引用则零语义)。
- 迁移只增列,`.down.sql` 删列;repos/task/plan 表不动。

### 3.2 触发与分流(opt-in,零行为漂移)

- `pipelines.plan_policy` **默认 adaptive** ⇒ 既有 CreatePipeline 调用(600+ 用例 + Web)建出的流水线全 adaptive,认领仍走 grow/patrol ⇒ 断言零动。
- `Service.CreatePipeline` **签名不变**,加 variadic `opts ...PipelineOpt`(如 `PipelinePlanPolicy("synthesize")`),旧调用编译不变;Web 建流水线表单可直传。另提供只读回显:GetPipeline 返回含 plan_policy(GET 流水线 JSON 加字段)。
- `runClaimed`(execution.go:75)三向分流:engineering + PipelineID≠nil → GetPipeline:
  - kind==ops_patrol → `runPatrol`(**模板优先,policy 忽略**,ops_patrol 恒例行模板;policy 存而不用,文档注明);
  - plan_policy==synthesize → `runSynthesized(ctx, workerID, t, pl)`(**新驱动**,见 §3.5);
  - 否则 → `runEngineering`(原样)。
(定稿门②:触发 = pipeline 列 plan_policy,默认 adaptive,Web 建单表单选;无按 run 临时勾选、无默认全开。)

### 3.3 合成步骤(首次认领;先于审批门)

`runSynthesized` 入口:
1. `GetTaskPlanByTask(t.ID)`:无 plan(非流水线/历史)→ 落 runEngineering 兜底;有且 materialized==upfront → 已合成(approve 后续跑)→ 跳 3(直接审批门 → 执行)。
2. materialized==grow(未合成)→ **合成调用**:
   - 端点/模型:review 槽(frontier,承 8.4 档位;engEndpointFor review 语义,缺失则落槽硬失败文案沿用)。
   - 调用:复用 modelCall 类(proto=openai;finish_reason=length→显式截断错误);scripted → `OS_SCRIPT_SYNTH`(envSeam 门内,产品不可达):设置即原文模型文本;空 → 降级。
   - Prompt(synthPrompt):意图(pipeline.description + task 护栏回显)+ 委派边界(整项目 git workspace、delegate-only、无外连、8.3 起点/commit 语义)+ workspace 现状摘要(git 最近 log 首行 + 目录一层树,只读)+ 输出契约要求 JSON。
   - 解析/校验(parseSynth + validateSynth):JSON `{summary, phases:[{seq,kind do|accept|dispose, title, allocator delegate|os|judge|manual, acceptance, output}]}`;校验:非空、kind/allocator 合法、**do→allocator=delegate**(或 planner 预留)、**accept→os|judge 且 acceptance 非空**、dispose→os|judge|manual、至多 ≤12 相位、至少含 1 do + 1 accept;违规 → 降级(见下)。
   - 落账本:置 materialized→upfront + CreatePlanPhase×N 全 pending(pre-materialize;UNIQUE(plan_id,seq) 按序 1..N)+ audit `eng_synth`(summary + N phases)。
   - **降级路径(合成失败/不可解析/模型错)**:audit `eng_synth_fail`(原因)→ 计划保持 grow → **落 runEngineering 走 grow 默认语义**(direct/split/ask 照旧);Web/run 侧可见降级标记(见 §3.6)。risk=high 合成失败 → grow 路径照常 requestApproval(只见任务描述,10.3 诚实边界)。
3. 审批门:needsApproval(已批准放行 / risk=high 或 policy → 需审)。需审 → requestApproval(**此时完整 upfront 计划已落账本**,审批页 GET plan 即见)。approve → ApproveTask 重新入队 → 复认领(1 已 upfront)→ 3 放行 → MarkTaskRunning + eng_start(audit「synthesized plan driver」)→ 执行 §3.5。reject/changes → 任务 failed(Decide 语义既有)。

### 3.4 frontier 合成计划形态(账本 phase 契约)

合成的每个 phase 落 task_plan_phase 同构字段:kind/title/allocator/note(acceptance 判据文本、output 产出物路径/契约放 note 或单独约定首行「acceptance: …」)。acceptance 只存引用/摘要(10.3 边界:证据不整存大产出)。**账本驱动不解释自由文本**——accept 的判读/机械由 allocator 决定,acceptance 是给 OS 机械(允许清单匹配)与判读模型/人看的判据,不是 OS 解释执行的代码。

### 3.5 通用逐阶段驱动 `runSynthesized`

参考形 = runPatrol 的 upfront 相位线性驱动(真边界翻状态 + 委派前后 git 对账);工程 run 的逐相位:

`loadRunLedger`(按 seq 升序)→ 对每个 status==pending 的 phase:
- `lgStart(seq)`(pending→running,记 started_at)。**do/accept 相位对**:do 委派 → accept 验收 → ok 才 advance 下一相位;accept fail → 免费返工 ≤1(同相位 do 重委派,note 带上一判读原因)→ 仍 fail → 该相位 fail → `engFail(ctx,t,"synthesized:phase<seq>",…)` 任务终败(evidence 落账本;attempt/max_attempts 语义沿用 engFail)。
- **do(delegate)**:委托面 = 项目 workspace(8.3 起点 baseline + 逐委派 commit 全沿用);简报 = 任务护栏 + **该相位 title/目标/输出契约/output/acceptance** + 上一相位结论(前相位 accept ok 才进得来)+ 返工时带 hint。产出 commit → lgFinish(do,ok,evidence=commit/diff 摘要 + report rel)。do 失败(委派非零)→ 相位 fail → 任务终败(证据留账本)。
- **accept(os — OS 机械执行,只读/确定性允许清单)**:OS 对 workspace 执行下列**固定允许项**,不跑任意命令:
  1. 净残留对账:本相位 do 委派后 porcelain post − pre(复用 10.3 `gitPorcelainSet` 对账)——**允许项** = 仅本相位声明 output 路径下的新增/改动 + 无其它意外新文件(意外残留 → fail,与 patrol 同文案族);
  2. 期望产出物存在:output 声明的相对路径文件存在(存在性检查,不读内容);
  3. 报告/证据非空;
  4. `git diff --check`(空白错误)干净(只读 git 检查)。
  全过 → ok(evidence 逐条);任一不过 → accept fail(走返工)。**允许清单外的判据(os 无法机械核)** → allocator 应为 judge(合成校验已逼:os 相位 output 必填,acceptance 必须落入上述形状;否则视为无效计划降级)。
- **accept(judge — 模型判读)**:把该相位 do 的报告/diff/产出物存在性喂判读模型(review 槽 frontier;scripted 走 OS_SCRIPT_* 既有判读 seam 族)→ ok/fail + reason → note/evidence。判读返回不可解析 → 相位 fail(显式错误,不静默放行)。
- **dispose(os|judge|manual)**:ok 收尾步。os → git 层收尾(无残留即 ok,evidence);judge → 模型判读是否需收尾动作;manual → 记录 note(人工处置项,不自动动作;任务已完成仍可后续处理)。首版 dispose 保持轻(参考 patrol seq5 dispose:ok→skipped / finding→chain/manual;合成 run 无内置 chain 时 = 同语义 skipped/manual note)。
- 全部相位 ok → `CompleteTask` + audit `eng_complete`(synthesized);任一相位终败 → engFail(如上)。

**注意**:runSynthesized 与 runEngineering **不共用 round 机代码**(新平行驱动;runPatrol/runEngineering/grow 零 body 改)。共享仅限账本 helper(lgStart/lgFinish/porcelain,10.3 已导出同包)与既有 engFail/审批/委派原语。

### 3.6 Web 同步(后端能力逐一面板化)

| 能力 | Web 承接 |
|---|---|
| plan_policy 作者面 | `pages/ProjectDetail.tsx` 建流水线 Modal 增「计划策略」下拉(自适应=grow 执行记账 / 合成=frontier 先行合成,Alert 说明合成首次认领触发、失败降级);`api/types.ts` Pipeline 加 plan_policy、endpoints 建单传 opts |
| run/任务形态可见 | ProjectDetail runs 行与 TaskDrawer「计划」段:materialized=upfront+synthesize → 「合成计划 · 先审后干/执行中」徽标;materialized=grow+policy synthesize 未认领 → 「认领后合成」空态文案(区别于 grow 记账的「执行中生成」) |
| 先审后干展示 | 复用 DecideModal mode=decide 渲染 upfront 计划(engineering upfront 现也走同款;10.3 的 grow→「执行中生成」文案分支按 materialized 不变) |
| 降级提示 | runs/审计处可见合成失败降级(audit eng_synth_fail;任务详情提示「合成失败,已按自适应执行」) |
| 合规 | 无 CLI 写面新增;无 env 泄漏 |

### 3.7 端点

- 只读 GET `/pipelines/{id}` 响应加 plan_policy;无新端点;建单 POST `/pipelines/{id}/run` 语义不变(策略是流水线属性,非 run 参数)。
- plan 读/审批决策沿用 10.3(Tsrv-1 / T5 覆盖读形状与翻转)。

## 四、文件落地清单

| 文件 | 动作 | 内容 |
|---|---|---|
| `internal/storage/migrations/0017_pipeline_plan_policy.up/down.sql`(新) | 增列 | `ALTER TABLE pipelines ADD COLUMN plan_policy TEXT NOT NULL DEFAULT 'adaptive';`/down 删列 |
| `db/schema.sql` + `db/queries/pipeline.sql` + sqlc regen | 改 | pipelines 表/query 模型带 plan_policy;既有插入不写列(默认 adaptive);新增 `SetPipelinePlanPolicy` query(只更该列) |
| `internal/pipeline/model.go` + repository/pipeline.go | 改 | 模型加 PlanPolicy;Store.GetPipeline 带回;SetPipelinePlanPolicy;常量 PlanPolicyAdaptive/Synthesize |
| `internal/service/pipeline.go` | 改 | CreatePipeline 加 `opts ...PipelineOpt`(兼容旧签名);变体当 opts 给 policy 时建后 Set;GetPipeline 透传;policy 白名单校验 |
| `internal/service/execution.go` | 改 | runClaimed 三向分流(synthesize → runSynthesized) |
| `internal/service/synthesize.go`(新) | 新 | `runSynthesized`、synthPrompt、OS_SCRIPT_SYNTH 解析、parseSynth/validateSynth、synth 落账本/降级/audit eng_synth(_fail)、逐相位驱动、osMechanicalAccept(允许清单) |
| `internal/service/engine.go` | 改(小) | 若按新 role 取 review 槽复用判读调用则加最小分支;否则复用 engCall(review) |
| `internal/service/plan.go` | 改 | 加「置 materialized→upfront」与逐相位 helper 若缺(10.3 lgStart/lgFinish 已有,仅补 materialized 翻转 store 步) |
| `internal/service/delegate.go` | 改(小) | 复用 gitPorcelainSet/commit 原语;可能加 per-phase 简报构造(改 delegateBrief 不加,新 synth 内构) |
| server(`api.go` + handler) | 改 | GET pipeline 响应含 plan_policy(读);无新端点 |
| `internal/service/synthesize_test.go`(新)+ 相关 | 新 | SY1–SY6(scripted,§五) |
| `internal/server/plan_api_test.go` 或新 `synth_api_test.go` | 新 | Vsrv-1(§五) |
| Web(`types.ts`/`endpoints.ts`/`ProjectDetail.tsx`/`TaskDrawer.tsx`/`utils/dicts.ts` 等) | 改 | §3.6 |
| 归档 `docs/phase10/stages/4.md` + 进度总表/design README | 改 | 收口凭证 |

## 五、用例清单

**SY\*(service,scripted;t.Setenv OS_ENGINE_MODE=scripted + OS_SCRIPT_SYNTH)**
- **SY1 合成先审后干主链**:synthesize pipeline(bugfix,risk=high)run-create → plan 行 materialized=grow(未合成诚实空态)→ claim1(OS_SCRIPT_SYNTH=合法 3 相位 JSON)→ 账本 upfront + 3 条 pending + GET plan 可见 → 未消费任何 writer/judge → waiting_approval;Decide approve → requeue → claim2 → 逐相位执行(do→commit、accept(os 或 judge 判读)→ok、dispose→skipped/note)→ 全 ok → task completed;断言相位 status/evidence/started/finished/commit 逐相位 + 审计 eng_synth/eng_complete。
- **SY2 合成失败降级**:OS_SCRIPT_SYNTH 置非法/空 → audit eng_synth_fail → 计划保持 grow → 落 runEngineering(OS_SCRIPT_PLAN direct)→ 走 grow round 机完成;断言 grow 记账照旧(T4 语义复现)。
- **SY3 OS 机械 accept 允许清单**:do 相位委派产出输出文件 → accept(os)全过 ok;do 留意外残留(stray 文件越出 output)→ accept fail → 返工 1 次(仍残留)→ 相位 fail → 任务 engFail(evidence);output 文件缺失分支;pre/post 用户既有脏项不误伤(复用 T3 思路)。
- **SY4 judge accept 返工上限**:OS_SCRIPT_* 判读两次 fail → 相位 fail → engFail;一次 ok 则放行(≤1 返工语义)。
- **SY5 默认 adaptive 零漂移**:CreatePipeline 默认 plan_policy=adaptive;同描述 develop run claim → 走 runEngineering,grow 计划(T4 断言 subset)零动;ops_patrol 建 synthesize policy 亦 runPatrol(模板优先)。
- **SY6 计划校验**:合成 JSON 缺 accept / allocator=os 无 output / >12 相位 / kind 非法 → 降级(不落半成品账本)。
- **Vsrv-1(server,真 svc + 真 http)**:HTTP 建 synthesize pipeline → GET pipeline 回显 plan_policy;POST run → 200;GET plan = materialized=grow(未认领不合成,诚实);非法 plan_policy 建单 → 400;无令牌 → 401。(合成执行本身 engine live 不触网,语义由 SY1/SY2 service 覆盖;server 只验作者面与读形状。)
- **Web**:tsc --noEmit + vite build(make ui 冒烟);ProjectDetail 建流水线 policy 下拉 + runs 徽标空态。
- **回归**:600+ 既有用例零 body 改全绿(默认 adaptive 保证);gofmt/go vet/go build/`-race`;二进制冒烟(建 synthesize pipeline + run + plan grow + GET / SPA + 401)。

## 六、风险取舍(定稿门通过,2026-09-08;按此实施)

- **D1 首交付边界 → 本契约 = 合成运行时;预算不做**:预算/成本治理归外部系统,产品不内建计量(见 §1.2);例行模板化数据化 / 凭据注入 / 主机 allowlist 亦不在此契约(§1.2),每轴独立可验收,后续按进度总表续接。
- **D2 触发形态 → pipeline 列 plan_policy**:默认 `adaptive`(零漂移,600+ 用例默认断言不动);作者在 Web 流水线表单显式选 `synthesize`;无按 run 临时勾选、无默认全开。
- **D3 OS 机械 accept 范围 → 只读确定性允许清单**:净残留对账 / 期望产出文件存在 / 报告非空 / `git diff --check`;**不执行任意项目代码**;os 相位必带 output、acceptance 必落该形状,否则判无效降级;go test/build 型 run 留未来形态(方向 §五 不做 CI/CD + 10.3 冻结边界)。
- **D4 驱动形态 → 新平行驱动 runSynthesized**:runEngineering/runPatrol/grow 零 body 改;共享仅账本 helper(lgStart/lgFinish/porcelain)与既有 engFail/审批/委派原语。

## 七、登记

已登记 `docs/phase10/design/README.md` + `docs/进度总表.md`(10.4 契约定稿,待实施),归档 `stages/4.md`(实施完成后)。
