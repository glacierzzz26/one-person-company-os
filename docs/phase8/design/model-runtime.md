# Phase 8 — 模型运行时:多厂商 tool-use + 角色分层(方向设计)

> 阶段方向级设计,已定稿(2026-09-04)。定稿冻结,按它立项、子阶段逐项验收。
> 承接:方向基线 v1.1(`Agent → Runtime → Model`)、Phase 6(端点池/回合机/熔断/planner 拆解)、
> Phase 7(控制台经 `/api/v1` 配端点/建工程请求)。
> 触发:用户方向纠正 ——「claude 应该只是一个工具才对:高智模型拆分,普通模型 + claude 干活」。
> 修订 A(2026-09-04,方向补充,已定稿):用户自建 **OpenAI 兼容 API 网关**(单 key 聚合多厂商、支持 function calling 透传);
> OS 不再按厂商适配 —— 8.1 由「anthropic/openai 双原生实现」收敛为「**单一 Chat Completions 兼容适配器对接网关**」;
> 选模型 = 网关模型目录内按档/角色挑;tier 档位与 claude Code agent 委派(8.5)原案保留。受影响段:§一 目标1、§三、§四 D1、§六 8.1/8.2、§八 验收 2,详见 §十一。

## 一、定位与目标

现状把「模型调用」做成**单一 claude 无头文本补全**(`claude -p`),导致三件事做不到:
想在池子里放便宜的第三方模型当写手 → 做不到;让模型在回合里真调用工具/真改工作区 → 做不到
(工具能力被 `claude -p` 内部的 agent 消耗,OS 只拿回文本用正则抠 PATCH/VERDICT);
按角色强制分档用模型(高智拆/审、普通写)→ 做不到。

**Phase 8 把「智能产生」这一层重做**:Provider 升级为**多厂商 HTTP + 原生工具调用(tool-use)**,
claude 从唯一后端**降为池中一个可调用的 agent 工具**;Go 保留回合骨架与全部治理
(审批/熔断/审计/账本/父子聚合),阶段由「文本提示 + 正则解析」升级为「模型原生 tool-call 回合」,
并由 Go **强制角色→模型档位分层**(planner=高智、writer=经济、review=高智,可显式覆盖)。

三个目标(验收时逐条可证):

1. **模型池可真用**(修订 A):自建 OpenAI 兼容网关(单 key)聚合多厂商,**厂商适配归网关**;OS 对接网关即可把任意网关模型按档接入池子并真实生成(不再只有 claude CLI)。
2. **阶段原生工具调用**:工程回合(writer/test/review/planner)里,模型能真正调工具 —— 在工作区写/改文件、跑测试、取回结果再决策,收敛信号结构化;不再靠正则解析 PATCH/VERDICT 文本。
3. **角色分层强制**:端点带档位,角色默认档位映射落成建单/执行的确定性规则,成本可控、可解释。

## 二、现状与差距(架构事实,全部经代码核实)

| # | 现状 | 与目标的差距 |
|---|---|---|
| 1 | `internal/provider` 只有 `Stub`(假)和 `ClaudeCLI`(真)。真后端 = `exec claude -p` 无头,文本进/文本出 | 唯一真后端是 claude 生态;openai/其他厂商在 `endpoint.models` 里看得到但**生成永远走 claude 子进程** → 差距 1 |
| 2 | 阶段产出是文本协议,Go 用 `extractDiff`/正则抠 PATCH、`parseTestPass`/`parseReviewVerdict` 判 TEST/VERDICT;diff 只当文本传来传去,**不在工作区落盘/真跑** | 模型不拥有工具;「写→测→审」是围绕一段文本的对话,不是在工作区真干 → 差距 2 |
| 3 | 任务级只有 `writer_endpoint_id`/`reviewer_endpoint_id`(建单显式给,不给就报错),无档位概念;planner 走公司 role=planner/pool | 没有「普通写 / 高智把关」的默认规则;成本与质量全靠人肉每次指定 → 差距 3 |
| 4 | `claude -p` 子进程跑在 server 自身 cwd(未设 `cmd.Dir`),不是任务 workspace | claude 的 agent 文件操作落在错误目录(代理漂移)→ 差距 4 |
| 5 | 治理链齐备:审批门/熔断(conflict≥3)/账本/审计/父子任务同步聚合,Go 语义冻结 | 这是**资产**,Phase 8 不动它,只在它下面换「智能产生」层 → 保持不变 |

## 三、目标架构(方向)

```
┌──────────────────────── Go 回合骨架(不动:审批/熔断/账本/审计/父子聚合)────────────────────────┐
│                                                                                              │
│  工程任务认领 → [planner 拆解] → 回合循环: [writer 产出] → [test 验证] → [review 把关]          │
│     每个阶段 = 一次「模型原生 tool-call 回合」:                                                │
│       Go 组 prompt/messages + 注入可用工具集 → 模型返回 text 或 tool_calls                     │
│       → Go 在任务 workspace 里执行工具 → 结果回填模型 → 直到阶段收敛信号(结构化)→ 下一阶段      │
│                                                                                              │
│   工具集(执行归 Go,权限沿用既有 policy):workspace 文件读写 / shell(测试执行)/ git /             │
│     apply_diff(把模型产出落成真实改动)… 以及一个特殊工具:claude(Code) 委派                      │
└───────────────┬──────────────────────────────────────────────────────────────┬───────────────┘
                ▼                                                                ▼
    模型端点池(OpenAI 兼容网关 · 多档)                                claude(Code agent)= 一个可调工具
    同一自建网关、单 key,按所选模型建多档端点                        (回合中模型可把有界子任务委派给它,
    每端点带 tier: frontier|standard|cheap                           在任务 workspace 内自主干活,结果回传;
    role 语义保留 pool|planner|standby                               修订 A 保留,独立于网关模型调用)
```

**角色 → 档位默认映射(定稿 2026-09-04;显式端点永远覆盖默认)**:

| 角色 | 干什么 | 默认档 | 说明 |
|---|---|---|---|
| planner | 拆分/方案(高智) | `frontier` | 「高智模型拆分」 |
| writer | 批量产出代码/改动(普通) | `cheap` | 「普通模型干活」;不够再上提 |
| test | 跑测试 + 判读 | `standard` | 执行是真工具,判读便宜档够 |
| review | 把关/熔断裁决(高智) | `frontier` | 评审门兜底 writer 的质量 |

显式指定(建单 `--writer-endpoint`/`--reviewer-endpoint` 或 planner 端点)永远覆盖默认 —— 默认只解决「不给就合理」。

## 四、方向决策(定稿点,每条含取舍)

- **D1 Provider 契约升级**(修订 A):`Generate(prompt)→text` ⇒ `Chat(messages, tools) → (text | tool_calls…)`,
  多轮、原生工具调用。**不再按厂商写原生实现**:OS 只实现一个 **OpenAI Chat Completions 兼容 HTTP 客户端**,
  对接用户自建网关(单 key;function calling 由网关透传底层模型);`scripted` 保留为确定性档(工具语义下仍可复现);
  现有 Generate 调用方经薄适配保留(标废弃),不炸 0–7 语义。
- **D2 阶段工具化**:回合内模型可发起工具调用,Go 在任务 workspace 执行并回填,循环至结构化收敛
  (apply 落盘 / TEST 判读 / VERDICT 裁决 / plan 拆解);工作区有路径即真执行,无路径保持纯产出(diff)向后兼容。
- **D3 档位落库与默认分层**:endpoint 增 `tier`;角色默认映射(§三表)在建单解析端点时生效;控制台/CLI 可见端点 tier 与角色默认。
- **D4 claude 降为工具**:池中可注册 vendor=claude 的「agent 工具」端点;回合中模型(或 planner)可把**有界子任务**
  (单条 file/单次 review)委派给 `claude`,在任务 workspace 内运行、结果回传回合 —— claude 不再隐式是全系统唯一后端。
- **D5 Go 骨架不变**:审批门/熔断/账本/审计/父子聚合语义逐字保留;只替换「一次智能产出如何产生」。
- **D6 安全/边界延续**:工具执行沿用宿主 workspace 边界 + 既有 permission 校验,不因 tool-use 扩大模型权限面。

## 五、边界 / 不做

- 不改动已冻结 Phase 0–7 的契约与语义(回合/端点/审批/API 字段逐字保留;迁移只增不改)。
- 不做:UI 新页面(端点池增 tier 字段展示即可)、流式输出、运行中成本仪表盘(可后议)、RBAC/多用户。
- 不做:把任意第三方模型当「可写任意文件的 agent」放养 —— 模型只通过 OS 暴露的工具动工作区,永远不直通 shell 之外。

## 六、子阶段拆分(每子阶段独立可验收)

| 子阶段 | 交付物 | 验证 |
|---|---|---|
| 8.1 Provider 抽象升级 | messages+tools 契约(OpenAI Chat Completions 方言);自建网关 HTTP 实现(单 key、工具透传);claude CLI 标废弃保留;scripted 保真 | httptest:网关请求/响应形状 + tool_calls 回传;`go test ./...` 绿 |
| 8.2 阶段工具化首个闭环 | writer/test 其中一个阶段先跑通「模型调工具→Go 执行→回填→收敛」真实 loop(fake OpenAI 兼容网关服务器驱动) | 单阶段 tool 回合 httptest;scripted 回归不变 |
| 8.3 全阶段工具化 | planner/writer/test/review 全部切原生 tool-call,结构化收敛替换正则抠取 | 离线真实 server 冒烟(scripted 下跑通整条);熔断/审批语义回归 |
| 8.4 档位分层强制 | endpoint.tier(迁移 0010)+ 角色默认解析 + CLI/控制台可见 | 建单不指定端点 → 按默认档落到对应端点;显式覆盖仍生效 |
| 8.5 claude 委派工具 | vendor=claude agent 工具(workspace 内子代理,结果回传)+ 边界 | 冒烟:回合内委派一次 claude 完成小活并回传;live 真实 key 验收归用户 |

## 七、风险与取舍

- **多厂商质量参差** → writer 默认 cheap 由 frontier review 把关,劣质产出在评审门被熔断,不会静默合入。
- **成本不可控** → 档位默认 + 回合熔断已有;需要时加「预算上限」后续立项(本阶段只做默认分层)。
- **旧 claude 文本路径** → scripted 保留;legacy 文本补全标废弃、8.x 内不删,留迁移窗口。
- **代理漂移(工具跑错目录)** → 所有工具调用(含 claude 委派)cwd 强制绑定任务 workspace(现状 4 的修复)。
- **tool-use 回合发散** → 每阶段套既有 round/conflict 上限与熔断,不新增无限循环面。

## 八、验收口径

1. `go build ./...`、`go test ./...` 全绿;0–7 既有测试(scripted 冒烟/httptest)零改动通过。
2. 离线起 `os server`,控制台/CLI:端点池可从**网关模型目录**建多档端点(每档绑一个网关模型,proto=openai)并标注 tier;建工程请求不显式给端点时,按默认档自动落到对应端点(日志/审计可见所选模型)。
3. 回合内模型确实在调工具(审计/日志可见 tool 名与执行),收敛信号结构化,不再依赖 PATCH/VERDICT 正则。
4. claude 作为工具被委派一次有界子任务并在 workspace 内完成回传(live 需真实 key,归用户验收)。

## 九、关联与登记

- 关联:`docs/phase6/design/rd-capability.md`(端点池/回合机/planner)、`docs/phase7/design/console-ui.md`(端点池页)、
  方向基线 `One-Person-Company-OS-Design-v1.0.md`。
- 本文件已**定稿**并登记 `design/README.md`、挂 Phase 8 到 `进度总表.md`/`项目详解.md`;后续方向变更以 §十一 修订记录为准。

## 十、定稿记录(2026-09-04,定稿门通过)

1. **档位表达**:endpoint 增 `tier` 三档 `frontier|standard|cheap`,UI 中文「高智/均衡/经济」;
   与既有 `role`(pool/planner/standby)正交。✅
2. **角色默认映射**:认可 §三表 —— planner=frontier、writer=cheap、test=standard、review=frontier;
   显式指定(建单 `--writer-endpoint`/`--reviewer-endpoint` 或 planner 端点)永远覆盖默认。✅
3. **writer 产出**:任务带工作区 → 真落盘 + test 真执行;无工作区 → 仍返 diff(向后兼容)。✅
4. **子阶段切分**:认可 8.1–8.5 顺序(Provider 升级 → 单阶段 tool 闭环 → 全阶段 tool 化 →
   档位强制 → claude 委派工具),每子阶段独立可验收。✅

## 十一、修订记录

- **修订 A(2026-09-04,方向补充,已定稿)**:用户自建 OpenAI 兼容 API 网关(单 key,function calling 透传)。
  8.1 由「anthropic/openai 双原生 HTTP 实现」收敛为「单一 Chat Completions 兼容适配器对接网关」;
  「选模型」= 网关模型目录内按档/角色挑(目录拉取 `FetchEndpointModels` 6.1 已具备,建单默认落档属 8.4)。
  tier 档位机制与 claude Code agent 委派(8.5)原案**保留**,与网关模型调用是两条正交路径。
  受影响段落:§一 目标1、§三 架构图左栏与 claude 注、§四 D1、§六 8.1/8.2、§八 验收 2。
  8.1 实施契约见 [provider-upgrade.md](provider-upgrade.md)。
