# Phase 4 — Business Capabilities 设计

> 执行蓝本。定义 Phase 4 的五个 Business Capability:Marketing / Sales / Customer / Operations / Finance,以及贯穿多 Capability 的业务流程(用户反馈 → Marketing → Sales → Customer → Operations),落地方向基线「每个 Capability 独立定义 Agents / Workflows / Tools / Policies / Metrics」。
> 状态:✅ 定稿(2026-09-01)
> 关联:[项目详解](../../项目详解.md) · [进度总表](../../进度总表.md) · 方向基线 `One-Person-Company-OS-Design-v1.0.md`(v1.1,§2.1 / §5.1-5.4 / Phase 4)· [Phase 3 设计](../phase3/design/research-product.md)

## 1. 背景与目标

Phase 2(Engineering)、Phase 3(Research / Product + 跨 Capability Workflow)已证明 OS 能把多个能力域的 Agent 在同一个 Workflow 中接力执行。Phase 4 的目标:

> OS 具备经营公司所需的基本业务能力——市场、销售、客户、运营、财务,并能让一条真实业务流(用户反馈 → 市场 → 销售 → 客户 → 运营)跨多个 Business Capability 编排执行。

方向基线 Phase 4 给出框架(每个 Capability 独立定义 Agents / Workflows / Tools / Policies / Metrics),不指定具体 Agent;本设计在**复用现有引擎与 Capability 模型**的前提下落地该框架,并选一个跨 Capability 业务流程作为端到端载体。

## 2. 范围与边界

**做**(方向基线 Phase 4 交付物):
- 五个 Business Capability:Marketing / Sales / Customer / Operations / Finance,各含 Agent、Workflow、Tools、Policies、Metrics 声明
- 一条跨 Business Capability 业务流程定义与运行(用户反馈 → Marketing → Sales → Customer → Operations)
- 复用 3.2 跨 Capability Workflow 引擎((capability, role) 解析、Task 落 capability_id)

**明确不做**:
- 内置 LLM 调用 / 真实 CRM / ERP / 支付集成(框架先行,同 Phase 2/3)
- React 前端、Metrics 采集(Prometheus 基线列 Phase 2,但 CLI 可完整交付,独立排期)
- 大规模 Multi-Agent 自动协作(基线 Phase 1 明确排除;Agent 仍「命令即 description」)
- 不改执行引擎内部机制(1.1-1.4 / 2.1-2.3 / 3.1-3.3 冻结)

## 3. 子阶段拆分(每子阶段一个可独立验收交付物)

| 子阶段 | 交付物 | 验收方式 |
|---|---|---|
| **4.1 Marketing / Sales Capability 就位** | marketing(3 Agent)+ sales(3 Agent)Capability + 权限策略 | `os capability/agent list` 就位;默认拒绝生效 |
| **4.2 Customer / Operations / Finance Capability 就位** | customer(2)+ operations(2)+ finance(2)Agent Capability + 权限策略 | 同上;5 个 Business Capability 齐 |
| **4.3 端到端业务流程** | 用户反馈 → Marketing → Sales → Customer → Operations 跨 Capability 流程定义与运行 | 一键运行、跨 5 Capability 节点接力、产出落 workspace、冒烟通过 |

每子阶段完成后写 `stages/<n>.md` 归档并更新进度总表;分支按 `dev → phase-4-business-capabilities` 迭代,阶段结束合回。

## 4. Capability 与 Agent 定义(方向基线 §5.1-5.2 框架落地)

沿用 3.2 的 (capability, role) 解析与 model_hint;role 命名避免跨 Capability 撞名。

| Capability code | Agent role | 职责(框架先行:命令由 workflow 定义提供) |
|---|---|---|
| marketing | campaign | 营销活动策划 |
| marketing | content | 内容生成(公告/文案) |
| marketing | seo | 曝光/关键词优化 |
| sales | lead | 线索跟进 |
| sales | proposal | 方案/报价撰写 |
| sales | account | 客户关系维护 |
| customer | support | 客户支持/答疑 |
| customer | onboarding | 上手引导 |
| operations | process | 流程协调 |
| operations | quality | 质量/合规检查 |
| finance | bookkeeping | 记账/对账 |
| finance | reporting | 财务报告 |

- 每个 Capability 独立声明 Policies(最小权限,默认拒绝)与 Metrics(见 §6,CLI 阶段为声明占位,不采集)。
- **不新建表**:capability / agent / policy / permission / workflow / task 表均已有。

## 5. 存储设计

- 复用现有表,不新增迁移。
- Business 业务产出(营销文案/报价单/支持记录/报告)为 workflow 节点 Task 在 workspace 用 file-write 写入的文档(框架先行,内容由流程定义提供)。
- Metrics 为 Capability 声明的元信息(见 §6),Phase 4 落文档不落库(基线未要求运行时采集)。

## 6. Metrics 声明(每个 Capability 独立)

| Capability | 示例 Metrics(本阶段声明,不采集) |
|---|---|
| marketing | 曝光量 / 线索量 / 转化率 |
| sales | 成交额 / 销售周期 / 赢单率 |
| customer | 响应时长 / 满意度 / 留存率 |
| operations | 交付周期 / 缺陷率 / 流程耗时 |
| finance | 营收 / 现金流 / 毛利 |

- 方向基线 §5.1「产生业务结果」在 CLI 阶段以「产出文档落 workspace」体现;真实指标采集(Prometheus)与 Dashboard 不在 Phase 4 范围。

## 7. 权限策略

每个 Business Capability 的 Agent 只授予其工作所需的最小 Tool 权限(默认拒绝):

| Capability | Agent role | 授予权限 |
|---|---|---|
| marketing | campaign / content / seo | read/file、write/file |
| sales | lead / proposal / account | read/file、write/file |
| customer | support / onboarding | read/file、write/file |
| operations | process / quality | read/file、write/file |
| finance | bookkeeping / reporting | read/file、write/file |

- 跨 Capability:marketing Agent 只碰自己的 workspace 文档,不得拿 sales/finance 权限。
- 沿用 Phase 0 policy + permission 表、CLI `os policy/permission`。

## 8. 端到端业务流程(4.3)

用户反馈处理流程 definition(节点:capability / agent_role / tool / risk):

| step | capability | agent_role | 动作 | risk |
|---|---|---|---|---|
| feedback | — | — | 输入用户反馈(起始节点,skip) | — |
| marketing | marketing | campaign | 分析反馈 → 输出营销调整建议 | low |
| sales | sales | account | 关联客户/线索,输出跟进方案 | low |
| customer | customer | support | 输出支持/答复方案 | low |
| operations | operations | process | 输出流程改进建议 | low |
| finance | finance | reporting | 输出财务影响评估(成本/营收) | low |
| approval | — | — | 审批门(业务执行前) | high |
| delivery | operations | quality | 交付(产出汇总报告 + git 记录) | high |

- 示例定义存 `docs/phase4/examples/business-feedback-workflow.json`(workspace 用占位符)。
- 一键 `os workflow run <id>`:7 可执行节点跨 5 个 Business Capability 接力,审批门人工决定,失败中断语义沿用(3.2/2.2)。

## 9. 验收标准(按子阶段)

**4.1**
1. marketing / sales Capability 及 6 Agent 就位(`os capability/agent list`),带 model_hint。
2. 默认拒绝:未授权 role 调 file-write 被拒(复用 2.1 校验)。

**4.2**
3. customer / operations / finance Capability 及 6 Agent 就位;5 个 Business Capability 全部可见。

**4.3**
4. 用户反馈 8 节点流程一键运行:跨 5 Business Capability 接力,节点 Task 的 (capability, role) 归属正确。
5. 审批门 approve 后续跑、reject 后中断;产出文档落 workspace;全程 Audit。

## 10. CLI 命令契约(在 Phase 1/2/3 之上复用)

| 命令 | 说明 |
|---|---|
| `os capability add/list` | 复用;add marketing/sales/customer/operations/finance |
| `os agent add/list` | 复用;为各 Capability 添加 Agent(--model 沿用 2.3) |
| `os policy/permission add` | 复用;授予各角色最小权限 |
| `os workflow add/run` | 复用;add 接受含 `capability` 字段的 definition,run 解析跨 Capability |
| `os task list/show` | 复用;展示 capability |

通用约定沿用 Phase 0-3(对齐文本表格、错误非 0 退出码、`config/os.yaml` + `--db` 覆盖)。

## 11. 已确认决策(待定稿确认)

- ✅ **5 个 Business Capability 全部在本阶段落地**(方向基线 Phase 4 为整体;不拆成 5 个子阶段,而按「Capability 就位 → 端到端流程」划分,复用已验证引擎)。
- ✅ **Metrics 为声明不采集**:方向基线 §5.1「产生业务结果」以产出文档体现;Prometheus 采集与 Dashboard 独立排期。
- ✅ **框架先行**:Business Agent 仍「命令即 description」,不接真实 CRM/支付/LLM;产出为受控 workspace 文档。
- ✅ **向后兼容**:所有新 workflow 声明 `capability`;未声明节点回退公司级解析(不影响已有流程)。
