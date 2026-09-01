# One-Person Company OS

## 总体设计与技术选型 v1.1

> 状态：Architecture Baseline\
> 修订(v1.1)：统一 Agent/Workflow 模型、审批按风险触发、明确治理强制层\
> 项目性质：全新项目，从零设计\
> 核心目标：构建一个由一个人负责最终决策、由 AI Agent
> 承担大量执行工作的公司操作系统。\
> 重要说明：One-Person Company OS 不从 AI-Fix 扩充而来。AI-Fix
> 是未来可接入 Engineering
> 能力域的一个子系统/能力，而不是本项目的基础架构。

------------------------------------------------------------------------

# 1. 项目目标与边界

## 1.1 项目目标

One-Person Company OS（以下简称 OS）的目标不是简单做一个"AI Agent
管理器"，也不是把传统公司的部门全部 AI 化。

目标是建立一套长期稳定的系统，使一个人能够：

-   定义公司的目标、策略和规则；
-   定义 AI 可以做什么、不能做什么；
-   将公司目标转化为 Workflow 和 Task；
-   将 Task 分配给不同能力域中的 Agent；
-   让 Agent 在受控环境中执行；
-   对执行结果进行验证和审查；
-   对高风险操作进行人工审批；
-   自动记录 Decision、Audit 和执行结果；
-   将长期经验沉淀为公司的 Knowledge / Memory。

核心模型：

``` text
Human
  ↓
Company OS
  ↓
Capability ──── Agent(执行角色)
  ↓
Workflow(流程编排，跨 Capability)
  ↓
Task
  ↓
Runtime
  ↓
Tool
  ↓
Delivery
```

> 两个视图：Agent 是"谁来做"——Capability 中的执行角色；Workflow 是"按什么流程做"——跨 Capability 的流程编排，不被单个 Agent 拥有。二者不在同一层级。Verification 与 Approval 属于治理链，按风险策略触发，不是每次执行的必经环节。

最终目标：

``` text
一个人定义方向和规则
        ↓
AI 负责大量具体执行
        ↓
系统负责流程、权限、验证和审计
        ↓
人只处理真正需要人做决策的事情
```

## 1.2 项目定位

One-Person Company OS 不是：

``` text
❌ 一个超级 Chatbot
❌ 一个简单 Agent Framework
❌ 一个纯 Workflow Engine
❌ 一个 AI-Fix 的升级版
❌ 一个传统 ERP
```

而是：

``` text
Company Operating System
+
AI Agent Execution Platform
+
Governance System
+
Knowledge System
```

## 1.3 与 AI-Fix 的关系

AI-Fix 是一个独立项目。

本项目不会直接修改 AI-Fix，也不以 AI-Fix 现有代码架构作为基础。

关系应该是：

``` text
                 One-Person Company OS
                          │
             ┌────────────┼────────────┐
             │            │            │
        Engineering    Research     Business
             │            │            │
          AI-Fix        Agents       Agents
```

未来可以通过标准接口将 AI-Fix 接入 Engineering Capability。

因此：

``` text
One-Person Company OS
        ↓
定义公司级能力、规则和流程

AI-Fix
        ↓
负责具体的软件工程自动化能力
```

二者保持独立。

## 1.4 第一阶段边界

第一阶段不实现完整的一人公司。

优先建立：

``` text
Company Kernel
Capability
Agent
Workflow
Task
Policy
Permission
Approval
Runtime
Tool
Memory
Audit
```

然后首先选择 **Engineering** 作为第一个真实业务能力域。

第一阶段不直接实现：

-   Marketing
-   Sales
-   Finance
-   HR
-   Customer Support
-   完整 CRM
-   完整 ERP
-   大规模 Multi-Agent 自动协作

这些属于后续 Capability。

------------------------------------------------------------------------

# 2. 核心设计原则

## 2.1 Capability，而不是 Department

系统底层采用：

> Capability-Oriented Architecture（能力域架构）

不直接按照传统公司部门划分系统。

推荐：

``` text
Company
  ↓
Capability
  ├── Agent(执行角色)
  └── Workflow → Task(流程编排)
```

例如：

``` text
Engineering
├── Architect Agent
├── Coding Agent
├── QA Agent
└── Review Agent

Research
├── Market Research Agent
├── Technology Research Agent
└── Competitor Research Agent

Product
├── Product Analyst Agent
├── PRD Agent
└── Roadmap Agent
```

Department 只是未来的一种组织视图，而不是底层架构。

原因：

同一个 Workflow 往往跨越多个传统部门。

例如：

``` text
用户反馈
  ↓
Research
  ↓
Product
  ↓
Engineering
  ↓
QA
  ↓
Release
  ↓
Marketing
```

因此真正连接公司的不是 Department，而是 Workflow。

Workflow 是跨 Capability 的流程编排：它定义"一类工作如何完成"，由 Capability 中的 Agent 执行其中的 Task；Workflow 不被单个 Agent 拥有。

## 2.2 Company 是最高层

Company 负责定义：

``` text
Vision
Goals
Strategy
Policy
Risk
Budget
Knowledge
```

Human 是 Company 的最终 Owner。

AI Agent 不拥有 Company 的最终决策权。

## 2.3 Human-in-the-loop

高风险操作必须经过人工审批。审批按风险策略触发：低风险、可自动验证的路径自动放行，人只处理真正需要决策的高风险事项；审批不是每次执行的必经环节。

``` text
AI 提议
  ↓
AI 执行
  ↓
Verification
  ↓
Risk Evaluation
  ↓
Human Approval
  ↓
Delivery
```

例如：

-   生产环境操作；
-   权限修改；
-   删除关键资源；
-   发布版本；
-   修改公司 Policy；
-   高风险资金操作；
-   受保护分支合并。

## 2.4 Policy First

系统的权力链：

``` text
Human Policy
      ↓
Permission
      ↓
Workflow
      ↓
Agent
      ↓
Tool
```

Policy 的权力由系统强制实施：Agent 运行在受控环境中，无法直接修改 Policy/Permission、删除 Audit 或绕过 Approval——治理约束由系统在执行路径上强制拦截，而非依赖 Agent 自觉。

Agent 不得：

``` text
修改自己的 Policy
修改自己的 Permission
绕过 Approval
删除 Audit
绕过 Workflow
```

## 2.5 Contract First

核心模块通过 Port / Interface 隔离：

``` text
Agent
Runtime
Model
Provider
Git
Storage
Tool
Workflow
```

具体实现只能作为 Adapter 存在。

## 2.6 Modular Monolith

第一阶段不做微服务。

目标：

``` text
One Binary
+
SQLite
+
Docker
```

理由：

-   一个人可以维护；
-   部署简单；
-   Debug 简单；
-   AI Coding 容易理解；
-   降低运维成本；
-   后续仍可以通过接口拆分。

## 2.7 AI 可替换

系统不能绑定某一个模型或 Agent Runtime。

正确模型：

``` text
Agent
  ↓
Runtime / Model Port
  ↓
Claude / GPT / Gemini / Local Model
```

第一阶段可以只实现一个 Runtime，但架构必须允许替换。

## 2.8 可审计

系统必须能够回答：

``` text
谁执行？
为什么执行？
依据什么 Policy？
执行了什么？
调用了什么 Tool？
修改了什么？
结果是什么？
是否经过审批？
消耗多少资源？
```

------------------------------------------------------------------------

# 3. 总体架构

``` text
                         Human
                           │
                           ▼
                  ┌────────────────┐
                  │    Company     │
                  │ Goals / Policy │
                  │ Strategy/Risk │
                  └───────┬────────┘
                          │
                          ▼
                 ┌──────────────────┐
                 │ Governance Layer │
                 │ Permission       │
                 │ Approval         │
                 │ Audit            │
                 └────────┬─────────┘
                          │
                          ▼
                 ┌──────────────────┐
                 │   Capabilities   │
                 └────────┬─────────┘
                          │
          ┌───────────────┼────────────────┐
          ▼               ▼                ▼
     Engineering       Research        Business
          │               │                │
        Agents          Agents           Agents
          │               │                │
          └───────────────┼────────────────┘
                          ▼
                 ┌──────────────────┐
                 │    Workflow      │
                 │     Engine       │
                 └────────┬─────────┘
                          │
                          ▼
                       Tasks
                          │
                          ▼
                 ┌──────────────────┐
                 │     Runtime      │
                 └────────┬─────────┘
                          │
                          ▼
                 ┌──────────────────┐
                 │  Sandbox / Tools │
                 └────────┬─────────┘
                          │
             ┌────────────┼────────────┐
             ▼            ▼            ▼
            Git        Web/API       Files
             │            │            │
             └────────────┼────────────┘
                          ▼
                    External World


                 ┌──────────────────┐
                 │      SQLite      │
                 │ Task / Workflow  │
                 │ Memory / Audit   │
                 │ Decision / Cost  │
                 └──────────────────┘
```

图中 Agents 是各 Capability 的执行角色，执行的是 Workflow Engine 编排出的节点 Task；Workflow Engine 负责跨 Capability 的流程编排，不属于任何单个 Agent。

## 3.1 两条核心链路

### 执行链

``` text
Company
  ↓
Capability
  ├── Agent(执行角色)
  ↓
Workflow(流程编排)
  ↓
Task
  ↓
Runtime
  ↓
Tool
```

Agent 是执行角色；Workflow 是跨 Capability 的流程编排，负责将工作拆解为 Task 并交给相应 Agent 执行。

### 治理链

``` text
Policy
  ↓
Permission
  ↓
Approval
  ↓
Audit
  ↓
Memory / Decision
```

执行链负责"把事情做完"。

治理链负责"保证事情以正确的方式完成"。

------------------------------------------------------------------------

# 4. 核心模型与技术选型

## 4.1 核心对象

  对象         职责
  ------------ -------------------------------
  Company      公司级目标、策略和上下文
  Capability   公司具备的业务能力域
  Agent        承担具体职责的 AI 执行者
  Workflow     描述一类工作的标准流程
  Task         工作的最小执行单位
  Policy       AI 和系统必须遵守的规则
  Permission   Agent / Runtime / Tool 的权限
  Approval     人工审批节点
  Runtime      Agent 的执行环境
  Tool         Agent 可以使用的具体能力
  Workspace    Task 的工作空间
  Memory       长期知识和经验
  Decision     人或系统产生的重要决策
  Audit        系统行为记录
  Provider     外部平台适配层
  Execution    一次实际执行过程

核心关系：

``` text
Company
   │
   ├── Capability
   │       │
   │       └── Agent
   │
   ├── Policy
   ├── Memory
   └── Decision

Workflow(流程编排)
   ↓
Task ──── 由 Capability 下的 Agent 执行
   ↓
Execution
   ↓
Runtime
   ↓
Tool
```

## 4.2 技术选型

  类型                技术                              决策
  ------------------- --------------------------------- ---------
  Language            Go                                确定
  Architecture        Modular Monolith                  确定
  CLI                 Cobra                             确定
  HTTP                net/http + Chi                    确定
  Database            SQLite                            确定
  SQL                 SQLC                              确定
  Migration           golang-migrate                    确定
  Queue               SQLite-backed Queue               确定
  Workflow            自研轻量 State Machine            确定
  Agent Runtime       自定义 Port                       确定
  Model               Model Provider abstraction        确定
  Git                 Git CLI + Go Wrapper              确定
  External Provider   Provider abstraction              确定
  Sandbox             Docker                            确定
  Config              YAML                              确定
  Logging             slog                              确定
  Search              SQLite FTS5                       Phase 1
  Frontend            React + TypeScript + Ant Design   Phase 2
  Metrics             Prometheus                        Phase 2
  Tracing             OpenTelemetry                     Phase 2

## 4.3 第一阶段明确不引入

``` text
Redis
Kafka
RabbitMQ
NATS
Temporal
Kubernetes
Microservices
Vector Database
复杂 RAG 平台
```

这些技术并不是禁止使用，而是在没有真实规模需求之前不增加系统复杂度。

原则：

> 一人公司的第一生产力不是技术复杂度，而是低维护成本。

## 4.4 Database

采用：

``` text
SQLite
+
SQLC
+
golang-migrate
```

结构：

``` text
Domain
  ↓
Repository Interface
  ↓
SQLC
  ↓
SQLite
```

未来如果需要 PostgreSQL：

``` text
Domain
  ↓
Repository
  ↓
SQLite / PostgreSQL
```

上层 Domain 不需要感知数据库类型。

## 4.5 Workflow

Workflow 与 Task 属于两个层级。**Task 状态机**管理单个执行单元的生命周期，第一阶段自研轻量 State Machine：

``` text
PENDING
  ↓
RUNNING
  ↓
WAITING_APPROVAL
  ↓
COMPLETED
```

异常：

``` text
RUNNING
  ↓
FAILED
  ↓
RETRY
```

**Workflow 编排**管理跨阶段、跨 Capability 的流程（如 5.3 中的多阶段业务流程），由节点与阶段流转组成，Task 是其执行单元；概要层面不展开编排细节。

暂不使用 Temporal。

## 4.6 Queue

采用 SQLite-backed Queue：

``` text
READY
  ↓
LEASED
  ↓
RUNNING
  ↓
COMPLETED / FAILED
```

支持：

-   Priority
-   Retry
-   Attempt
-   Lease
-   Timeout
-   Worker ID
-   Recovery

只有真实负载证明不足时才升级为外部 Queue。

## 4.7 Agent Runtime

统一接口：

``` go
type Runtime interface {
    Execute(ctx context.Context, task Task) (Result, error)
}
```

Runtime 负责：

-   启动 Agent；
-   管理执行环境；
-   管理超时；
-   管理资源；
-   收集输出；
-   返回执行结果。

Agent 本身不能直接控制底层基础设施。

## 4.8 Tool

Tool 是 Agent 能够执行的最小外部能力。

例如：

``` text
Git
File
Shell
HTTP
Search
Repository
Issue Tracker
Test
Build
```

每个 Tool 必须有：

``` text
Tool Definition
Permission
Input Schema
Output Schema
Risk Level
Audit
```

## 4.9 Sandbox

Agent 默认在受控 Workspace 中运行。

``` text
Task
 ↓
Workspace
 ↓
Sandbox
 ↓
Agent
 ↓
Tools
```

需要控制：

-   Filesystem；
-   Network；
-   CPU；
-   Memory；
-   Timeout；
-   Host Access；
-   Secret Access。

Docker 是第一阶段的 Sandbox 实现。

## 4.10 Memory

第一阶段采用：

``` text
SQLite
+
FTS5
+
Structured Memory
```

Memory 类型：

``` text
Company Knowledge
Project Context
Decision
Lesson
Architecture
Convention
Task History
```

暂不引入 Vector Database。

------------------------------------------------------------------------

# 5. 能力域、Agent、Workflow 与 Task

## 5.1 Capability

Capability 表示：

> 公司具备的一种长期业务能力。

例如：

``` text
Engineering
Research
Product
Marketing
Sales
Customer
Operations
Finance
```

Capability 不是 Department。

它应该能够：

-   拥有 Agent；
-   拥有 Workflow；
-   定义输入输出；
-   使用 Tool；
-   遵守 Policy；
-   产生业务结果。

## 5.2 Agent

Agent 是 Capability 中承担具体职责的执行单元。

例如：

``` text
Engineering
├── Architect Agent
├── Coding Agent
├── QA Agent
└── Review Agent
```

Research：

``` text
Research
├── Market Research Agent
├── Technology Research Agent
└── Competitor Research Agent
```

Product：

``` text
Product
├── Product Analyst Agent
├── PRD Agent
└── Roadmap Agent
```

Agent 不等于模型。

``` text
Agent
  ↓
Runtime
  ↓
Model
```

同一个 Agent 可以未来切换不同模型。

## 5.3 Workflow

Workflow 描述：

> 一类工作应该如何完成。

例如产品开发：

``` text
Idea
 ↓
Research
 ↓
Product Analysis
 ↓
PRD
 ↓
Engineering
 ↓
QA
 ↓
Release
 ↓
Marketing
```

例如 Bug 修复：

``` text
Issue
 ↓
Analysis
 ↓
Implementation
 ↓
Test
 ↓
Review
 ↓
Approval
 ↓
Delivery
```

Workflow 是跨 Capability 的关键机制。

## 5.4 Task

Task 是实际执行单位。

例如：

``` text
Product Launch Workflow

T001 Research competitors
T002 Analyze customer demand
T003 Generate PRD
T004 Implement feature
T005 Run tests
T006 Prepare release
T007 Generate announcement
```

Task 至少包含：

``` text
ID
Company
Capability
Project
Workflow
Description
Priority
Status
Agent
Workspace
Attempt
Risk
Result
CreatedAt
UpdatedAt
```

------------------------------------------------------------------------

# 6. 治理、安全与工程约束

## 6.1 Policy

Policy 是整个系统的最高级约束之一。

Policy 可以规定：

``` text
Agent 能做什么
Agent 不能做什么
哪些操作需要审批
哪些资源不能访问
哪些环境不能修改
哪些工具不能调用
```

Policy 不应该由 Agent 自己生成最终版本。

## 6.2 Permission

权限至少分为：

``` text
Read
Write
Execute
Network
Secret
Production
Administrative
```

Permission 必须遵循最小权限原则。

## 6.3 Approval

高风险行为进入：

``` text
WAITING_APPROVAL
```

Human 完成：

``` text
Approve
Reject
Request Changes
```

Agent 不得伪造 Approval。

## 6.4 Agent 禁止事项

``` text
❌ 修改 Policy
❌ 修改自己的 Permission
❌ 删除 Audit
❌ 绕过 Approval
❌ 修改系统核心约束
❌ 未授权访问宿主机
❌ 未授权修改生产环境
❌ 未授权执行资金相关操作
```

## 6.5 Agent 必须事项

``` text
✅ 遵守 Task Scope
✅ 遵守 Policy
✅ 使用授权 Tool
✅ 在 Workspace 工作
✅ 修改后验证
✅ 输出结果
✅ 输出风险
✅ 记录关键行为
```

## 6.6 工程开发流程

项目自身也采用：

``` text
Requirement
 ↓
Design
 ↓
Contract
 ↓
Implementation
 ↓
Test
 ↓
Review
```

重要设计必须先形成文档和约束，再开始 Coding。

------------------------------------------------------------------------

# 7. Roadmap

## Phase 0：Company Kernel

从零开始建立：

``` text
Company
Policy
Permission
Task
Capability
Agent
Workflow
Audit
Storage
```

目标：

> 先把"公司操作系统"的核心模型建立起来，而不是先写具体业务 Agent。

## Phase 1：Execution Kernel

实现：

``` text
Task Queue
Workflow Engine
Agent Runtime
Tool
Workspace
Sandbox
Approval
Execution
```

目标：

> OS 可以真正让 Agent 在受控环境中执行任务。

## Phase 2：Engineering Capability

建立第一个完整 Capability：

``` text
Engineering
├── Architect Agent
├── Coding Agent
├── QA Agent
└── Review Agent
```

实现：

``` text
Issue
 ↓
Analysis
 ↓
Plan
 ↓
Implementation
 ↓
Test
 ↓
Review
 ↓
Approval
 ↓
Delivery
```

此时再把现有 AI-Fix 作为独立 Engineering 能力接入，而不是反过来让 OS
依赖 AI-Fix。

## Phase 3：Research / Product

增加：

``` text
Research
Product
```

例如：

``` text
Research
 ↓
Product Analysis
 ↓
PRD
 ↓
Engineering
 ↓
QA
```

形成跨 Capability Workflow。

## Phase 4：Business Capabilities

逐步增加：

``` text
Marketing
Sales
Customer
Operations
Finance
```

每个 Capability 独立定义：

``` text
Agents
Workflows
Tools
Policies
Metrics
```

## Phase 5：One-Person Company OS

最终形成：

``` text
                         Human
                           │
                     Company OS
                           │
             ┌─────────────┼─────────────┐
             │             │             │
        Engineering      Research      Business
             │             │             │
          Agents         Agents        Agents
             │             │             │
             └─────────────┼─────────────┘
                           │
                       Workflows
                           │
                         Tasks
                           │
                        Runtime
                           │
                         Tools
                           │
                     External World
```

最终人负责：

``` text
Vision
Strategy
Capital
Policy
Risk
Final Decisions
```

AI 负责：

``` text
Research
Planning
Execution
Verification
Reporting
Knowledge Accumulation
```

------------------------------------------------------------------------

# 附录 A：Repository Structure

``` text
one-person-company-os/
│
├── cmd/
│   └── os/
│
├── internal/
│   ├── company/
│   ├── capability/
│   ├── agent/
│   ├── workflow/
│   ├── task/
│   ├── policy/
│   ├── permission/
│   ├── approval/
│   ├── runtime/
│   ├── tool/
│   ├── workspace/
│   ├── execution/
│   ├── memory/
│   ├── decision/
│   ├── audit/
│   ├── storage/
│   ├── config/
│   └── observability/
│
├── providers/
│
├── runtimes/
│
├── capabilities/
│   ├── engineering/
│   ├── research/
│   └── product/
│
├── workflows/
│
├── migrations/
│
├── docs/
│   ├── architecture/
│   ├── policies/
│   ├── decisions/
│   ├── workflows/
│   └── agents/
│
├── tests/
│
├── go.mod
└── README.md
```

------------------------------------------------------------------------

# 附录 B：ADR 基线

建议从项目第一天建立：

``` text
ADR-001  Capability-Oriented Architecture
ADR-002  Go + Modular Monolith
ADR-003  SQLite as Primary Storage
ADR-004  SQLC instead of ORM
ADR-005  SQLite-backed Task Queue
ADR-006  Task State Machine + Workflow Orchestration
ADR-007  Agent Runtime Port
ADR-008  Tool Permission Model
ADR-009  Human Approval for High-Risk Actions
ADR-010  Policy has Authority over Agent
ADR-011  Sandbox-based Agent Execution
ADR-012  AI-Fix is an External Engineering Capability
```

------------------------------------------------------------------------

# 附录 C：最终核心模型

整个 One-Person Company OS 最核心的模型是：

``` text
                    Company
                       │
                ┌──────┴──────┐
                │             │
             Strategy       Policy
                │             │
                └──────┬──────┘
                       │
                  Capability
                       │
                 Agent(执行角色)
                       │
                   Workflow(流程编排)
                       │
                     Task
                       │
                  Execution
                       │
                    Runtime
                       │
                     Tool
                       │
                 External World
```

治理模型：

``` text
Policy
  ↓
Permission
  ↓
Approval
  ↓
Audit
  ↓
Memory
  ↓
Decision
```

组织模型：

``` text
Company
  ↓
Capability
  ↓
Agent
```

执行模型：

``` text
Workflow
  ↓
Task
  ↓
Execution
```

因此：

> **Capability 是组织能力，Agent 是执行角色，Workflow 是工作方法，Task
> 是具体工作，Runtime 是执行环境，Tool 是执行手段，Policy / Permission /
> Approval / Audit 是治理体系。**

Department 不属于核心架构。

未来如果需要展示传统组织结构，可以建立：

``` text
Organization View
        ↓
Capability
        ↓
Agent
```

作为上层视图，而不改变底层模型。

------------------------------------------------------------------------

# 最终原则

One-Person Company OS 不追求：

``` text
让 AI 替代所有人
```

而追求：

``` text
Human 定义目标和规则
        ↓
OS 将目标转化为可执行工作
        ↓
Capability 提供专业能力
        ↓
Agent 执行工作
        ↓
Workflow 管理过程
        ↓
Policy 限制边界
        ↓
Verification 验证结果
        ↓
Human 对关键决策负责
        ↓
Audit + Memory 沉淀公司资产
```

这套结构必须优先于具体 Agent、具体模型和具体业务功能。

**先建立 OS，再向 OS 中加入能力。**
