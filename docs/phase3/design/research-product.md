# Phase 3 — Research / Product 跨 Capability Workflow 设计

> 执行蓝本。定义 Phase 3 的 Research / Product 两个 Capability,以及首个**跨 Capability Workflow**(Research → Product → Engineering → QA),把方向基线「Workflow 是跨 Capability 的流程编排」落地到执行引擎。
> 状态:✅ 定稿(2026-09-01)
> 关联:[项目详解](../../项目详解.md) · [进度总表](../../进度总表.md) · 方向基线 `One-Person-Company-OS-Design-v1.0.md`(v1.1,§2.1 / §5.1-5.4 / Phase 3)· [Phase 2 设计](../phase2/design/engineering-capability.md)

## 1. 背景与目标

Phase 2 建立了第一个业务能力域(Engineering:4 Agent + 8 阶段流程),但所有 Agent 都在**单个 Capability** 内、由**公司级 role** 解析。方向基线 Phase 3 要求:

> OS 能把跨 Capability 的完整业务流编排起来——一次产品从 Idea 到交付,Research / Product / Engineering / QA 各能力域的 Agent 在同一个 Workflow 中接力执行,人只做关键决策(审批)。

即新增 **Research**(Market / Technology / Competitor 研究)与 **Product**(Analyst / PRD / Roadmap)两个 Capability,并把 2.2 的「单 Capability 8 阶段流程」扩展为**跨 Capability 流程**:`Research → Product Analysis → PRD → Engineering → QA`(方向基线 §5.3 产品开发示例)。

## 2. 范围与边界

**做**(方向基线 Phase 3 交付物):
- Research / Product 两个新 Capability + 各自 Agent 就位(方向基线 §5.2:Research 3 Agent、Product 3 Agent)
- **跨 Capability Workflow 执行**:workflow 节点可声明所属 capability,引擎按 `(capability, role)` 解析 Agent、Task 落 `capability_id`
- 一个端到端跨 Capability 流程定义与运行(产品从研究到交付)
- 跨 Capability 场景下权限策略复用(默认拒绝 + 按 capability 角色授予)

**明确不做**:
- 内置 LLM 调用(同 Phase 2,框架先行;Agent 命令仍由 workflow 定义提供)
- 真实网络调研/爬虫 Tool(方向基线 Sandbox 无网络;Research/Product 用现有 shell/file/git Tool 在受控环境执行,产出文档落 workspace)
- React 前端、Metrics / Tracing
- Business Capabilities(Marketing / Sales / Customer / Operations / Finance,Phase 4)
- 不改执行引擎内部机制(1.1-1.4 / 2.1-2.3 冻结)

## 3. 子阶段拆分(每子阶段一个可独立验收交付物)

| 子阶段 | 交付物 | 验收方式 |
|---|---|---|
| **3.1 Research / Product Capability 就位** | Research(3 Agent)+ Product(3 Agent)Capability + CLI 展示 + 权限策略 | `os capability list` 见 research/product;`os agent list` 各见 3 个 Agent;策略按 role 授予 |
| **3.2 跨 Capability Workflow 引擎** | workflow 节点支持 `capability` 字段;Agent 按 (capability, role) 解析;Task 落 `capability_id` | 定义含 research+product+engineering 节点的 workflow,run 后各节点 Agent 来自正确 Capability,Task.capability 正确 |
| **3.3 端到端跨 Capability 流程** | 产品开发流程定义(Idea→Research→Product Analysis→PRD→Engineering→QA→Approval→Delivery)一键运行 | 全链路按序执行、Agent 各自就位、审批门触发、产出落 workspace、冒烟通过 |

每子阶段完成后写 `stages/<n>.md` 归档并更新进度总表;分支按 `dev → phase-3-research-product` 迭代,阶段结束合回。

## 4. 领域模型扩展

### 4.1 新 Capability 与 Agent(方向基线 §5.2)

| Capability code | Agent role | 定位 |
|---|---|---|
| research | market | 市场调研(需求/趋势) |
| research | technology | 技术调研(方案可行性) |
| research | competitor | 竞品调研 |
| product | analyst | 产品分析(需求 → 方案取舍) |
| product | prd | PRD 撰写 |
| product | roadmap | 路线图规划 |

- 沿用 2.3 的 `agent.model_hint`(CLI `os agent add --model`),Agent 仍不触发真实模型调用。
- **不新建表**:capability / agent 表 Phase 0 已建,复用。

### 4.2 Workflow 节点支持跨 Capability

2.2 的 `WorkflowNode` 扩展:

| 字段 | 说明 |
|---|---|
| `capability`(新增) | 节点所属 Capability code(如 `research` / `product` / `engineering`);空 = 沿用 2.2 公司级 role 解析(向后兼容) |
| `agent_role` | 节点执行 Agent 的 role;结合 capability 解析为 `(capability_id, role)` |

**解析规则**(引擎内 `resolveAgentForNode`):
1. 节点声明 `capability` → 按 `(company_id, capability.code)` 查 Capability,再按 `(capability_id, agent_role)` 查 Agent;查不到 → 报错中断。
2. 节点未声明 `capability` → 回退 2.2 的 `GetAgentByRole(company_id, agent_role)`(兼容现有 engineering workflow)。

**Task 归属**:节点创建 Task 时写 `task.capability_id`(表列 Phase 0 已建,此前 workflow run 未落)。`os task list/show` 沿用。

## 5. 存储设计

- 复用现有表(company/capability/agent/workflow/task/execution/approval/audit),**不新增迁移**。
- capability_id 已存在于 task 表;workflow.definition 为 JSON 文本,新增 `capability` 字段只改 JSON 结构,不落新列。
- Research / Product 的产出(调研报告/PRD/路线图)作为 workflow 节点 Task 的 workspace 文件,由现有 file-write / shell Tool 产生。

## 6. Capability 与权限策略(3.1)

每个新 Capability 的 Agent 只能调其被授予的 Tool(默认拒绝,Phase 1 校验生效):

| Capability | Agent role | 授予权限(示例策略) |
|---|---|---|
| research | market / technology / competitor | read/file、write/file(产出调研文档) |
| product | analyst / prd / roadmap | read/file、write/file(产出分析/PRD/路线图) |
| (既有) engineering | architect / coding / qa / review | 沿用 2.1 |

- 跨 Capability 场景:research Agent 只碰自己的 workspace 文档,**不得**拿 engineering 的 git 权限。
- 策略仍用 Phase 0 的 policy + permission 表,CLI `os policy` / `os permission` 创建。

## 7. 跨 Capability Workflow 引擎(3.2)

`RunWorkflow`(2.2)扩展:

- 解析节点 JSON:识别新增 `capability` 字段;`capability` 为空或 `engineering` 时行为不变。
- `resolveAgentForNode`:按 §4.2 规则解析 Agent,并把 `capability_id` 透传给 `CreateTask`。
- 校验:声明的 `capability` 在公司下不存在,或该 capability 下无对应 role 的 Agent → 该节点报错,workflow 中断(遵循 2.2 失败中断语义)。
- 审批门/轮询/中断逻辑(2.2)全部复用,不重写。

## 8. 端到端跨 Capability 流程(3.3)

产品开发流程 definition(节点:capability / agent_role / tool / risk):

| step | capability | agent_role | 动作 | risk |
|---|---|---|---|---|
| idea | — | — | 输入 Idea(起始节点,skip) | — |
| research | research | market | 市场调研,产出调研文档 | low |
| product-analysis | product | analyst | 分析需求与方案取舍 | low |
| prd | product | prd | 撰写 PRD,落 workspace | low |
| engineering | engineering | architect | 按 PRD 实现(git + file) | medium |
| qa | engineering | qa | 测试校验 | medium |
| approval | — | — | 审批门(交付前) | high |
| delivery | engineering | coding | 交付产物(git commit/tag) | high |

- 示例定义存 `docs/phase3/examples/product-workflow.json`(workspace 用占位符)。
- 一键 `os workflow run <id>`:8 节点跨 3 个 Capability 接力,审批门人工决定,失败中断语义沿用。

## 9. 验收标准(按子阶段)

**3.1**
1. `os capability list --company <id>` 显示 research / product;`os agent list --capability <id>` 各见 3 个 Agent(带 model_hint)。
2. research/product Agent 权限默认拒绝:未授权 role 调 file-write 被拒(复用 2.1 校验)。

**3.2**
3. 定义含 research + product + engineering 节点的 workflow,`os workflow run` 各节点 Agent 来自正确 Capability(能验证跨 Capability 解析)。
4. 节点 Task 的 `capability_id` 正确落库(`os task list` 或 db 查询验证);不存在的 capability / 无匹配 Agent → 中断。

**3.3**
5. 产品开发 8 节点流程一键运行:Research→Product→Engineering→QA 跨 3 Capability 接力;approval 触发人工审批,approve 后继续、reject 后中断。
6. 产出文档(调研/PRD)落 workspace,交付节点产出 git 记录;全程 Audit。

## 10. CLI 命令契约(在 Phase 1/2 之上新增/增强)

| 命令 | 说明 |
|---|---|
| `os capability add/list` | 复用;add research/product(已有) |
| `os agent add/list` | 复用;为 research/product 添加 Agent(--model 沿用 2.3) |
| `os policy / permission add` | 复用;授予 research/product 角色 tool 权限 |
| `os workflow add/run` | 复用;add 接受含 `capability` 字段的 definition,run 解析跨 Capability |
| `os task list/show` | 复用;可展示 capability |

通用约定沿用 Phase 0/1/2(对齐文本表格、错误非 0 退出码、`config/os.yaml` + `--db` 覆盖)。

## 11. 已确认决策(待定稿确认)

- ✅ **跨 Capability 用节点级 `capability` 字段表达**(而非 Workflow 级归属):Workflow 本身跨 Capability,节点各自声明归属,符合方向基线「Workflow 不被单个 Capability 拥有」。
- ✅ **不新增迁移**:capability_id 列、Agent 表、JSON definition 均已有,纯逻辑增强。
- ✅ **框架先行**:Research/Product Agent 仍是「命令即 description」执行,不接真实 LLM / 网络调研;新增产出为受控 workspace 文档。
- ✅ **向后兼容**:未声明 capability 的节点回退 2.2 公司级 role 解析,现有 engineering workflow 不受影响。
