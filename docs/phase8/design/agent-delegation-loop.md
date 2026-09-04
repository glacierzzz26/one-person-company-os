# Phase 8.2 — 执行委派首个闭环(writer)+ 判读回网关(实施契约)

> Phase 8 子阶段实施契约,承接方向设计 [model-runtime.md](model-runtime.md) §六 8.2 行与 §四 D2/D4(修订 B:执行=委派集成 agent CLI)+ D1(修订 B:OS 回合只走 text Chat 承担判读)。
> 用户方向纠正(修订 B 定稿门 2026-09-04,commit `34f26f4`):「我感觉不需要切原生的工具,用 claude 这种集成的就好了」。
> 本文件是 8.2 的执行蓝本:**代码现状全部经仓库核实,类型/签名照抄真实代码,不凭想象**;范围分叉(是否含「判读生产路径切网关」)已于 2026-09-04 定稿门由用户拍板 = **含**。
> 定稿冻结,按它写代码;落地后归档 `stages/2.md` 并在进度总表登记。

## 一、范围与边界

**做(8.2 交付物,两条生产接线 + 前置/审计):**

1. **writer live = 委派集成 agent CLI**(修订 B 核心):需真动手的 writer 阶段(live)不再文本产出,改为 OS 委派 **claude Code**(`claude` 二进制 agent 模式)在**任务 git workspace** 自主读/改/跑;委派结束 OS 用 **git 捕获真实 diff** 作该 writer 阶段产出。逐次委派留审计。
2. **判读生产路径切网关**:service `modelCall` 由 `ClaudeCLI.Generate`(claude -p 文本)改为 **OpenAI 兼容网关 `Chatter.Chat` 单轮文本**(不带 tools)—— planner/triage/test/review 判读统一回网关(8.1 契约 §七 预留接线点即此;修订 B「test 判读回网关」的执行口径)。`scripted` 服务层零改动。
3. **git 委派前置**:writer 委派要求任务工作区为 **git 仓库且认领起点 clean**;缺失 → 清晰报错(指引用 repo 绑定工程任务),不静默。
4. **新用例**:委派骨架 + git 捕获 + 审计 actor + 判读命中网关 + scripted 回归(新增最小驱动闭环回归,补齐 6.2 起 driver 无单测的空缺)。

**不做(留给 8.3–8.4,防越界):**

- **不改结构化收敛/正则**:`parseReviewVerdict`/`parseTestPass`/`extractDiff` 判读解析、`verdict` 信号、writer 返工反馈均不动(判读结构化属 8.3,替换正则与「再喂判读失败原因给 writer」同批落)。
- **不做多任务同库仲裁/提交语义**:planner split ≤8 子任务共享同一 repo workspace 的工作树隔离、agent 自行 `git commit` 的处置、任务完成是否自动提交/建 PR —— 属 8.3(委派边界)。8.2 场景 = **单条原子工程任务(direct / direct_work umbrella 单 issue)绑定自己的 git workspace**,认领起点 clean。
- **不做 agent CLI 工具族选型**:claude→codex 注册/选型属 8.3。8.2 只固定 `claude`。
- **不做 tier 默认落档**:`endpoint.tier` + 角色默认映射属 8.4;8.2 判读用任务显式端点(建单 `--reviewer-endpoint`/planner 端点),不引新字段、不迁移。
- **不加迁移 / 不加 sqlc / 不改 go.mod / 不改 endpoint 表**。
- **writer 端点的「cheap 配对」不作 8.2 强制**:`writer_endpoint_id` 可选(仅作 claude 委派 env);没设 = claude 自带鉴权/模型。OpenAI 方言网关不能作 claude Code 后端(见 §七 风险),故 8.2 不把判读端点与委派端点强行绑定。

## 二、代码现状核实(8.2 的立足点,全部属实)

| # | 事实 | 位置 |
|---|---|---|
| 1 | 工程任务 = `ToolName=="engineering"`;回合机 `runEngineering`(写→测→审;test 失败免费返工;review needs_changes→conflict+1;conflict≥3→waiting_approval→approve 续跑归零)→ 每阶段 `runEngPhase` 建 execution 并调 `engCall` | `internal/service/driver.go:34-191` |
| 2 | `engCall` 分界:`OS_ENGINE_MODE=scripted` → `engScripted`(确定性文本,writer 返回假 diff);live → `engEndpointFor`(test/review 回退 writer 端点)→ 活动端点校验 → `modelCall` | `internal/service/engine.go:43-63, 82-93, 96-133` |
| 3 | `modelCall` = `endpoint.OpenToken` 解密 → `provider.NewClaudeEndpoint(...).Generate`(唯一真调用点;**planner 拆解与 intake triage 共用**) | `internal/service/engine.go:67-78` |
| 4 | 判读解析:`parseReviewVerdict`/`parseTestPass`/`extractDiff` 文本正则;review/test prompt 为单行裁决(TEST OK/FAIL、VERDICT approve/needs_changes) | `internal/service/engine.go:148-239`(prompt 在 driver.go:210-239) |
| 5 | 任务已带 `WorkspacePath`;建单 `TaskParams.Workspace` 落库。通道 B:repo 登记路径(intake `createIssueTask` 用 `r.WorkspacePath` = GitHub checkout,天然 git);planner 子任务继承父工作区;通道 A workflow 节点默认 `os.MkdirTemp`(非 git) | `internal/task/model.go:23`、`internal/service/task.go:44-67`、`intake.go:183-210`、`plan_driver.go:105-120`、`workflow.go:96-104` |
| 6 | 8.1 provider 已就位:`Chatter`/`ChatRequest`/`ChatResponse`(Content/FinishReason)、`NewOpenAI(baseURL,key,model)`、消息构造 `Sys/User/...`、确定性 `ChatStub`;`ClaudeCLI` 已标 Deprecated(0-7 保留),头注写「claude Code agent 委派属 8.5(独立路径)」 —— 该注随修订 B 过时 | `internal/provider/openai.go:28`、`messages.go:70-89`、`chatstub.go`、`claudecli.go:13-16` |
| 7 | `endpoint.Endpoint` 含 `BaseURL/Proto(auto\|anthropic\|openai)/Vendor/SelectedModel/TokenEnc/Role(pool\|planner\|standby)/Status(active\|disabled)`;token AES-GCM `enc:v1:` 明文不出库 | `internal/endpoint/model.go`、`seal.go` |
| 8 | 审计 `audit(ctx, entityType, entityID, action, actor, detail)`;工程驱动内动作 actor=`taskActor(t)`(`agent:<id>`/`agent:none`);execution 行由 `runEngPhase` 建,完成时 `FinishExecution(completed, "eng_role round=..\n"+out)` | `internal/service/service.go:30`、`execution.go:170`、`driver.go:164-191` |
| 9 | `runContext(t.TimeoutSec)` 已对整轮加墙钟;git/file 工具沿用 `git -C <ws>` 本地白名单、`resolveWorkspacePath` 边界(委派 helper 只发固定命令,不经用户串) | `internal/service/execution.go:181`、`internal/tool/git.go`、`tool/file.go:108` |
| 10 | 迁移最大 0009,无迁移 runner 外文件;`go.mod` 现状零新依赖压力(8.2 只用 stdlib `os/exec`+`net/http` 既有) | `internal/storage/migrations/` |
| 11 | 端到端测试 harness:service 级用例 = `storage.Open(tmp db)` → `repository.NewStore(db)` → `service.New(st)`(server 包 api_test 已用);driver/engine 目前**无** service 级单测(6.2 冒烟为 CLI/离线,补在 8.2 用例) | `internal/server/api_test.go`、`internal/service/` 仅 digest/planner 纯函数测试 |

**端点接入约定(网关判读世界)**:给判读用的端点须 `proto=openai`(自建网关),base_url/token/selected_model 齐全、active;anthropic 原生 claude 端点不再被 `modelCall` 消费(live 报错指引换网关端点),仅 0-7 遗留 `Generate` 路径与委派 env 底座可用。

## 三、契约设计

### 3.1 执行分界(修订 B 落地形态)

| 角色 | live 路径 | scripted(`OS_ENGINE_MODE=scripted`) |
|---|---|---|
| writer(执行) | **委派集成 agent CLI**(claude Code)在任务 git workspace 真干 → git 捕获真实 diff | `engScripted` 假 diff(确定性,不变) |
| planner / test / review / intake triage(判读) | 网关 `Chatter.Chat` **单轮文本**(不带 tools),proto=openai | `engScripted*` 确定性(不变) |

- `engCall` 内 scripted 前置判断在第一位,scripted 下 writer/test/review 永不落委派/网关(离线可复现、CI 不依赖真实 claude/网关)。
- live writer 不再要求判读端点;live test/review/planner 端点须为 openai 网关端点。

### 3.2 判读生产路径切网关(modelCall 重指向)

签名与调用点不变(planner/triage 无需改),只换内部实现 + proto 门:

```go
// modelCall 对给定端点执行一次网关判读(修订 B:OS 回合判读只走 text Chat)。
// 端点必须 proto=openai(自建网关);claude CLI legacy(ClaudeCLI)不再被 modelCall 消费。
func (s *Service) modelCall(ctx context.Context, e endpoint.Endpoint, prompt string) (string, error) {
    if !strings.EqualFold(e.Proto, "openai") {
        return "", fmt.Errorf("judging requires proto=openai gateway endpoint %q (proto=%s); add an OpenAI-compatible gateway endpoint", e.Name, e.Proto)
    }
    token, err := endpoint.OpenToken(e.TokenEnc)
    if err != nil {
        return "", err
    }
    c := provider.NewOpenAI(e.BaseURL, token, e.SelectedModel)
    resp, err := c.Chat(ctx, provider.ChatRequest{
        Messages: []provider.Message{provider.User(prompt)},
    })
    if err != nil {
        return "", err
    }
    if resp.FinishReason == "length" {
        return "", fmt.Errorf("judge output truncated (finish_reason=length)")
    }
    return resp.Content, nil
}
```

规则:
- **单轮、不带 tools**:判读 = 文本信号(verdict/test 行/plan JSON/拆解),工具执行只发生在 writer 委派(agent 侧自洽),OS 不再自研逐 call 编排。
- **proto 门**:非 openai → 报错含端点名与 proto,指引建网关端点。`auto` 也拒(显式声明的网关才消费,避免误接 anthropic 端点打 `{base}/v1/chat/completions`)。
- **length 截断**:显式错误 → `engFail` 走 requeue/fail,不静默把截断文本当信号。
- **空 key**:不发鉴权头(OpenAI 客户端已有语义),本地网关可用。
- `ClaudeCLI` 保持 Deprecated、代码不动;其 `Generate` 消费方(0-7 provider 配置路径)不受影响。头注「委派属 8.5」改口为「委派底座见 service `claudeDelegator`(8.2),走 claude 二进制 agent 模式」—— 仅注释。

### 3.3 writer 委派闭环

#### 3.3.1 Delegator 接口(测试注入缝,真实 = claudeDelegator)

```go
// Delegator 把有界工程任务委派给集成 agent CLI 在任务 workspace 自主执行(修订 B)。
// 8.2 实现 = claudeDelegator(claude 二进制 agent 模式);codex 等工具族选型属 8.3。
type Delegator interface {
    // Delegate 运行一次委派。实现须把 cwd 钉在 spec.Workspace;返回 res.Diff(res.Workspace 相对 HEAD 的改动)。
    Delegate(ctx context.Context, spec DelegateSpec) (DelegateResult, error)
}

// DelegateSpec 一次有界委派的输入。brief = 任务简报 + 约束(见 3.3.4)。
type DelegateSpec struct {
    Workspace string        // 任务 git workspace(委派 cwd,唯一可写面)
    Brief     string        // 委派简报
    ModelEnv  []string      // 可选 cheap 配对:"ANTHROPIC_BASE_URL=.." / "ANTHROPIC_AUTH_TOKEN=.." / "ANTHROPIC_MODEL=.."
    Timeout   time.Duration // 单次委派墙钟上限(≤ 任务剩余预算)
}

// DelegateResult 委派产出。Diff 为空 → 视为未产出(审计告警)。
type DelegateResult struct {
    Diff   string // workspace 相对 HEAD 的真实改动(git diff 文本)
    Report string // agent 自述(改了哪些文件 / 自测命令与结果),落审计与 execution 供人复核
}
```

真实实现要点:
- 命令:`claude -p <brief> --output-format json --permission-mode bypassPermissions --permission-prompts none`,cwd = `Workspace`;env = `os.Environ()` + `ModelEnv`;`exec.CommandContext(ctx,...)` 尊重 ctx(超时)。
- stdout 为 JSON,取 `result` 文本作 `Report`(agent 自述);**OS 不信任其自述为产出** —— 产出一律以 3.3.2 git 捕获为准。
- `bypassPermissions` 的信任面 = OS 自有 workspace(创建方是 OS/仓库登记),非用户随意目录;真实 claude 授权与 flags 版本差异归 live 验收(§六),契约固定 flags 见上、实现时以 `claude --help` 复核。

`Service` 注入:s `Service` 增加字段 `delegator Delegator`,`New` 默认赋 `&claudeDelegator{}`;测试改写 fake(记录 spec、向 workspace 落一个文件、返回 canned report)。

#### 3.3.2 git 助手(service 内固定命令,不经用户串;镜像 tool/git.go 本地白名单)

| 助手 | 语义 | 实现 |
|---|---|---|
| `wsIsGit(ws)` | 是否为 git 仓库 | `git -C ws rev-parse --is-inside-work-tree` 退出码 0 |
| `wsPorcelainClean(ws)` | 工作树/index 相对 HEAD 无改动(含未跟踪) | `git -C ws status --porcelain` 输出为空 |
| `captureChanges(ws)` | 捕获真实 diff(含新增/删除/改名) | `git -C ws add -A` → `git -C ws diff --cached`;空 diff 也可(记告警) |

- 不删除/不 `reset --hard`:8.2 非破坏。baseline = 任务认领起点(clean HEAD);rework/后续 round 在同一工作树**累积迭代**(都是本任务自己的改动),下一轮 writer 简报带当前状态即可。
- capture 用 `add -A` 顺带把改动暂存(index 留痕);若后续要 git 语义收敛,8.3 定提交/分支策略。

#### 3.3.3 writer live 接线与前置

`engCall` writer 分支(live)改为走委派,不再 text:

```go
// 3.2/3.3 的 engCall live 段(engine.go:47-63 重构后):
//   - role==writer → s.delegateWriter(ctx, t, c)
//   - 其余判读角色 → 原 engEndpointFor + modelCall(见 3.2)

func (s *Service) delegateWriter(ctx context.Context, t task.Task, c engCallCtx) (string, error) {
    ws := t.WorkspacePath
    if ws == "" || !wsIsGit(ws) {
        return "", fmt.Errorf("writer delegation requires a git workspace (task workspace=%q is not a git repo; bind an os repo checkout)", ws)
    }
    spec := DelegateSpec{
        Workspace: ws,
        Brief:     delegateBrief(t, c),
        ModelEnv:  s.delegateEnv(t),           // writer_endpoint_id 可选 cheap 配对
        Timeout:   delegateTimeout(t, c),      // min(任务剩余, delegateMax)
    }
    res, err := s.delegator.Delegate(ctx, spec)
    if err != nil {
        return "", fmt.Errorf("delegate (round=%d retry=%d): %w", c.round, c.retry, err)
    }
    diff, err := captureChanges(ws)
    if err != nil {
        return "", fmt.Errorf("capture diff: %w", err)
    }
    if diff == "" {
        return "", fmt.Errorf("delegation produced no workspace changes (round=%d retry=%d)", c.round, c.retry)
    }
    // 逐次委派审计(actor 沿既有工程动作语义)
    if _, err := s.audit(ctx, "task", t.ID, "eng_delegate", taskActor(t), delegateAuditDetail(t, spec, res)); err != nil {
        return "", err
    }
    return diff, nil
}
```

- **clean 前置**:只检查一次、只在 live、在进入 round 循环首个 writer 前(非每轮 rework)。driver.go 在 writer 首次调用前:`if live { if !wsPorcelainClean(t.WorkspacePath) { return engFail(writer, "task workspace not clean at claim ...") } }`。scripted 跳过。
- `delegateEnv`:有 `writer_endpoint_id` 且 active → `OpenToken` 解密,组 `ANTHROPIC_BASE_URL/ANTHROPIC_AUTH_TOKEN/ANTHROPIC_MODEL`(语义与 `ClaudeCLI.env` 相同);没设 → `nil`(claude 自带鉴权)。
- **返回 = diff 文本**,延续 `runEngPhase` 语义(writer execution 完成结果 = 真实 diff;test/review prompt 照旧消费该 diff)。
- 空 diff 不静默:报错走 `engFail`(requeue/fail),或由调用方改判重试—— MVP 定为报错(可审计、可人工看)。

#### 3.3.4 writer 简报(delegateBrief)

一段有界任务简报(区别于旧「只输出 diff 围栏」提示,专为真干 agent 写):

```
你是 OS 委派的编码工程师,在一个受控 git workspace 里为任务工作。
任务: <title>
描述: <description 前若干行>
上下文: round=<n> reviewer_conflicts=<c>  [retry>0: 这是本轮第 <retry+1> 次改写]
边界:
- 只能在 workspace(<ws>)内读/写文件;只改与本任务直接相关的文件。
- 不要 git commit / push / fetch / pull(OS 会捕获你的改动)。
- 改完运行相关测试/构建自证自洽;命令与结果写进你的结束报告。
- 不改权限、审计、审批、配置等治理文件。
结束报告需列出:改动的文件;跑过的命令与输出结论。
```

round/conflict/retry 语义与旧 writer prompt 对齐(driver 已组好上下文,delegateWriter 自建简报而非复用 engWriterPrompt)。

### 3.4 闭环数据流(live 一轮)

```
runEngineering(认领) → [live] clean 前置(ws git + clean)
  → planner 拆解(网关 Chat 文本,不变逻辑) → direct
  → writer = delegateWriter:
       spec{Brief, ModelEnv?, cwd=workspace} → claude Code 自主读/改/跑
       → git add -A + diff --cached = 真实 diff → audit eng_delegate → execution completed
  → test 判读(网关 Chat 单轮,解析 TEST OK/FAIL 不变)
       FAIL → 免费返工:再次 delegateWriter(retry+1,工作树累积)…(≤3)
  → review 判读(网关 Chat 单轮,VERDICT 不变)
       approve → CompleteTask(id, diff) 完成(结果 = 真实 diff)
       needs_changes → conflict+1 … 熔断/返工语义不变
```

scripted:同流程图但 writer/test/review 全走 `engScripted`,与 6.2 完全一致(回归红线)。

## 四、文件落地清单(8.2)

| 文件 | 内容要点 |
|---|---|
| `internal/service/delegate.go`(新) | §3.3.1 `Delegator`/`DelegateSpec`/`DelegateResult` + `claudeDelegator`(claude -p agent 模式)+ §3.3.2 git 助手(`wsIsGit`/`wsPorcelainClean`/`captureChanges`) + §3.3.3 `delegateWriter`/`delegateEnv`/`delegateTimeout` + §3.3.4 `delegateBrief` |
| `internal/service/engine.go`(改) | `modelCall` 切网关(§3.2 代码);`engCall` live writer 分支 → `delegateWriter`;头注同步(判读=网关文本、writer=委派) |
| `internal/service/driver.go`(改) | live 认领 clean 前置(首个 writer 前一次);其余回合/熔断/审计语义逐字不动 |
| `internal/service/service.go`(改) | `Service` 增 `delegator Delegator` 字段;`New` 默认 `&claudeDelegator{}` |
| `internal/provider/claudecli.go`(改注) | 头注「claude Code agent 委派属 8.5」改口为 8.2 `claudeDelegator`(claude 二进制 agent 模式,同 env 语义);Generate 仍 Deprecated、代码零改动 |
| `internal/service/delegate_test.go`(新) | §五 用例 1-4(注入 fake delegator + 假网关) |
| `internal/service/driver_test.go`(新) | §五 用例 5-6(scripted 回归闭环 + writer 前置失败)—— 补齐 driver 单测空缺 |
| `docs/phase8/design/provider-upgrade.md`(改注,可选) | §七 最后一条备忘「委派工具被模型当 function tool 调用」随修订 B 过时,加一行指向 8.2/修订 B |
| `docs/phase8/stages/2.md`(落地后归档)+ 进度总表登记 | 收口凭证 |

依赖:全部 stdlib(含既有 `os/exec`/`net/http`);**无迁移、无 sqlc、无 go.mod 变更**。

## 五、用例清单(验收凭证)

假网关 = `httptest.NewServer` 实现 `/v1/chat/completions`(捕获 method/header/body,按调用序号回 TEST OK / VERDICT approve 等)。fake delegator 直接写 workspace 文件、返回 canned report。harness:`storage.Open(t.TempDir())` → `repository.NewStore` → `service.New`,seed company/capability/endpoint(task 用)。

| # | 用例 | 断言 |
|---|---|---|
| 1 | git 助手 | seed git ws(init+user+base file):`wsIsGit` true;clean→`wsPorcelainClean` true;fake 写 新增+修改+删除 文件 → `captureChanges` diff 三态齐全、clean 变 false;非 git 目录 `wsIsGit` false |
| 2 | writer 委派闭环(fake delegator + 假网关判读) | 建 engineering task(git ws,reviewer_endpoint=假网关 openai);`s.delegator=fake`;执行 writer→test→review 一轮:writer execution 完成、结果含真实 diff(fake 写入文件在 diff 内)、audit `eng_delegate` 存在(actor=`agent:…`);假网关收到 POST `/v1/chat/completions` + `Authorization: Bearer`(证判读切网关);task completed 且 result=最终 diff |
| 3 | 判读 proto 门 | `modelCall(proto=anthropic)` → error 含 `proto=openai` 提示;`proto=openai` 正常回 Content |
| 4 | 判读 length 截断 | 假网关回 `finish_reason:"length"` → error 含 truncated(不静默当成功) |
| 5 | scripted 回归闭环 | `OS_ENGINE_MODE=scripted` + `OS_SCRIPT_TEST=pass` + `OS_SCRIPT_REVIEW=approve` 跑最小一轮 → completed、结果含确定性假 diff;委派/网关零命中(fake delegator 未调用计数 0) |
| 6 | writer 前置失败 | 任务 workspace 非 git / 认领时不 clean → `engFail(writer,…)` 报错含 git workspace 指引;scripted 下不受影响 |

验收口径:
1. `go build ./...`、`go test ./...` 全绿;**0-7 与 8.1 既有测试零改动**(8.2 只改 service 三个文件 + 注释 + 新增测试)。
2. 新增用例全绿;`go vet ./internal/...` 干净。
3. 无迁移、无 go.mod 变更;scripted 服务层确定性文本与 6.2 一致。
4. **live 验收归用户**:真实 claude Code 授权下的委派(自主改文件 → OS 收真实 diff → 网关判读闭环);`--permission-mode bypassPermissions` 等 flags 按本机 `claude --help` 复核;cheap 配对需 Anthropic 方言端点(claude 自己的经济模型,或 Anthropic 兼容网关)。

## 六、风险与取舍

- **claude Code 授权/live 依赖** → 委派真实调用需用户 claude 会话/API 授权;离线/CI 以 fake delegator + scripted 全绿证明逻辑,live 冒烟归用户验收(同 6.x 既有惯例)。
- **bypassPermissions 信任面** → 委派 cwd 钉死 OS 自有 git workspace;治理仍由 round 审批/熔断兜底;目录若共享用户真实 checkout,认领 clean 前置 + 简报「只改本任务相关」双保险;8.3 收紧(allowlist/Bash 过滤/沙箱)。
- **OpenAI 方言网关 ≠ claude Code 后端** → claude CLI 走 Anthropic `/v1/messages` 方言;若 cheap 配对端点是 OpenAI-only 网关,委派会 404。8.2 定位:writer_endpoint 可选且方言自负,文档明示;多数场景 claude 自带经济模型即满足「cheap」。
- **agent 自行 commit → capture 空** → 简报禁 commit + 空 diff 显式报错(不静默);git 提交/分支语义 8.3 定。
- **判读切网关后 live 端点要求变严** → 判读端点须 proto=openai;旧 anthropic 端点 live 报错指引换网关端点(方向既定,scripted 不受影响)。
- **length 截断 / judge 长 diff** → 截断显式报错返工;diff 文本较大时 judge token 成本与截断风险上升 → 判读结构化(8.3)与预算(8.4 后续)兜底,8.2 不预支。
- **工作树累积返工** → rework 迭代同一工作树(非破坏),diff 随 round 累积变大;8.3 子任务隔离/重置语义一并解决。

## 七、留给 8.3 的接线点(备忘,不在 8.2 做)

- **判读结构化**:收敛信号替换 `parse*` 正则;writer 返工「喂上轮 test 判读失败原因」;planner/test/review 输出结构化 JSON 信号。
- **委派边界**:planner split 子任务共享工作树仲裁(提交/分支/reset 语义、串行排队);agent 工具族注册选型(claude→codex);委派权限收紧(allowlist/受限 Bash/沙箱)。
- **tier 默认落档(8.4)**:`endpoint.tier` 迁移 0010 + 角色默认映射(planner/review=frontier、test=standard、writer cheap 配对落 claude env),判读端点按档自动挑 —— 8.2 判读仍走显式端点。

## 八、实施修订记录(2026-09-04,8.2 代码落地时;定稿契约仍冻结,以下为实施期对个别要点的落地口径微调)

实现按本契约 §四 文件清单落地,`go build ./...` + `go test ./...` 全绿、0-7 与 8.1 零改动。两处**实施期口径细化**(非方向变更,均因照本契约原样落地会与既定语义冲突,如实记录):

1. **认领起点 clean 前置的判别细化(§3.3.3/§五 用例 6)**:原契约「脏即 engFail」。实现改为 **非 git → 硬错误;脏且本任务从未委派过(无 `eng_delegate` 审计)→ 硬错误;脏但已有本任务 `eng_delegate` 审计 → 放行(残留 = 本任务先前 attempt/返工的产物,累积迭代)**。原因:回合机自有 rework/熔断续跑/requeue 会让任务第二次进入 runEngineering 时工作树里**必是自己的残留**;若脏一律硬失败,任务将永远无法在自己残留上续跑(自锁)。判别靠新增 `HasAuditAction`(见下),把「首次认领保护无关改动」与「自己的残留可续跑」分开 —— 契约精神(认领起点须 clean,不吞并先前存在的无关改动)保持不变。
2. **文件清单追加一项(§四)**:`internal/storage/repository/audit.go`(改)—— 新增 `HasAuditAction(entity_type, entity_id, action)` 存在性查询(手写参数化 SQL,无迁移/无 sqlc),供第 1 条判别使用。其余清单逐项照落地。
3. **接口口径确认(§3.3.1)**:`DelegateResult.Diff` 在真实 `claudeDelegator` 中留空 —— 产出统一由 `delegateWriter` 的 `captureChanges` 捕获(OS 不信任 agent 自述),与 §3.3.3 一致;Diff 字段仅为未来自行捕获的实现保留。

live 验收仍归用户(真实 claude Code 授权下的委派 + `--permission-mode bypassPermissions` flags 按本机 `claude --help` 复核)。8.2 落地归档与登记见 `stages/2.md` / 进度总表。
