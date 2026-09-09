# Phase 10.6 实施契约 — OS 机械扩 build·test(合成相位显式 verify;D4 真裁判)

> 重开 10.4 契约定稿门③(OS 机械 accept = 只读确定性允许清单,go test/build 型 run 留未来)。方向 [declarative-pipelines.md](declarative-pipelines.md)(定稿,**正文零改**):§六 #1 验证可信度「真正裁判是 go test/build 跑一遍;报告内嵌原始证据」;§三 中间层「能机械验证 OS 直接执行」。本子阶段把 10.4 只读允许清单扩一个**显式 verify 通道**:OS 在合成计划里被显式声明的 os-accept 相位真跑**封闭命令集**(go build ./… / go test ./…)作为机械验收判据。
>
> 契约定稿门四项决策通过(2026-09-09,ExitPlanMode 计划):① **触发面 = 仅合成 + 相位显式声明** —— OS 执行项目代码的唯一路径是 synthesize run 的 upfront 计划里一个显式声明 verify 的 os-accept 相位(封闭标记行);默认 adaptive(grow)不真跑(test 仍模型判读 diff 文本),OS 真 build/test 归下一子阶段;② **命令封闭集双键** `go-build`/`go-test`(固定 argv、无 shell;go-test 隐含全量编译),词表外 verify 值 → 计划无效降级;③ **网络策略零改**:继承宿主 go env 与 module cache,缺依赖/私有模块拉不到 → 如实失败并把输出首行回显进 evidence;④ **残留回收**:verify 跑后仅删除 pre→post 期间新出现的 untracked(测试临时产物),verify 改动 tracked → verify 判 fail。按此实施。

## 一、范围与边界

### 1.1 做什么

1. **合成计划可选 verify 声明**:`synthPhaseSpec` 增可选 JSON 字段 `verify`(零迁移,无新列/表),值 ∈ 封闭集 `{go-build, go-test}`;落 phase.note 标记行 `verify: <key>`(`synthVerifyPrefix`,与 acceptance/output 同构;计划仍不解释自由文本)。
2. **校验**:verify 只允许出现在 **allocator=os 的 accept**(judge accept / do / dispose 上出现 → 计划无效降级);非空须在封闭词表内(词表外 → 降级,不落半成品 upfront)。
3. **OS 机械 verify(验收真裁判)**:`osMechanicalAccept` 只读三检查全过后,若该 accept 相位带 verify → 跑一次机械 verify:前置 go.mod 门(workspace 根须有 go.mod,否则立即 fail 不调 runner)→ porcelain pre 快照 → 封闭命令执行(s.runner,verifyMax 预算)→ post−pre 对账(Q4)→ exit 0 → PASS / 非零 → FAIL(输出首行回显)→ 超时 → TIMEOUT。结果折回既有 (bool, reason) → evidence(≤120)与返工 hint / 终败 note,driver 控制流**零改动**。
4. **执行 seam**:`Service` 增 unexported `runner cmdRunner` 字段(nil → 真实现 fallback,继承宿主 env);测试同包注入 fake。只被带 verify 的 os-accept 触达 → 无 verify 标记的既有相位(全部 SY*)永不 exec go → **600+ 用例零 body 改、真实 go 只在显式声明时被 OS 执行**。
5. **合成提示词**:生成计划的 prompt 注明 os-accept 相位**可选** verify(语义 + 需 go.mod),不强制、默认不加。

### 1.2 不做(边界)

- **grow/自适应默认不真跑**:方向 §三 默认语义不变 —— 非 synthesize / 无 verify 标记 → OS 恒不执行项目代码,test 仍模型判读 diff 文本;OS 真 build/test 全面化(自适应路径也要真实跑测试)归下一子阶段。
- **命令集不扩**:仅 `go-build` = `go build ./…`、`go-test` = `go test ./…` 双键;python/npm/rust 等留未来(契约登记封闭集与语义即接口)。
- **输出全量不落库**:verify 原始 stdout/stderr 只用于提取首行进 evidence(≤120 截);全量输出留未来形态(evidence/note 有界,10.3 边界)。
- **基线红如实失败**:do 相位引入前项目本就红(go build/test 失败非本相位造成)→ verify 如实判 fail → 返工 ≤1 → 仍 fail → 任务 engFail。首版**不捕获 verify 基线**、不自动放行;诚实边界(方向 §六 #1 真裁判)。
- **不改网络策略**:无 GOPROXY/GOFLAGS/私有凭据注入(Q3);私有模块拉不到 = 命令失败如实回显。
- **不做 CI/CD**:本子阶段是 **run 内相位级验收判据**(单仓库、无发布/集成语义),非方向 §五「不做 CI/CD」的 CI 引入;边界解释以契约为准。
- **零 Web / 零 API / 零 CLI diff**:verify 经既有 plan evidence/note 透传(Web PlanBlock 已逐字渲染);无新端点、无新页面。

### 1.3 守护(沿既定,逐字保留)

- 产品代码**恒不读 OS_\* env**(9.4 反向门控默认关):verify 不走任何 env seam,命令与预算全代码内常量。
- 600+ 既有用例**零 body 改**:`synthPhaseSpec.Verify` 可选(旧 fixture 空 = 无 verify,行为逐字不变);osMechanicalAccept 无 verify 分支不改;runner 无 verify 相位永不触。
- 无迁移、无 sqlc、无 go.mod 变更、零 Web diff。
- 一次 commit 收口本子阶段;提交前还原占位 `internal/console/ui/index.html`。

## 二、代码现状核实(2026-09-09;只读)

- **osMechanicalAccept** `synthesize.go:698` 已是 `*Service` 方法,返回 (bool, string detail):产出 os.Stat 存在(:700)→ porcelain − output 白名单净残留对账(:707,越界 → residue fail)→ `git diff --check` 未提交 + `HEAD^..HEAD` 相位空白双查(:726/:731,浅历史跳过)。全过 → (true, "os mechanical: …residue=0; diff --check clean")。**driver 控制流零改动**成立:runSynthUnit 把 accept 返回值直接转 evidence(ok → `truncate(reason,120)` :498;fail → reason 首行进返工 hint + 终败 note :506-515),verify 判定只需折进 osMechanicalAccept 的返回。
- **相位字段在 note 标记行**:`synthAcceptPrefix`/`synthOutputPrefix`(:37-38),`synthPhaseNote`(:160)组行、`phaseAcceptance`(:172)/`phaseOutputs`(:182)/`outputsFromNote`(:390)读回;do 相位经 `mergeDoOutputs`(:368)并入相邻 accept 的 output(只并 output 行,verify 行不并 → do 侧不泄漏)。`synthPhaseSpec`(:55)JSON {kind,title,allocator,acceptance,output};`validateSynthPlan`(:85)。
- **Service 注入范式**:unexported `delegator`/`prPub`(service.go:18/22;prPub nil → 真实现 fallback pr.go:222)。机械路径现无 exec seam、**无 exec("go") 先例**;git 走 `gitDirCmd`(delegate.go:161,stdout+stderr 合并、caller ctx、env 继承)。env 全继承 os.Environ()。
- **执行原语**:`runSynthAccept`(:673)AllocatorOS → `s.osMechanicalAccept(ctx, ws, acc)`(:676,现传外层 ctx);`runSynthUnit`(:476)返工 ≤1(`synthMaxRework`,rework 上限后 engFailTerminal 终败不复活)。do 委派后工作树净态 = HEAD 新 OS commit、porcelain 空(commitDelegation)。
- 常量区(:31-39)加 `synthVerifyPrefix`;相位/证据写 API = service/plan.go `lgFinish`(唯一 evidence 写口)。

## 三、契约设计

### 3.1 verify 声明(零迁移)

- `synthPhaseSpec` 加 `Verify string \`json:"verify"\``(**可选**;空 = 无 verify,行为不变)。
- 落账本:`synthPhaseNote` 对非空 Verify 追加一行 `verify: <key>`(新 const `synthVerifyPrefix = "verify: "`);读回 `phaseVerify(ph)`(仿 phaseAcceptance,取首个 verify 行 trim)。materializeSynthPlan 不改(do 只并 output,verify 留在其 accept 相位 note)。

### 3.2 校验(validateSynthPlan,降级不改落账本路径)

- verify 值须单行(与 acceptance/output 同族多行检查)。
- **位置约束**:verify 非空 仅当相位 kind==accept 且 allocator==os(do / dispose / judge-accept 上出现 → 计划无效降级)。
- **词表**:非空值 ∈ `{go-build, go-test}`(`verifyIsKey`;词表外 → 降级)。acceptance/output 既有必填约束不动。

### 3.3 OS 机械 verify(新 `internal/service/verify.go` + synthesize.go 小改)

- `runSynthAccept` os 分支(:676)改传 **runCtx**(任务时间预算)进 osMechanicalAccept(签名不变)。
- `osMechanicalAccept` 只读三检查**全过后**,若 `phaseVerify(acc)` 非空 → `runMechanicalVerify(ctx, ws, key)` 覆盖返回值:
  1. **git 门**:workspace 非 git → (false, `verify <key>: workspace is not a git repo`)(do 相位已 commit,理论不可达;防御);
  2. **go.mod 门**:`<ws>/go.mod` 不存在 → (false, `verify <key>: no go.mod in workspace (verify expects a Go module)`),**runner 不调用**(do 返工可把模块建出来 → 重 verify 过);
  3. **pre 快照**:`gitPorcelainSet`(do 已 commit,通常净);
  4. **执行**:`s.runner.Run`(nil → realCmdRunner)跑固定 argv(cwd=ws;env 继承 os.Environ() 不改网络策略;ctx = `context.WithTimeout(ctx, verifyMax)`,verifyMax=5m 镜像既有 push 30s / pull 20s / delegate 20m 封顶惯例);stdout+stderr 合并;
  5. **对账(Q4)**:post porcelain − pre —— 新增 `??` untracked → `os.RemoveAll`(仅这批新出现路径;绝不碰 pre 已有 / 已提交 / tracked;删失败 → fail);**tracked 改动**(post 里有、pre 无的非 `??` 行)→ (false, `verify <key> dirtied tracked file: <path>`) —— 下相位 delegateBaseline 净起点保证;
  6. **判定/证据**(detail 单行;evidence 经 truncate(…,120) 截,首段即有用信息):
     - exit 0 → `verify <key>: PASS (exit 0, <s>)`,有回收追加 `; verify residue cleaned=N`;
     - 非零/命令错 → `verify <key>: FAIL: <输出首非空行 或 err 首行>`(go-test 首个 --- FAIL/编译错、go-build 首行编译错经首行透出);全量输出不落库;
     - ctx 超时 → `verify <key>: TIMEOUT (<s>)` → fail。
- PASS detail 前置(保 120 截断不丢 verify 判定),原只读三检查 evidence 并入其后:返回值 (true, `verify…: PASS…; os mechanical: …`) / (false, verify FAIL reason)。
- verify 结果折回既有 runSynthUnit 返工机:FAIL → reason 进 hint → do 重委派 → 重 verify ≤1 → 仍 FAIL → do/accept 相位 fail → engFail(evidence 落账本)任务**不复活**。verify PASS 不改变 runSynthPhases/finishRun/complete 语义。

### 3.4 runner seam(仿 prPub 范式)

```go
type cmdRunner interface { Run(ctx context.Context, dir, name string, args ...string) (string, error) }
type realCmdRunner struct{}
func (realCmdRunner) Run(...) // exec.CommandContext, cmd.Dir=dir, stdout+stderr 合并, env 继承(不改网络)
```
- `Service` 增 unexported `runner cmdRunner`(service.go,注释范式同 delegator/prPub);调用点 nil → realCmdRunner fallback;测试同包直赋 fake。
- 封闭命令表 `verifyCommands`(map 固定 argv;现双键 go 一族)+ `verifyCommandKeys()` 供校验。新增键 = 契约登记 + 词表同步,不在此子阶段。

### 3.5 合成提示词

- synthPrompt 规则区注明:os-accept 相位**可选** `"verify": "go-build" | "go-test"` 让 OS 跑真实命令作额外机械验收(go-test = 全量编译 + 测试;require go.mod present at accept time;default omit)。不改变其余指令。

## 四、文件落地清单

| 文件 | 动作 | 内容 |
|---|---|---|
| `internal/service/verify.go`(新) | 新 | cmdRunner + realCmdRunner;verifyCommands 封闭表 + `verifyCommandKeys`/`verifyIsKey`;`runMechanicalVerify`(git/go.mod 门 → pre 快照 → 执行 → 对账回收 → 判定/证据);porcelain 行归类(untracked 前缀 `?? ` / tracked 改动) |
| `internal/service/synthesize.go` | 改 | const `synthVerifyPrefix`;`synthPhaseSpec.Verify`(JSON 可选);`phaseVerify` 读回;`synthPhaseNote` 落 verify 行;`validateSynthPlan` verify 词表 + 位置 + 单行约束;`osMechanicalAccept` 只读三检查后接 verify(runCtx);`runSynthAccept` os 分支传 runCtx;`synthPrompt` 说明可选 verify |
| `internal/service/service.go` | 改 | Service 增 `runner cmdRunner` 字段(注释范式) |
| `internal/service/verify_test.go`(新) | 新 | V1–V7(§五) |
| `internal/service/synthesize_test.go` | 改(按需) | SY 现有断言零动;必要时补 verify 夹具常量 |
| `docs/phase10/design/os-mechanical-verify.md`(新)+ `README.md`/`进度总表.md`/`stages/7.md` | 登记/归档 | 收口凭证 |

无迁移、无 sqlc、无 go.mod 变更、**零 Web diff**、无新 CLI/API 端点。

## 五、用例清单

**V\*(service,scripted 公司 + 真 git 项目根[go.mod 已提交] + 注入 fake runner;离线确定性;scripted do 产出 fix.go + commit → accept os 触发 verify)**
- **V1 verify PASS 主链**:OS_SCRIPT_SYNTH = do + accept(os,`verify: go-test`)+ dispose(manual);fake runner exit 0(`ok pkg 1.2s`)→ completed `synth: 3 phases executed`;acc evidence 含 `verify go-test: PASS`;runner 恰调 1 次(argv = go test ./…,dir = ws);逐相位 ok + 审计闭环。
- **V2 verify FAIL → 返工 → 仍 FAIL → 不复活**:fake runner 恒非零(输出带 `--- FAIL: TestX`)→ 返工 1 仍 FAIL → 任务 **failed**;acc evidence 首段含 `verify go-test: FAIL` + 首行;runner 调 2 次;do/accept 相位 fail 落账本。
- **V3 verify FAIL → 返工 → PASS**:fake runner fail-once(首跑非零、二跑 exit 0)→ completed;runner 调 2 次;返工后放行语义。
- **V4 词表外 verify**(`verify: python-pytest`)→ validate 错 → audit eng_synth_fail → 降级 runEngineering(OS_SCRIPT_PLAN direct)→ completed;runner 不触;materialized 保持 grow。
- **V5 位置约束**:verify 出现在 judge-accept / do / dispose → 计划无效降级(同上断言,不落半成品)。
- **V6 残留对账双分支**:fake runner exit 0 且写一个 untracked 临时文件 → accept ok 后该文件被回收、工作树归净、evidence `verify residue cleaned=1`;另一分支 fake runner 改动已提交文件 → verify fail `dirtied tracked file`。
- **V7 无 go.mod 门**:项目根无 go.mod + verify go-test → accept fail、reason 含 `no go.mod`、runner **不调用**(调 0 次);go-build/go-test 双键经 V1/V7 各覆盖。
- **回归**:既有 600+ 用例零 body 改全绿(grow/patrol/无 verify 合成逐字不动);gofmt/vet/build + `-race`(service);**零 Web diff → 不跑 tsc/vite**。
- 二进制冒烟:合成 verify 需 frontier 合成 → 离线不可达端到端;跑既有回归冒烟证明无回归,verify live 端到端(真 frontier + Go 项目 + 真 go test 修复后过门)如实标注待用户边界(同各阶段惯例)。

## 六、风险取舍(定稿门通过,2026-09-09;按此实施)

- **D1 触发面 → 仅合成 + 相位显式声明**:OS 执行项目代码 = upfront 计划里显式 declare verify 的 os-accept 的唯一通道;adaptive(grow)不真跑(默认语义零漂移)。命令恒封闭、可审(计划先审后干在 verify 之前)。
- **D2 命令集 → go-build/go-test 双键**:固定 argv、无 shell、无模型注入 argv;词表外 → 计划无效降级(校验即接口,计划生成端闭口)。
- **D3 环境 → 继承宿主 go env**:不改网络策略(无凭据注入 / GOPROXY 改写);缺依赖如实失败回显。模块缓存命中即离线可跑,失败 = 诚实信号不是故障。
- **D4 残留 → 仅回收新增 untracked,改动 tracked → fail**:verify 破坏只读性(clone 前工作树净起点;下相位 delegateBaseline 不吞脏)时才 fail;测试临时产物自动回收不残留。

## 七、登记

已登记 `docs/phase10/design/README.md` + `docs/进度总表.md`(10.6 取号冻结;declarative-pipelines 方向正文零改),归档 `stages/7.md`(实施完成后)。
