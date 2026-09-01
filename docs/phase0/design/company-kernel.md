# Phase 0 — Company Kernel 设计

> 执行蓝本。定义 Phase 0 的领域模型、SQLite 存储与 CLI 契约。
> 状态：✅ 定稿（2026-09-01）
> 关联：[项目详解](../项目详解.md) · [进度总表](../../进度总表.md) · 方向基线 `One-Person-Company-OS-Design-v1.0.md`(v1.1)

## 1. 背景与目标

Phase 0 建立"公司操作系统"的核心模型地基：Company / Capability / Agent / Policy / Permission / Workflow / Task / Audit 的**领域定义与持久化**，通过 CLI 可创建与读取。

> 先把核心模型建立起来，而不是先写具体业务 Agent（方向基线 Phase 0 目标）。

## 2. 范围与边界

**做**：领域类型、SQLite schema、SQLC 查询、Repository 接口与实现、CLI CRUD、写操作自动落 Audit。

**明确不做**（留到 Phase 1）：Task Queue、Workflow 引擎编排执行、Agent Runtime、Tool、Workspace/Sandbox、Approval 流程、模型接入、Memory/Decision 实体。

## 3. 领域模型（遵循 v1.1）

> 关键：Agent 是 Capability 中的**执行角色**；Workflow 是**跨 Capability 的流程编排**，不被单个 Agent 拥有；Task 是 Workflow 的执行单元（Phase 0 仅定义与存储，编排执行在 Phase 1）。

| 实体 | 字段 | 约束 |
|---|---|---|
| **Company** | id, name, vision?, created_at, updated_at | id 为主键 |
| **Capability** | id, company_id→company, code, name, description?, created_at, updated_at | UNIQUE(company_id, code) |
| **Agent** | id, capability_id→capability, name, role, model_hint?, created_at, updated_at | role ∈ {architect, coding, qa, review} 起步 |
| **Policy** | id, company_id→company, name, kind, statement, enabled, created_at, updated_at | kind ∈ {allow, deny} |
| **Permission** | id, policy_id→policy, subject, action, resource, created_at | subject 为 role/agent_id；action ∈ {read, write, execute, network, secret, production, admin} |
| **Workflow** | id, company_id→company, name, description?, definition?, created_at, updated_at | definition 为 JSON 文本，Phase 0 仅存储 |
| **Task** | id, company_id→company, capability_id?, workflow_id?, agent_id?, title, description?, status, priority, attempt, risk, created_at, updated_at | status ∈ {pending, running, waiting_approval, completed, failed}，**创建时固定 pending**（流转归 Phase 1）；risk ∈ {low, medium, high} |
| **Audit** | id, entity_type, entity_id, action, actor, detail?, created_at | **append-only**，应用层禁止 update/delete |

**幂等与审计**：所有写操作（create/update）经由 service 层，自动写入一条 Audit 记录（entity_type/entity_id/action/actor）。Phase 0 的 Audit 由应用层保证 append-only。

## 4. 存储设计（SQLite + SQLC + golang-migrate）

- 迁移文件：`migrations/0001_init.up.sql` / `.down.sql`
- 表：`company`, `capability`, `agent`, `policy`, `permission`, `workflow`, `task`, `audit`（单数表名）
- 主键：`TEXT`（UUID v4 由应用生成），外键显式声明
- SQLC 生成 `internal/storage/query/`（queries 按实体分文件），Repository 接口在 `internal/storage/repository/`
- 时间戳：SQLite `TEXT`（RFC3339）或 `INTEGER`（unix epoch）——采用 `INTEGER`(unix) 便于排序

## 5. CLI 命令契约（Cobra，`cmd/os`）

| 命令 | 说明 |
|---|---|
| `os init` | 应用 migrations 初始化数据库 |
| `os company create --name <n> [--vision <v>]` | 创建公司 |
| `os company list` / `os company show <id>` | 查询公司 |
| `os capability add --company <id> --code <c> --name <n>` | 添加能力域 |
| `os agent add --capability <id> --name <n> --role <r>` | 添加执行 Agent |
| `os policy add --company <id> --name <n> --kind allow\|deny --statement <s>` | 添加 Policy |
| `os permission add --policy <id> --subject <role> --action <a> --resource <r>` | 添加 Permission |
| `os workflow add --company <id> --name <n> [--definition <json>]` | 添加 Workflow（仅存储） |
| `os task create --company <id> --title <t> [--capability <id>] [--workflow <id>] [--agent <id>] [--risk low]` | 创建 Task |
| `os task list [--status <s>] [--company <id>]` / `os task show <id>` | 查询 Task |
| `os audit list [--entity-type <t>]` | 查询审计 |

通用约定：输出为对齐文本（CLI 表格）；错误退出码非 0 并打印到 stderr；数据库路径来自 `config/os.yaml`（默认 `./os.db`），可用 `--db` 覆盖。

## 6. 验收标准

1. `os init` 成功应用 migrations，生成 `os.db`
2. `os company create` → `os company list/show` 往返一致
3. `os policy add`、`os task create` 等写操作后，`os audit list` 可查到对应记录
4. `os task create`（指定 capability/workflow/agent 可选）→ `os task list --status pending` 可查
5. 无未应用迁移（`migrate` 状态干净）

## 7. 实现顺序

1. `go.mod` + 目录骨架（按方向基线附录 A）+ `config/os.yaml` 与配置加载
2. `migrations/0001_init.sql`（全部表）
3. SQLC 配置 + queries + `sqlc generate`
4. 领域类型 `internal/{company,capability,agent,policy,permission,workflow,task,audit}`
5. Repository 接口 + SQLC 实现
6. service 层（写操作自动落 Audit）
7. Cobra CLI 命令
8. smoke test（对照第 6 节验收标准）

## 8. 已确认决策

- **Task 状态**：创建时固定 `pending`，状态流转（running / completed / failed / waiting_approval）由 Phase 1 的 Runtime 驱动，Phase 0 不做状态变更命令。
- **Permission**：Phase 0 只定义模型并支持 CLI 录入，不做任何执行时校验；Enforcement（强制实施）是后续阶段的治理层工作。
