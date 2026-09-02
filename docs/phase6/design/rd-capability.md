# Phase 6 — 研发部(Engineering Capability)能力就位 设计

> 执行蓝本。Phase 6 把 DevLoop(双模型编码⇄评审闭环)作为**研发部**融入 OS:研发部 = engineering Capability 的部门视图,研发任务的数据、审批、审计、决策全部进单一 os.db;公司派活与 GitHub 自驱两路接活;研发部内部以"写→审→分歧→熔断"循环执行,出口统一回到公司治理。
> 状态:✅ 定稿(2026-09-02)
> 关联:[项目详解](../../项目详解.md) · [进度总表](../../进度总表.md) · 方向基线 `One-Person-Company-OS-Design-v1.0.md`(v1.1,§2.1 Capability 而非 Department / §4.7 Agent Runtime / §7 Phase 2)· [Phase 1 设计](../phase1/design/execution-kernel.md) · [Phase 2 设计](../phase2/design/engineering-capability.md)
> 前置:DevLoop 落地计划 v1.1(ricky97gr/tests `devloop-落地计划.md`),作为研发部执行引擎的来源设计

---

## 1. 背景与目标

0-5 期已交付:Company Kernel(0)、Execution Kernel(1)、Engineering Capability 框架(2,Provider 为 stub、aifix 为 mock)、Research/Product + 跨 Capability(3)、Business Capabilities(4)、Memory/Decision + 完整形态(5)。

OS 的 engineering Capability 现状:**框架先行**——agent 是角色、模型是 config 里声明的 stub、`aifix` tool 返回 mock、节点由 workflow definition 静态定义、执行是"worker 跑一次 tool"。它**不会真写代码、不会真评审**。

DevLoop 是同一个愿景下**研发部工作流的完整实现**:模型池动态接入 + 真调 claude、issue 分诊、planner 拆解、A→C→B 轮换、评审返工、3 轮熔断、PR 交付。它的形态是一个自治体(自带审批/日志/看板),直接并排跑会在公司里再造一个"公司"。

Phase 6 目标:

> 把 DevLoop 收编为研发部——不新增部门层级,而是把 engineering Capability 的"内核"升级为真实研发闭环;DevLoop 的自治权(发布/日志/模型)全部归口 OS 单一 os.db 的治理链。

**已确认方向(2026-09-02)**:① 研发部两路接活(公司派活 + GitHub 自驱);② 拆入单一 os.db(不保留 DevLoop 独立库);③ 回合在研发执行驱动内部(引擎不加回环状态机);④ planner 动态拆子任务(上限 8,超出 ask_human)。

---

## 2. 定位:研发部 = engineering Capability 的部门视图

方向基线 §2.1 定调:**Capability-Oriented,Department 不是底层架构**。因此不新增"部门"实体或第二套 engineering 能力域(那会双份打架),而是:

``` text
Company(单一 os.db)
  └─ Capability(组织维度)
       └─ engineering = 研发部 ★(部门只是视图标签,name 可取"研发部")
            ├─ agents:architect(拆解/规划)/ coding(写)/ qa(测)/ review(审)
            └─ 执行内核:Phase 6 升级为 DevLoop 循环引擎
```

**不变量(0-5 冻结)**:`Company → Capability → Agent`、`Workflow → Task → Execution → Tool`、治理链 `Policy → Permission → Approval → Audit → Decision`、任务状态机、workflow 顺序编排、单二进制 + SQLite。Phase 6 只在**工程域内部**加引擎、加通道,不改这些冻结语义。

---

## 3. 范围与边界

**做**:
- 研发执行驱动(Engineering Driver):一条 engineering Task 内部的"写→测→审→返工→熔断"回合循环,回合落 execution
- 模型池落库(endpoints)+ Model Provider 真实现(claude CLI 子进程),替换 stub 与 aifix mock
- 研发 Intake:GitHub issue 轮询/webhook + 分诊 + planner 拆解;公司 workflow 的 engineering 节点下发接活
- 服务化通道:OS 首次 HTTP server(chi,webhook 入口)+ cron 定时 + 飞书通知
- git tool 网络操作解禁(挂审批门)

**明确不做**:
- 改已冻结引擎语义(任务状态机、workflow 顺序编排、approval/audit 触发路径)——见 §6.2"执行家族分发"的加分支不改造方式
- 保留 DevLoop 独立库/独立 approvals/events 表/独立看板数据源
- A→C→B 模型轮换的**异厂商强校验**落地(先把单任务回合跑通,轮换/交叉校验作为研发域 policy 后续叠加)
- React 看板 UI、Metrics/Tracing(独立排期,同 Phase 4/5 决策)
- Memory 接 LLM/RAG(沿用基线 §4.10)
- 真实资金/生产发布自动合并(仍以人工审批为准)

---

## 4. 架构总览

``` text
                         Company OS(单一 os.db)
  治理链: Policy → Permission → Approval → Audit → Decision → Memory(全公司统一)
        │
  接活 ── 通道A:公司 workflow 的 engineering/qa 节点(带 PRD)────┐
  接活 ── 通道B:GitHub issue 轮询/webhook → 分诊 → 排队 ────────┤
        │                                                    ▼
  研发部 = engineering Capability
        │  ┌────────────────────────────────────────────────┐
        │  │ 研发 Intake(新服务,常驻)                           │
        │  │   triage 分诊:合并批次 / 直接开工 / 追问 / 跳过       │
        │  │   planner 拆解:大活拆 ≤8 条真实子任务,超出 ask_human  │
        │  └───────────────────────┬────────────────────────┘
        │                          ▼ create N 条 engineering task(入队)
        │  ┌────────────────────────────────────────────────┐
        │  │ 研发执行驱动 Engineering Driver(新执行路径)        │
        │  │   单条 task 内: writer 写 → test 测 → review 审    │
        │  │   test 失败=免费返工(不计 conflict)               │
        │  │   review 驳回 → conflict+1 → 同任务返工(≤3 轮)     │
        │  │   达 3 轮 → 熔断 → 转 waiting_approval(复用审批)   │
        │  │   每回合 = 一条 execution                          │
        │  └───────────────────────┬────────────────────────┘
        │                          ▼
  交活 ── 合并/交付 + 结果回写(execution / approval→decision / memory)
        │                          ▼
                            os overview / 看板 / 飞书
```

执行链上的关键约定(对齐现有实现):

- **现有 worker 路径(`queue work` / `task run`)逐条消费 READY Task,对每条 Task 走一次 tool 执行**——engineering 任务需要多回合,因此**单独一条执行驱动**,复用 store 原语(CreateExecution / CompleteTask / RequeueTask / FailTask / RequestApproval),不改 `runClaimed` 的既有语义。
- **熔断 = 现有 approval**:回合数 ≥3 时任务置 risk=high 语义走 `requestApproval` → `waiting_approval`,人工 approve 后放行、reject/changes 后 failed——审批结果照旧自动落 decision(治理链闭环,免费获得)。
- **子任务 = 真实 task 行**:planner 拆出的子项 create 成独立 task(带 `parent_task_id`),各自排队、各自有 execution/approval/audit,公司 overview 可见。

---

## 5. 领域模型扩展

### 5.1 Task 扩展(对齐真实字段:tool_name/status/qstatus/attempt/risk/workspace_path)

| 新增字段 | 说明 |
|---|---|
| `parent_task_id TEXT` | 拆解出的子任务挂父请求(通道 B/大活拆解);空 = 顶层任务 |
| `round_no INTEGER DEFAULT 0` | 当前评审回合(回合在驱动内部,不另建状态机) |
| `conflict_count INTEGER DEFAULT 0` | reviewer 驳回累计;test 失败不累计 |
| `writer_endpoint_id TEXT` / `reviewer_endpoint_id TEXT` | 本条任务的写/审模型端点;空 = 走 agent 默认(Phase 6 先单端点跑通) |

### 5.2 新增表

| 表 | 关键字段 | 说明 |
|---|---|---|
| `endpoints` | id, company_id, name, base_url, token_enc, proto(anthropic/openai/auto), vendor, selected_model, role(pool/planner/standby), status, models_cache(json), created_at | 模型池落库(DevLoop 模型接入);Token 加密存储(aesgcm,key 自 env),不落明文 |
| `repos` | id, company_id, name, repo_url, workspace_path, created_at | 研发仓库登记:issue 归属 → workspace;git/file 边界沿 workspace 强制 |

- 迁移沿用 0-5 自研 runner,自动应用:计划 `0007_endpoints` / `0008_task_round` / `0009_repo`。
- Task 写路径仍经 service 层自动落 Audit(0-5 不变式)。

---

## 6. 研发域两个新组件

### 6.1 研发 Intake(接活 + 拆解)

职责:把"活"变成队列里的 engineering task。

- **通道 A(公司派活)**:公司 workflow(如新品发布)的 engineering 节点,description 即研发请求(PRD)。节点执行时若命中研发请求格式,Intake 把整包请求交给 planner。
- **通道 B(GitHub 自驱)**:`os server` 常驻轮询 + webhook(双通道,webhook 优先、轮询兜底)。每 issue 一次模型调用做分诊(triage),四种处置:合并批次 / 直接开工 / 追问(issue 下评论提问)/ 跳过(附理由,可转孵化)。
- **planner 拆解**:请求信息充分 → 模型拆成 ≤8 条子任务,每条 create 一条 engineering task(挂 `parent_task_id`,按 (capability=engineering, role=coding) 解析 agent);拆解上限 8,**超出必须 ask_human**(走 approval),引擎不提供绕过开关。

实现边界:Intake 是 service 层新文件(读 store、写 task),不需要审批/审计之外的新引擎能力。

### 6.2 研发执行驱动 Engineering Driver(回合循环)

**为什么不能是普通 tool**:现有 `Tool.Execute(ctx, task)` 是单发契约,拿不到 store/service,无法驱动多回合。因此 engineering task 走**新增的执行驱动分支**:

- worker 领取 Task 后按 `tool_name` 家族分发——新增 tool 名 `engineering`(或其下 `eng-writer` / `eng-review` / `eng-test`)。命中工程家族 → 交给 Engineering Driver;其余(0-5 全部)仍走 `runClaimed` 现路径,**行为不变**。这是加分支不改语义,不是引擎改造。

Driver 对单条 engineering task 的循环:

``` text
round=0
writer:调 claude(写者端点)产出 patch/diff → 落 execution(result=diff)
test:  qa 角色跑测试
       ├─ test 失败 → 免费返工:同 writer 端点重写,round_no 不变,conflict 不累计(devloop:"测试失败 ≠ 评审分歧")
       └─ test 通过 → review:审阅端点审 diff,产出结构化 verdict
              ├─ approve → 任务完成(按 risk 决定是否需审批门:merge/交付 high → 现有审批)
              └─ needs_changes → conflict_count+1
                     ├─ conflict_count < 3 → 同任务返工(写/审模型对不变)
                     └─ conflict_count ≥ 3 → 熔断 → RequestApproval(task 转 waiting_approval,人工决定)
```

- 每回合 = 一条 `execution` 行;round_no / conflict_count 落 task 行(§5.1),供 overview 与回放。
- 超时/重试沿用现有 attempt/timeout 语义;claude CLI 输出解析做容错 + 版本锁定,解析失败置任务 error 而非静默重试。
- 模型调用走 §7 的 Model Provider 真实现;agent 的 role(coding/qa/review)映射到写/测/审三个动作,端点由 agent 的 model_hint/endpoints 解析。

---

## 7. 模型池与 Model Provider 真实现

- 现状:`config/os.yaml providers` 仅声明、`provider.Stub.Generate` 返回 mock(0-5 冻结框架不动)。
- Phase 6:**endpoints 落库为事实源**,Provider 增加 `ClaudeCLIProvider`(实现现有 `Generate(ctx, req)` 接口)——以 `claude -p` 子进程调用,三元组环境变量注入:

``` bash
ANTHROPIC_BASE_URL=<endpoint.base_url>
ANTHROPIC_AUTH_TOKEN=<endpoint.token 解密>
ANTHROPIC_MODEL=<endpoint.selected_model>
```

- 换模型 = 换 endpoint / 换 selected_model,零改码;拉模型列表走 `GET {base_url}/v1/models`(anthropic 头 x-api-key+anthropic-version,openai 头 Authorization,auto 双试),结果缓存 models_cache。
- 安全:token 只存加密列,密钥从 env(`OS_ENDPOINT_KEY`)读;runner 内不入日志。
- aifix tool(mock)保留向后兼容,真实入口以 `engineering` 工具族为准。

---

## 8. 服务化通道(OS 首次常驻)

| 通道 | 形态 | 说明 |
|---|---|---|
| HTTP | `os server [--port]`(chi,方向基线 4.2 已定,0-5 未实现) | webhook 入口 `/api/webhook/github`;健康检查;常驻进程 |
| 定时 | 轻量调度(首版单协程 tick,后续可换 robfig/cron) | GitHub 轮询(≥5 分钟兜底)、例行回归、每日摘要 9:00 |
| 通知 | 飞书机器人 webhook(新增 `internal/notify`) | 熔断告警(即时)/ 每日摘要 / 待审批提醒 |
| 背景运行 | workflow run 支持后台 + 前台 approve(0-5 已支持) | server 模式沿用 |

- `os server` 是首个长驻命令;CLI 其余命令形态不变。新增依赖:chi、robfig/cron(进 go.mod)。

---

## 9. CLI 契约(Phase 6 新增)

| 命令 | 说明 |
|---|---|
| `os endpoint add --company <id> --name <n> --base-url <u> --token <t> [--proto auto]` | 接入模型端点(token 加密落库) |
| `os endpoint list --company <id>` / `os endpoint show <id>` | 查看(不显示明文 token) |
| `os endpoint models <id>` | 测试连接 + 拉取 /v1/models 列表 |
| `os endpoint select <id> --model <m> [--role pool]` | 选定模型/角色 |
| `os repo add --company <id> --name <n> --repo-url <u> --workspace <dir>` | 登记研发仓库 |
| `os repo list --company <id>` | 查看仓库 → workspace 映射 |
| `os server [--port 8080]` | 常驻:HTTP + webhook + 轮询 + cron(研发部通道) |
| `os intake sync` | 单次手动同步(轮询兜底,server 内部亦调用) |
| `os task create ... --tool engineering` / workflow 节点 tool=engineering | 研发任务走 Engineering Driver |

通用约定沿用 0-5(对齐文本表格、错误非 0 退出码、`--config`/`--db` 覆盖)。新增命令经 `internal/cli` 注册,`root.go` 挂载。

---

## 10. 子阶段拆分与验收标准(每子阶段一个可独立验收交付物)

| 子阶段 | 交付物 | 验收方式 |
|---|---|---|
| **6.1 模型池落库 + Provider 真实现** | migration 0007(endpoints)+ ClaudeCLIProvider(`claude -p`)+ endpoint CLI | 接入 ≥2 异厂商端点;`os endpoint models` 拉取非空;真实提问返回合法 JSON;token 库内为密文 |
| **6.2 回合驱动 + 熔断** | migration 0008(task 扩展)+ Engineering Driver 循环 + `engineering` 工具族 | 一条真实 bugfix 任务:写→测→审通过→完成;构造 review 驳回 → conflict+1 同任务返工;达 3 轮 → 转 waiting_approval,approve 续跑且 decision 自动落库 |
| **6.3 服务化通道** | migration 0009(repos)+ `os server`(chi/webhook/cron)+ GitHub issue 同步 + triage 分诊 | 建一个测试 issue,≤5 分钟带分诊结论出现在研发队列;webhook 实时优先 |
| **6.4 planner 拆解 + 两路接活** | Intake 拆解(≤8,超 ask_human)+ 公司 workflow engineering 节点接活 | 新品发布 workflow 跑到 engineering 节点,planner 拆出 ≤8 子任务全部跑完;channel B issue 亦拆解 |
| **6.5 运营视图 + 触达** | overview 聚合研发部状态 + 飞书(cron 摘要/告警/待审批提醒) | overview 可见研发部 pending/熔断/待审批;熔断 ≤1 分钟飞书告警;9:00 收到昨日摘要 |

每子阶段完成后写 `docs/phase6/stages/<n>.md` 归档、更新进度总表;分支按 `dev → phase-6-rd-capability` 迭代,子阶段结束合回 dev。

---

## 11. 已确认决策(2026-09-02)

- ✅ **研发部 = engineering Capability 部门视图**,不新增部门实体/第二套 engineering 能力域(基线 §2.1)。
- ✅ **两路并收**:公司 workflow 下发(通道 A)+ GitHub 自驱(通道 B),共享同一研发执行驱动。
- ✅ **拆入单一 os.db**:研发任务/回合/审批/审计/decision 全进公司库;不复用 DevLoop 的 approvals/events/独立 DB;熔断、生产发布走现有 approval 与自动 decision。
- ✅ **回合在研发执行驱动内部**:引擎不加回环状态机;round_no/conflict_count 落 task 行,每回合一条 execution。
- ✅ **planner 动态拆子任务**:≤8 拆成真实 task 队列;超出 ask_human(引擎不提供绕过开关)。
- ✅ **worker 加执行家族分发、不改既有语义**:engineering 走 Driver,0-5 全部 tool 路径行为不变。
- ✅ **治理链闭环免费继承**:熔断/审批 → decision 自动落库、audit 全程、memory 沉淀沿用 5.1/5.2。

## 12. 待定事项(定稿前拍板)

1. **真实模型成本与 key**:6.1/6.2 验收需要真实 API key 与 claude CLI 可用;本地 mock 是否保留一键切换(`--driver mock|real`)以支持无 key 冒烟?倾向:保留 mock,验收用 real。
2. **异厂商轮换(A→C→B)与交叉校验**:本次设计只跑通单任务写/审(同端点或两端点);轮换铁律是否在同子阶段内落地,还是 6.2 之后作为 engineering 域 policy 叠加?
3. **回合驱动与现有 worker 的并发**:Engineering Driver 驱动的工程任务由谁领取(专用 worker `os eng work` 还是分发进 `queue work`)?倾向专用驱动,避免与现 worker 竞争单写锁。
4. **server 常驻的最低范围**:6.3 的 GitHub 通道是否必须要真实 token 与公网 webhook(HTTPS)?若暂无,先以轮询 + `os intake sync` 验收。
