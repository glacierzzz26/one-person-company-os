# Phase 8.3 — agent CLI 工具族 + 委派边界 + 判读结构化(实施契约)

> Phase 8 子阶段实施契约,承接方向设计 [model-runtime.md](model-runtime.md) §六 8.3 行
> (claude Code/codex 等工具族注册与选型;委派边界收紧;planner/test/review 判读信号结构化,替换正则抠取)
> 与 8.2 契约 [agent-delegation-loop.md](agent-delegation-loop.md) §七「留给 8.3 的接线点」。
> 用户定稿门决策(2026-09-06):**8.3 单阶段整体交付**(一份契约、三块工作流、一块归档);
> **共享工作树仲裁 = OS 逐次委派自动 commit**(每次委派结束 capture 后由 OS 提交,见 §三 C1)。
> 本文件是 8.3 的执行蓝本:**代码现状全部经仓库核实,类型/签名照抄真实代码,不凭想象**。
> 已定稿(2026-09-06,定稿门通过)。定稿冻结,按它写代码;落地后归档 `stages/3.md` 并在进度总表登记。

## 一、范围与边界

**做(8.3 交付物,三块工作流):**

1. **判读结构化(A)**:planner / test / review / intake triage 判读输出统一为**结构化 JSON 信号**(单一对象、fence 可容忍),
   解析 **JSON 为主、旧标记兜底为辅** —— 正则不再当执行通道;review/test 的**理由/失败摘要被捕获**;
   writer 返工把上一判读的**失败原因喂回**(test 失败摘要、review needs_changes 理由进下一轮 writer 简报)。
2. **agent CLI 工具族注册与选型(B)**:委派目标由「钉死 claude」改为**按族注册 + 选型**(`OS_AGENT_CLI`,
   默认 `claude`);`codex` 注册槽位(可用性 = `exec.LookPath` 门),真实适配留 live 验收;
   逐次委派审计带族名,与网关档位正交(委派 claude 时 cheap 配对语义不变,8.2 既有)。
3. **委派边界 + 共享工作树仲裁(C)**:**OS 逐次委派自动 commit**(当前分支、确定性 message、仓库身份;
   无身份 → 清晰报错);**baseline ref**(`refs/os/tasks/<taskID>` 轻量引用,首委派钉 HEAD)使
   capture 语义 = **净 diff(自任务起点)**,且每次委派后工作树归 clean —— planner split ≤8 子任务
   共享同一 repo workspace 串行执行时,前一子任务的产出以 commit 收口,不再污染下一子任务的 clean 认领前置
   (8.2 遗留「多任务同库仲裁」的根治);委派工具残留目录(`.claude/` 等)以 `.git/info/exclude` 排除,不入 diff、不脏 clean 门。

**不做(留给 8.4 / live / 后续,防越界):**

- **不改 endpoint.tier / 角色默认落档 / 迁移 0010**:属 8.4。8.3 判读仍走任务显式端点(建单
  `--reviewer-endpoint`/planner 端点),不引新字段、不加迁移、不加 sqlc、不改 go.mod。
- **不做真实 codex 适配的 live 验证**:codex 二进制本机未见,8.3 只落注册槽位 + 可用性门 + 明确报错;
  真实 codex 运行(flags/授权)归用户 live 验收(同 6.x/8.2 惯例)。
- **不做 OS 级沙箱/受限 Bash 包 claude 内部**:`bypassPermissions` 下 OS 无法在子进程内逐条过滤 agent 的 shell;
  边界 = 委派 cwd 钉死 workspace + 有界简报 + 净 diff 审计 + round/熔断治理 + commit 原子化(可回退)。目录若共享用户真实
  checkout,clean 认领前置 + 简报「只改本任务相关」双保险(8.2 既有)保持。
- **不改已冻结 Phase 0–7 契约与 8.1/8.2 已冻结语义**:回合机/熔断/审批/审计字段/scripted 分界逐字保留;
  8.3 只改「判读信号形态、委派收口方式、委派目标选型」三处,且均加向后容忍(旧标记文本仍可判)。

## 二、代码现状核实(8.3 立足点,全部经仓库核实)

| # | 事实 | 位置 |
|---|---|---|
| 1 | review/test 判读 = **单行标记 + 正则抠取**:`parseReviewVerdict`(VERDICT regex)、`parseTestPass`(TEST OK/FAIL + 「无失败即过」启发式)、`extractDiff`(diff 围栏,writer 已走委派故实为遗留);错误分支拿不到理由 | `internal/service/engine.go:168-215` |
| 2 | 判读 prompt 要求单行文本(review: `VERDICT: approve|needs_changes[:reason]`;test: `TEST OK|TEST FAIL:<reason>`),脚本态 `engScripted` 吐同样标记文本 | `internal/service/driver.go:227-247`、`engine.go:116-153` |
| 3 | planner 已 JSON 优先(`{"action":..,"subtasks":..,"reason":..}`),正则仅兜底;triage 仍 **正则主解析**(`DISPOSITION:` 单行,`dispositionRe`) | `internal/service/planner.go:127-178`、`intake.go:315-318` |
| 4 | writer 返工**不喂失败原因**:test 免费返工与 review needs_changes 后,下一 writer 只见 retry 计数 / conflict 数;`delegateBrief` 无失败摘要段 | `internal/service/driver.go:99-122,145-167`、`delegate.go:244-265` |
| 5 | writer live 委派 = `delegator.Delegate`(Service.delegator,默认 `claudeDelegator`),命令钉死 `claude -p ... bypassPermissions`,族不可选 | `internal/service/delegate.go:53-92`、`service.go` |
| 6 | capture = `git add -A` + `git diff --cached`(相对 HEAD,**不 commit**);工作树脏残留跨委派/跨任务累积;8.2 baseline 靠「本任务有无 `eng_delegate` 审计」放行自己的残留 | `internal/service/delegate.go:123-129,171-190` |
| 7 | **split ≤8 子任务共享父工作区、串行驱动**(`createPlannedSubtask` 继承 `t.WorkspacePath`;`driveChildrenToDone` 顺序 ExecuteTask);子任务各是独立 task ID —— 子任务 1 的未提交残留会让子任务 2 的 clean 认领前置硬失败(8.2 遗留自锁,8.3 根治点) | `internal/service/plan_driver.go:84-137`、`delegate.go:171-190` |
| 8 | 通道 A workflow 工程节点工作区 = `os.MkdirTemp`(**非 git**);通道 B repo checkout 天然 git。委派 git 前置要求不变(非 git → 清晰报错) | `internal/service/workflow.go:102-112` |
| 9 | scripted 分界在 `engCall` 第一位;scripted 下 writer/test/review 永不落委派/网关(离线可复现红线) | `internal/service/engine.go:46-49` |
| 10 | git 助手固定命令(`git -C ws ...`)、不删不 reset(8.2 非破坏语义);测试 harness = `storage.Open(tmp)` → `repository.NewStore` → `service.New`,假网关按 body 分类回 `TEST OK`/`VERDICT: approve`,git ws 由 `seedGitWorkspace` 建(init+config+seed commit) | `internal/service/delegate_test.go:158-181,198-248`、`driver_test.go` |

**委派收口语义前提(定稿门决策,§一 C)**:capture 由「相对 HEAD 的累积脏 diff」改为「相对 **baseline ref** 的净 diff」。
旧语义下 HEAD 从不移动(不 commit),净 diff 与累积 diff 内容**一致**;新语义下每次委派后 OS commit、HEAD 前移,
净 diff(相对仍钉在任务起点的 baseline ref)依然等于任务自起点以来的全部真实改动 —— 判读(test/review)与完成结果
消费的 diff 语义**与 8.2 逐字等价**,只是收口方式变了(8.2 工作树残留 → 8.3 commit 原子化)。

## 三、契约设计

### A. 判读结构化

#### A1 统一判读 JSON 信号(执行通道主契约)

各判读角色输出**恰好一个 JSON 对象**(无 prose、无 fence;解析容忍 fence/前后缀),字段与既有语义对齐:

| 角色 | JSON 信号 | 现有 parse 被替换为 |
|---|---|---|
| test | `{"pass":true,"summary":"<结论>"}` / `{"pass":false,"summary":"<失败摘要,喂 writer>"}` | `parseTestPass` → `parseTest(out) (pass bool, summary string)` |
| review | `{"verdict":"approve"}` / `{"verdict":"needs_changes","reason":"<理由>"}` | `parseReviewVerdict` → `parseReview(out) (verdict, reason string)` |
| planner | `{"action":...}`(**不变**,已 JSON;8.3 收进统一解码) | `parsePlan` 保留(换用统一解码助手) |
| triage | `{"disposition":"direct_work\|ask\|skip\|merge","note":"..."}` | `parseDisposition`(正则主解析) → JSON 主 + 旧标记兜底 |

解析层统一收进新文件 `internal/service/judge.go`:
- `decodeJudgeJSON(out string, v any) bool`:去 ``` 围栏、TrimSpace、`json.Unmarshal`,成功即真。
- **旧标记兜底(防模型不守约,不是执行通道)**:JSON 缺省时退守既有单行标记扫描(review VERDICT / test OK-FAIL /
  triage DISPOSITION / planner ACTION 的**紧凑前缀/包含扫描**,把 engine.go 的整段 regex 语义收敛为一个 `legacyVerdict`/
  `legacyTest` 等小函数);scripted 输出也切到 JSON(见 A2),故兜底仅对 stray 文本生效。
- 每角色 validator(如 review 未知 verdict → 空 = 不可判,driver 走既有 engFail 报错,与现状一致)。

#### A2 scripted 输出同步

`engScripted` 的 test/review 分支输出改为新 JSON 契约(确定性),与 parse 主路径同构:
`OS_SCRIPT_TEST` pass/fail-once/fail-all → `{"pass":true,...}` / `{"pass":false,"summary":"scripted permanent failure"}` 等;
`OS_SCRIPT_REVIEW` approve/reject/reject:N → `{"verdict":"approve"}` / `{"verdict":"needs_changes","reason":"scripted reviewer disagreement"}`。
**writer scripted 不变**(writer 已被委派取代,其确定性假 diff 文本继续作为脚本态 writer 阶段产出,0-7/8.2 断言照旧)。

#### A3 返工喂失败原因(rework feedback)

- `engCallCtx` 增 `hint string`:上一判读的失败/驳回理由(test 失败摘要 或 review needs_changes 的 reason)。
- `delegateBrief`(writer 委派简报)在 `Context:` 段后追加:
  `Previous judging feedback: <hint>`(hint 非空时)。
- driver 循环:
  - test 免费返工:test FAIL 后把 `summary` 作为 hint 喂下一次 writer(重写尝试被告知「上一版为何没过测试」);
  - review needs_changes:把 `reason` 作为 hint 喂**下一轮** writer(conflict 返工时告知 reviewer 具体顾虑)。
- 8.2 已禁 agent 自行 commit;commit 全由 OS 做(见 C1),故 hint 只进简报文本,不产生 repo 写入。

### B. agent CLI 工具族注册与选型

`delegate.go` 增注册表 + 选型(接口 `Delegator` 不变,族适配器各自实现它):

```go
// agent CLI 工具族(修订 B「claude 首位 → codex 等」)。注册槽位 + 选型,8.3 真实现 = claude。
const (
    agentCLIClaude = "claude" // claude Code(agent 模式,8.2 claudeDelegator,真实现)
    agentCLICodex  = "codex"  // codex CLI:注册槽位;可用性=LookPath 门;真实适配留 live 验收
)

// agentCLIRegistry 族名 → Delegator 适配器。真适配器自行在 Delegate 内做二进制可用性检查。
var agentCLIRegistry = map[string]Delegator{
    agentCLIClaude: &claudeDelegator{},
    agentCLICodex:  &codexDelegator{},
}

// agentCLIFromEnv 选型:OS_AGENT_CLI(默认 claude);未知族 → error(列已知族)。
// 与网关档位正交:writer 委派 claude 的 cheap 配对(ANTHROPIC_* env)语义 8.2 既有,族不影响。
```

- `DelegateSpec` 增 `Family string`(本次委派族名,落审计,供人复核「谁在被委派」)。
- `codexDelegator` = 注册槽位:`Delegate` 先 `exec.LookPath("codex")`(缺失 → 明确报错「codex 未安装 /
  8.3 仅注册槽位,真实适配留 live」);已装也返回 not-implemented(防误以为 codex 已真跑)。**诚实占位**。
- `Service.delegator` 注入缝语义不变(测试注入 fake 仍走 `claude` 默认族;fake 无视 Family)。

### C. 委派边界 + 共享工作树仲裁(核心 = OS 逐次委派自动 commit)

#### C1 git 助手改造(baseline ref + commit,替换 8.2 captureChanges 语义)

新增/改动固定命令(service 内,不经用户串):

| 助手 | 语义 | 实现 |
|---|---|---|
| `wsBaselineRef(ws, taskID)` | 任务起点 ref 名 | `refs/os/tasks/<taskID>` |
| `excludeWorkspaceTools(ws)` | 委派工具残留目录不进 repo | 把 `.claude/`、`.codex/` 追加进 `.git/info/exclude`(幂等、本地、不入 commit) |
| `ensureBaseline(ctx, ws, taskID)` | 首委派钉起点 | ref 不存在 → `excludeWorkspaceTools` + `git update-ref refs/os/tasks/<id> HEAD`;已存在 → no-op(ref 钉在任务起点,续跑/返工不重钉) |
| `captureNet(ctx, ws, ref)` | 净 diff(自任务起点) | `git add -A` → `git diff --cached <ref>`(含中间 commit + 未提交改动;与 8.2 累积 diff 内容等价) |
| `commitDelegation(ctx, ws, msg)` | OS 提交本次委派 | 有已暂存改动(`git diff --cached --quiet` 非 0)→ `git commit -m <msg>`(用仓库身份;无身份 → 清晰报错给 git config 指引);无暂存(agent 已自 commit / 只回退)→ skip |
| `delegateCommitMsg(c)` | 确定性 message | `os-delegate: <task8> round=<r> retry=<ret> family=<fam>` |

流程变化(delegateWriter,8.2 §3.3.3 的修订):git 前置检查(wsIsGit)→ `ensureBaseline` → 委派 →
`captureNet`(空 → 既有「no workspace changes」报错)→ audit `eng_delegate`(detail 带 family + ref + diff 规模,
先行记录保证 commit 失败后重认领仍可放行)→ `commitDelegation`。**工作树在每次委派成功后归 clean**。

commit 语义要点:
- **当前分支直接提交**,message 带 task8,历史可归属;agent 简报已禁自行 commit,OS 是唯一提交者。
- **baseline ref 永不前移**:任务完成后可 `git update-ref -d refs/os/tasks/<id>` 或交由后续阶段 GC(8.4 不涉及;
  残留 ref 无害、属任务账本)。清理策略记入 §六 风险(可选收口动作,不做也自洽)。
- 首委派要求工作树 clean 的既有前置保证 ref=HEAD 合法(脏起点会在 baseline 门被挡,见 C2)。

#### C2 baseline 门判别(修订,8.2 delegateBaseline 的语义延续)

| 认领时工作树 | 判定 | 结果 |
|---|---|---|
| 非 git | 硬错误 | 既有(git workspace 指引) |
| clean | 放行 | 首委派 ensureBaseline 钉 ref=HEAD |
| 脏 + 本任务已有 `eng_delegate` | 放行 | 残留 = 本任务中断委派(commit 前被杀)的产物,`captureNet` 一并计入净 diff |
| 脏 + 本任务从未委派 | 硬错误 | 既有(不吞并先前无关改动) |

与 8.2 的差别:**不再需要「跨任务同血缘放行」** —— 前一任务/前一子任务的产出已由 C1 以 commit 收口,工作树归 clean,
下一任务认领天然过 clean 门。跨任务残留仅剩「委派被 kill 在 commit 之前」的窄窗,仍由「脏 + 有本任务 eng_delegate」兜住,
其他任务无法认领他人残留(安全)。

#### C3 多任务同库 / 子任务共享工作树(现状与 8.3 收口)

- split ≤8 子任务共享父 repo workspace、**串行驱动**(6.4 既有,server 队列亦单 goroutine 排空):C1 commit 收口后,
  子任务 1 completed → 子任务 2 认领见 clean → 正常委派。**多任务同库 = 串行 + 每委派原子 commit**,不引入并发写。
- 子任务各自 `ensureBaseline` 钉各自的 ref(独立 taskID),净 diff 互不混淆;父任务聚合摘要逻辑(6.4)不变。

#### C4 边界收紧密集(诚实口径)

bypassPermissions 下 OS 不逐条过滤 agent 内部 shell。8.3「收紧」落地为可审计的收口边界:
1. cwd 钉死 workspace(既有)+ **每委派一次 commit = 一个可回退的原子单位**(`git reset --hard refs/os/tasks/<id>` 可整任务回退);
2. 净 diff 全部落 `eng_delegate` 审计(相对任务起点,非 8.2 的相对 HEAD 残留)—— 复核者看到**本任务到底改了什么**;
3. `.claude/`/`.codex/` 工具残留不进 diff、不进 commit、不脏 clean 门;
4. 简报边界(只改本任务文件 / 禁 commit·push / 自测自证)既有保持,失败原因回喂(A3)后简报更有的放矢。

## 四、文件落地清单(8.3)

| 文件 | 内容要点 |
|---|---|
| `internal/service/judge.go`(新) | A1 统一解码 `decodeJudgeJSON` + 旧标记兜底小函数;`parseTest`(pass,summary)/`parseReview`(verdict,reason) 及各自 validator;triage JSON 解析(`parseDisposition` 改 JSON 主 + 兜底,可留在 intake.go 或迁此 —— 落地取迁此,删 intake.go 的 dispositionRe) |
| `internal/service/engine.go`(改) | `engScripted` test/review 输出切 JSON(A2);删 `reviewVerdictRe`/`testOkRe`/`testFailRe`(正则语义并入 judge.go 兜底);`parse*` 旧函数移除/改指 judge.go |
| `internal/service/driver.go`(改) | 循环用 `parseTest`/`parseReview` 拿 summary/reason;test 免费返工与 review 返工把 hint 喂下一 writer(A3);`engCallCtx.hint` 贯通 |
| `internal/service/delegate.go`(改) | B 注册表(`agentCLIRegistry`/`codexDelegator`/`agentCLIFromEnv`)+ `DelegateSpec.Family`;C1 助手(`wsBaselineRef`/`excludeWorkspaceTools`/`ensureBaseline`/`captureNet`/`commitDelegation`/`delegateCommitMsg`) 替换 captureChanges 语义;delegateWriter 新流程;delegateBrief 追加 hint 段;audit detail 带 family+ref |
| `internal/service/service.go`(改) | `New` 默认 delegator 保持 `&claudeDelegator{}`(族 claude);必要时暴露选型入口(经 delegateWriter 内 `agentCLIFromEnv` 解析,Service 本身不存族) |
| `internal/provider/claudecli.go`(改注,可选) | 头注 8.2 口径已含「claudeDelegator」;若提 codex 族,补一句指向 8.3 注册表 —— 仅注释 |
| `internal/service/judge_test.go`(新) | A 用例:各角色 JSON 主解析 + 旧标记兜底 + 坏 JSON/未知 verdict 行为 |
| `internal/service/delegate_test.go`(改) | 适配:C1 commit 后 `captureNet`/clean 断言、audit detail 新形状、假网关回 JSON 内容;新增 C 共享 repo 两子任务串行端到端(C1 收口 → 子任务 2 clean 过门) |
| `internal/service/driver_test.go`(改) | 适配 scripted 新 JSON 输出 + rework hint 断言(返工简报含 test 失败摘要) |
| `docs/phase8/stages/3.md`(落地后归档)+ `design/README.md` 登记 + `docs/进度总表.md` | 收口凭证 |

依赖:全部 stdlib;**无迁移、无 sqlc、无 go.mod 变更**。

## 五、用例清单(验收凭证)

harness 沿用 delegate_test/driver_test(seedGitWorkspace + 假网关 + writingDelegator)。假网关按 body 分类回 **JSON 判读内容**。

| # | 用例 | 断言 |
|---|---|---|
| A1 | judge JSON 主解析 | `parseTest({"pass":false,"summary":"x"})` → (false,"x");`parseReview({"verdict":"needs_changes","reason":"r"})` → ("needs_changes","r");带 ``` fence / 前后缀 prose 也可解;planner/triage JSON 同过统一解码 |
| A2 | judge 旧标记兜底 | 模型回 `TEST FAIL: boom` → parseTest(false,...);`VERDICT: approve` → approve;坏 JSON → 不崩,走兜底或不可判(按角色语义) |
| A3 | scripted JSON 回归闭环 | `OS_ENGINE_MODE=scripted` + `OS_SCRIPT_TEST=pass` + `OS_SCRIPT_REVIEW=approve` → completed、result=writer 确定性假 diff(不变);委派/网关零命中(fake 计数 0) |
| A4 | rework hint 回喂 | test fail-once 冒烟:第二次 writer 委派(或 scripted 报错路径)的 `DelegateSpec.Brief`/日志含上一版失败摘要;review needs_changes 后下一轮 writer brief 含 reason |
| B1 | 族选型 + 槽位 | `OS_AGENT_CLI=codex`(未装)→ delegateWriter 明确报错(codex 槽位/未装,不假装能跑);`=claude`(默认)→ 真 claude 路径;`=bogus` → 列已知族;eng_delegate audit detail 含 family=claude |
| C1 | commit 原子化 + 净 diff | 委派一次(fake 落文件)→ 工作树归 clean、HEAD 前移一个 commit、message=os-delegate: 前缀;`captureNet`(ref) 返回含 fake 落盘文件的净 diff;task result = 净 diff(与 8.2 语义等价断言) |
| C2 | 共享 repo 两子任务串行 | 同一 git ws 两个工程子任务(A→B)串行 ExecuteTask:B 的 clean 认领前置放行(A 的产出已 commit 收口);A/B 各自净 diff 不混淆;两次委派 audit 各带各自 ref |
| C3 | commit 前 kill 窄窗兜底 | 制造脏 + 本任务已有 eng_delegate(无 commit)→ baseline 放行,captureNet 净 diff 计入残留(8.2 语义延续) |
| C4 | `.claude/` 排除 | 首委派后工作区内出现 `.claude/x` 未跟踪目录 → porcelain 仍 clean、captureNet 不含其内容 |

验收口径:
1. `go build ./...`、`go test ./...` 全绿;0–7 与 8.1 既有测试零改动,8.2 测试仅按新契约(JSON 网关内容 / audit detail / capture 语义)适配。
2. `go vet ./internal/...` 干净;无迁移、无 go.mod 变更;scripted 分界与确定性保持(离线可复现)。
3. 判读 JSON 主契约下,同一 scripted/假网关输入产出与原 8.2 相同阶段语义(直接/熔断/审批回归)。
4. **live 验收归用户**:真实 claude Code 委派(commit 收口 + 净 diff 判读闭环)、codex 真适配与 flags、cheap 配对 —— 同 6.x/8.2 惯例。

## 六、风险与取舍

- **OS commit 进用户仓库历史** → message 带 `os-delegate:` + task8 前缀可归属;agent 已禁自行 commit,OS 唯一提交者;
  若仓库将来 push,自动化提交随之可见 —— 收口动作可后续提供「整任务 squash / reset 到任务分支」,8.3 不做。
- **仓库无 git 身份 → commit 失败** → 报错给 `git config user.name/email`(或 OS_GIT_* env 未来覆盖)指引;
  委派本身已成功、diff 已在审计,不丢产出,重认领走自身残留放行续收口。
- **rejected 中间 commit 累积** → 每委派一次一提交,review 驳回的中间态留在历史;净 diff(相对任务起点)是判读口径,
  历史脏中间态不影响语义;后续可 squash(见上),8.3 不预支。
- **codex 槽位无真适配** → LookPath 门 + not-implemented 明确报错,不假装可跑;真适配 + flags 复核归 live 验收(明示于 §五 4)。
- **JSON 模型不守约** → 旧标记兜底仅防解析崩,执行通道语义不依赖正则(与 8.2 prompt 单行标记的差异 = 主契约 JSON、
  理由可捕获);scripted/假网关确定性保证离线回归线。
- **baseline ref 残留** → 每任务一条轻量 ref,无害、属账本;清理(completed/fail 后 `update-ref -d`)为可选项,后续阶段定。

## 七、实施修订记录

按 §四 清单落地(2026-09-06),实施期对个别要点的口径微调如下(live 验收仍归用户,同 8.2 §八惯例):

1. **助手签名按落地简化**:设计表 `wsBaselineRef(ws, taskID)` → 实现 `wsBaselineRef(taskID)`(ref 名只依赖 taskID,ws 冗余);
   `delegateCommitMsg(c)` → 实现 `delegateCommitMsg(t, c, family)`(message 需 task8 = `short8(t.ID)` 与 family,表内为简写)。
   语义均与表逐字一致(净 diff 空判、commit skip 分支、message 形态)。
2. **`captureChanges` 移除**:8.2 助手「相对 HEAD 不 commit」语义不再有任何调用方,删除;`TestGitHelpers` 重写为 C1 循环
   (pin → net → commit → clean → 净 diff 跨 commit 保持),与 §五 C1 口径一致。
3. **codex 可用性门在适配器内**:`codexDelegator.Delegate` 内 LookPath(与 §三 B「真适配器自行在 Delegate 内做二进制可用性检查」一致);
   缺失/已装都在**委派前**报错(不落盘、不审计、不 commit),`delegateWriter` 的族错误经 `delegatorFor`/`Delegate` 上游返回。
4. **空 diff 判在净 diff 上**(§三 C1 流程逐字):commit 后净 diff 含历史收口内容,「本次委派无新改动但任务有历史产出」不报空 ——
   与 8.2「累积 diff 非空即放行」语义等价(判读重判同一内容),不引入 staged-vs-HEAD 的额外判别。
5. **`commitDelegation` 以 `git diff --cached --quiet` 退出码分支**:无暂存 → skip(agent 只回退到基线 / 上一委派已收口);
   有暂存 → `git commit`(仓库身份;无身份 → 报错含 `git config user.name/email` 指引,委派产出已在审计不丢)。
6. **`excludeWorkspaceTools` 幂等追加**带一行归属注释(`# one-person-company-os: agent CLI tool residue (Phase 8.3 C1)`),
   仅本地 `.git/info/exclude`,不入 commit、不脏 clean 门。
7. **文件清单超量**:`internal/provider/claudecli.go` 头注补 8.3 族注册表指针(仅注释,§四 可选落地)。

**用例覆盖对照(§五)**:A1/A2 → `judge_test.go` 单测(JSON 主/fence/prose/兜底/坏输入);A3 → `TestScriptedRoundLoopRegression`(原样保持);
A4 → `TestReworkHintFeedsNextWriterBrief`(live 假网关 fail-once → 第二次 writer 简报含失败摘要);B1 → `TestAgentCLIFamilySelection`;
C1/C4 → `TestGitHelpers`(C1 循环)+ `TestExcludeWorkspaceToolsKeepsClean` + `TestDelegateWriterRoundLoopGateway` 扩展(clean/commit/ref 断言);
C2 → `TestSharedWorkspaceSubtaskSerialCommits`;C3 → `TestDelegateCommitKilledResidueRecovery`。
