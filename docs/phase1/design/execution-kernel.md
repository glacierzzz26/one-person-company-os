# Phase 1 — Execution Kernel 设计

> 执行蓝本。定义 Phase 1 的领域模型扩展、执行引擎、Tool/Workspace/Sandbox、Workflow 编排与治理链(Approval/Permission 校验),以及 CLI 契约。
> 状态:✅ 定稿(2026-09-01)
> 关联:[项目详解](../项目详解.md) · [进度总表](../../进度总表.md) · 方向基线 `One-Person-Company-OS-Design-v1.0.md`(v1.1)

## 1. 背景与目标

Phase 0 建立了核心模型与持久化(Task 创建后固定 `pending`,仅存储,不执行)。Phase 1 的目标:

> OS 可以真正让 Agent 在受控环境中执行任务。

即:任务被**领取 → 执行 → 流转 → 记录**,高风险行为进入**人工审批**,执行全程可审计,Workflow 能把一组任务编排起来自动推进。

## 2. 范围与边界

**做**(方向基线 Phase 1 交付物):Task Queue、Workflow Engine、Agent Runtime、Tool、Workspace、Sandbox、Approval、Execution。

**明确不做**(留到后续):具体业务 Agent(Phase 2 Engineering Capability)、模型接入(model_hint 仅存储)、Memory/Decision 实体、跨 Capability 业务流程(Phase 3)、Permission 对 Runtime 宿主机级强制(超出应用层可控范围,依赖 Sandbox 形态)。

## 3. 子阶段拆分(每子阶段一个可独立验收交付物)

| 子阶段 | 交付物 | 验收方式 |
|---|---|---|
| **1.1 Execution Core** | Task Queue + Task 状态机 + Execution 实体 + Worker 循环 + Runtime 接口 | `os queue work` 消费 pending task → running → completed/failed,Execution 落库,attempt/retry/timeout/lease 生效 |
| **1.2 Tool + Workspace + Sandbox** | 首个真实 Tool(Shell)+ Task 独立 Workspace + Docker Sandbox | Task 在隔离 Workspace 内通过 Tool 产生真实结果;Tool 调用落 Audit;未授权 Tool 被拒 |
| **1.3 Workflow Engine** | 轻量编排:workflow definition 解析为有序节点,自动生成/推进 Task | 定义多阶段 workflow,一键执行按序跑完各节点 Task |
| **1.4 Approval + Permission 校验** | 治理链接入执行路径:high-risk → waiting_approval;Tool 调用前 permission 校验 | high-risk Task 未经人工批准不进入 running;approve/reject/request changes 生效 |

每子阶段完成后写 `stages/<n>.md` 归档并更新进度总表;分支按 `dev → phase-1-execution-kernel` 迭代,阶段结束合回。

## 4. 领域模型扩展

### 4.1 Task 状态机(对齐方向基线 4.5)

``` text
pending ──▶ running ──▶ completed
              │  ▲
              │  └── failed ──▶ (retry → running)
              ▼
      waiting_approval ──▶ running(批准) / failed(拒绝)
```

- Phase 0 的 `pending` 保留;新增流转命令/worker 驱动。
- `failed` 后按 `attempt < max_attempts` 决定 retry 回 `running` 还是终态 `failed`。
- high-risk(或命中 policy)的 Task 执行前先入 `waiting_approval`。

### 4.2 新增实体

| 实体 | 字段 | 说明 |
|---|---|---|
| **Execution** | id, task_id→task, worker_id, attempt, status(running\|completed\|failed\|timeout), started_at, finished_at?, result?, error?, created_at | 一次实际执行过程;Task 每次(重)执行一条 |
| **Approval** | id, task_id→task, risk, reason, status(pending\|approved\|rejected\|changes), requested_by, decided_by?, decision_note?, created_at, decided_at? | 人工审批节点;只对进入审批的 Task 产生 |
| **Workspace** | task_id 关联,Tool 在其内运行(方向基线 4.9 `Task → Workspace → Sandbox → Agent → Tools`);Phase 1 以 Task 独立目录实现,目录归属可追溯 | 不建独立实体表,以 task 的 `workspace_path` 表达 |

### 4.3 Task 字段扩展

在 Phase 0 Task 基础上新增:

``` text
max_attempts   int   默认 1
timeout_sec    int   默认 0(不限);>0 时 worker 超时杀进程
last_error     text  最近一次失败原因
result         text  最近一次执行结果(摘要)
workspace_path text  Task 工作目录
```

## 5. 存储设计

- 迁移:`internal/storage/migrations/0002_execution.up.sql` / `.down.sql`(沿用 Phase 0 自研迁移 runner)。
- 新表:`execution`、`approval`。
- `task` 增加列:`max_attempts`、`timeout_sec`、`last_error`、`result`、`workspace_path`。
- 索引:`idx_execution_task`(execution.task_id)、`idx_approval_task`(approval.task_id)。
- 分层不变:`Domain → Repository 接口 → SQLC → SQLite`;写操作统一经 service 层自动落 Audit。

## 6. 执行引擎(Task Queue + Worker)

对齐方向基线 4.6 SQLite-backed Queue:

``` text
READY ──▶ LEASED ──▶ RUNNING ──▶ COMPLETED / FAILED
```

- **队列状态**独立于 Task 状态:READY(可领取)→ LEASED(worker 领取,租约中)→ RUNNING(真正执行)→ COMPLETED/FAILED。
- **Worker 循环**:`os queue work --worker <id> [--limit <n>]` 启动一个 worker:领取 READY task(按 priority desc, created_at asc)→ 置 LEASED → 调 Runtime 执行 → 按结果置 COMPLETED/FAILED。
- **Lease / Timeout**:领取时写租约(worker_id + lease_until);执行超时(if timeout_sec>0)判 FAILED(timeout)。
- **Attempt / Retry**:FAILED 且 attempt < max_attempts → attempt+1 回 READY 重试;否则终态 FAILED。
- **Recovery**:启动时把孤儿 LEASED(lease_until 过期且无对应 worker 心跳)回收为 READY。
- **并发安全**:沿用 SQLite 单写者(`SetMaxOpenConns(1)`),领取走 `UPDATE ... WHERE status='READY' RETURNING *` 原子占用。

## 7. Agent Runtime + Tool + Workspace + Sandbox

### 7.1 Runtime 接口(方向基线 4.7)

``` go
type Runtime interface {
    Execute(ctx context.Context, task task.Task) (Result, error)
}
```

- 第一个实现:**Workspace Shell Runtime**——在 Task 独立 `workspace_path` 下执行命令,受 timeout/资源限制,收集 stdout/stderr 作为 result。
- 架构上保留替换点(方向基线:"第一阶段可以只实现一个 Runtime,但架构必须允许替换"),`internal/runtime/` 为接口,`runtimes/` 放实现。

### 7.2 Tool(方向基线 4.8)

每个 Tool 必须有:`Definition`、`Permission`、`Input Schema`、`Output Schema`、`Risk Level`、`Audit`。

- **Tool 接口 + Registry(可热插拔)**:Tool 以接口定义,经 `internal/tool` 的 Registry 按名字注册/查询。执行引擎只依赖 Registry,不感知具体 Tool——新增/替换 Tool = 实现接口 + 注册,不改执行链路(起步用内置注册表,架构允许运行时动态注册,满足后续热插拔)。
- Phase 1 首个 Tool:**Shell**——在容器 Workspace 内执行命令,调用前检查 Permission(`action=execute`),调用自动落 Audit。
- CLI `os tool list` 读取注册表,展示各 Tool 的 permission/risk/input/output 元数据。

### 7.3 Sandbox(方向基线 4.9)

- 目标形态(硬性要求):Docker 隔离(Filesystem/Network/CPU/Memory/Timeout/Host/Secret),方向基线指定 Docker 为第一阶段 Sandbox;本机已验证 Docker 29.1.3 可用。
- **必须 Docker,不做非 Docker 降级**:1.2 在 Docker 容器内运行 Task Workspace,实现 Sandbox 控制项(Filesystem/Network/CPU/Memory/Timeout/Host/Secret)。

## 8. Workflow Engine(轻量编排)

- 方向基线:自研轻量 State Machine,管理跨阶段/跨 Capability 流程;Task 是执行单元(4.5),暂不使用 Temporal。
- **Definition**:`workflow.definition`(JSON)声明有序节点:`[{ "step": "research", "title": "...", "agent_role": "architect", "risk": "medium" }, ...]`。
- **执行**:`os workflow run <id>` → 引擎解析 definition → 为每个节点创建 Task(挂 `workflow_id`)→ 交给 6 节执行引擎按序执行 → 节点失败默认中断(可配置)。
- 节点间不并行(Phase 1 顺序执行),跨 Capability 编排语义(Phase 3)预留接口。

## 9. 治理链接入执行路径

### 9.1 Approval(方向基线 6.3)

- **触发**:worker 领取的 Task 若 `risk=high`,或命中 policy 的审批规则(`statement` 声明需审批的 action/resource)→ 置 `waiting_approval` 并创建 Approval 记录,**不进入 running**,等待人工。1.4 范围含两者。
- **人工操作**(CLI):`os approval approve/reject/changes <id> [--note <n>]`。
  - approve → Task 回 `running` 继续执行;
  - reject / changes → Task 置 `failed`(changes 带备注,可重试)。
- Agent 不得伪造 Approval:Approval 状态仅由人工命令(actor=`human:cli`)变更,worker/工具路径无法触碰。

### 9.2 Permission 校验(最小权限)

- Tool 调用前校验:subject(Agent role)→ permission(action, resource)匹配则放行,否则拒绝并落 Audit。
- 系统核心约束(改 Policy / 改 Permission / 删 Audit / 绕过 Approval)由应用层拦截:涉及 `policy`、`permission`、`audit` 的写操作只暴露受控 CLI 命令,不暴露给 Tool/Runtime 通用通道。

## 10. CLI 命令契约(在 Phase 0 之上新增)

| 命令 | 说明 |
|---|---|
| `os queue work --worker <id> [--limit <n>]` | 启动 worker,消费 READY Task(前台运行) |
| `os task run <id> [--nowait]` | 单任务手动执行(等价一次 worker 领取该任务) |
| `os execution list [--task <id>] [--status <s>]` / `os execution show <id>` | 查询执行记录 |
| `os workflow run <id>` | 执行 workflow definition,按序生成并推进节点 Task |
| `os approval list [--status <s>]` / `os approval show <id>` | 查询审批请求 |
| `os approval approve\|reject\|changes <id> [--note <n>]` | 人工审批(actor=human:cli) |
| `os tool list` | 查看已注册 Tool(元数据:permission/risk/input/output) |
| `os task list`(增强) | 增加 `--risk`、`--attempt` 过滤;展示 attempt/result |

通用约定沿用 Phase 0(对齐文本表格、错误非 0 退出码、`config/os.yaml` + `--db` 覆盖)。

## 11. 验收标准(按子阶段)

**1.1**
1. `os task create` 后 `os queue work` 消费 → status 经 running 到 completed;Execution 落库。
2. `max_attempts>1` 且任务失败 → 自动重试 attempt+1,达上限终态 failed。
3. 设置 `timeout_sec` 的 task 超时 → failed(timeout)。
4. 两个 worker 并发领取 → 无任务被重复领取(原子占用);孤儿 LEASED 可被 recovery 回收。

**1.2**
5. Task 在独立 `workspace_path` 执行 Shell Tool,result 反映真实输出;`os tool list` 与 Tool 注册表一致。
6. 未授权 Tool 调用被拒并落 Audit。

**1.3**
7. `os workflow run` 按 definition 顺序生成并完成各节点 Task;中途失败默认中断。

**1.4**
8. high-risk Task 入 `waiting_approval` 不执行;approve 后放行,reject 后 failed。
9. Tool 调用前 permission 校验生效;涉及核心约束的写操作仅受控 CLI 可触达。

## 12. 已确认决策(2026-09-01 定稿)

- ✅ 子阶段拆分与顺序(3 节):认可四子阶段(1.1-1.4)。
- ✅ Tool 采用接口 + Registry,可热插拔:新增/替换 Tool 不改执行引擎(7.2)。
- ✅ 首个 Tool 用 Shell(7.2,起步实现,架构可替换)。
- ✅ Sandbox 必须 Docker,本机已验证可用(7.3),不做非 Docker 降级。
- ✅ 触发审批判定:`risk=high` OR 命中 policy 审批规则(9.1,1.4 范围)。
