# Phase 5 — One-Person Company OS(完整形态)设计

> 执行蓝本。Phase 5 是把 0-4 期沉淀的 Engineered/Research/Product/Business 全部 Capability 收拢为「一个人运营的公司 OS」:补齐基线核心模型最后两个缺口(Memory / Decision,知识沉淀与决策记录),提供一人全景运营视图,并以一条跨全部 Capability 的完整流程证明完整形态。
> 状态:✅ 定稿(2026-09-01)
> 关联:[项目详解](../../项目详解.md) · [进度总表](../../进度总表.md) · 方向基线 `One-Person-Company-OS-Design-v1.0.md`(v1.1,§1.4 / §4.10 / §7 Phase 5 / 附录 C)· [Phase 4 设计](../phase4/design/business-capabilities.md)

## 1. 背景与目标

0-4 期已交付:Company Kernel(0)、Execution Kernel(1)、Engineering(2)、Research / Product + 跨 Capability Workflow(3)、Business Capabilities(4)。执行链(Company → Capability → Agent → Workflow → Task → Execution → Runtime → Tool)与治理链(Policy → Permission → Approval → Audit)均已落地。

基线 §1.4 第一阶段边界列出的核心对象中,唯一仍未落地的是 **Memory** 与 **Decision**——即「知识沉淀」与「决策记录」,对应 §1.2 定位中的 Knowledge System 与"自动记录 Decision"的项目目标。Phase 5 的目标:

> OS 具备完整形态:一个人既能调度全部 Capability 协同执行,也能看到公司全景、记录决策、沉淀知识——公司资产随使用而增长,而非用完即弃。

## 2. 范围与边界

**做**:
- **Memory(Knowledge System)**:结构化知识落库 + FTS5 全文本搜索 + `os memory` CLI(基线 §4.10)
- **Decision**:人类/系统决策落库,审批结果自动记录,`os decision` CLI(基线 §4.1「自动记录 Decision」)
- **全景运营视图**:`os overview` 单命令聚合全公司状态,突出「需要人决策的事项」(待审批)
- **完整形态流程**:一条跨全部 Capability(Engineering + Research + Product + Marketing + Sales + Customer + Operations + Finance)的公司级流程,一键运行 + 审批门

**明确不做**:
- 前端 / Dashboard、Metrics 采集 / Prometheus(独立排期,同 Phase 4 决策)
- Memory 接 LLM / RAG / Vector DB(基线 §4.10「暂不引入」;结构化 Memory + FTS5 搜索即达 CLI 目标)
- 真实 CRM/ERP/支付/LLM 集成、大规模 Multi-Agent(框架先行,同 0-4)
- 改写已冻结的执行引擎与治理强制层(1.1-4.3 冻结)

## 3. 子阶段拆分(每子阶段一个可独立验收交付物)

| 子阶段 | 交付物 | 验收方式 |
|---|---|---|
| **5.1 Memory 落库 + 搜索** | migration 0005(memory 表 + FTS5)+ `os memory add/list/search` | add/list/search 冒烟;FTS5 MATCH 命中 |
| **5.2 Decision 落库 + 自动记录** | migration 0006(decision 表)+ `os decision add/list/show` + approval 结果自动落 Decision | approve/reject 后 decision list 出现记录 |
| **5.3 全景运营视图** | `os overview` 聚合命令(capabilities/agents、运行中 workflows、待审批、最近 decisions/tasks、memory 高亮) | overview 展示全公司状态;待审批项醒目 |
| **5.4 完整形态流程** | 跨全部 Capability 公司级流程(新品发布)定义与运行 + 审批门 + Memory/Decision 沉淀 | 一键运行、跨 8 Capability 接力、审批、知识/决策落库 |

每子阶段完成后写 `stages/<n>.md` 归档并更新进度总表;分支按 `dev → phase-5-oneperson-os` 迭代,阶段结束合回 dev。

## 4. Memory(Knowledge System,5.1)

### 4.1 表设计(migration 0005)

沿用 TEXT UUID PK / company_id FK / created_at+updated_at INTEGER 约定。

```sql
CREATE TABLE memory (
    id          TEXT PRIMARY KEY,
    company_id  TEXT NOT NULL REFERENCES company(id),
    type        TEXT NOT NULL,   -- lesson | knowledge | project_context | decision_ref | architecture | convention | task_history
    title       TEXT NOT NULL,
    content     TEXT NOT NULL,
    source      TEXT NOT NULL DEFAULT '',  -- 来源血缘:workflow:<id> / task:<id> / approval:<id> / manual
    tags        TEXT NOT NULL DEFAULT '',  -- 空格分隔
    created_at  INTEGER NOT NULL,
    updated_at  INTEGER NOT NULL
);

CREATE VIRTUAL TABLE memory_fts USING fts5(memory_id, title, content, tokenize='unicode61');
```

- `memory_fts.memory_id` 存 memory 行 id,由触发器在 memory INSERT/UPDATE/DELETE 时同步,保证 `os memory search` 可 join 回 memory 取完整行。
- type 对应基线 §4.10 的 Memory 类型(Company Knowledge / Project Context / Decision / Lesson / Architecture / Convention / Task History);decision_ref 指涉 decision 表的记录。
- **source 字段是数据血缘**:每条知识知道来自哪个 workflow/task/审批。

### 4.2 CLI 契约

| 命令 | 说明 |
|---|---|
| `os memory add --company <id> --type lesson --title ... --content ... [--source ...] [--tags ...]` | 写一条知识(人沉淀或流程产出) |
| `os memory list --company <id> [--type lesson]` | 列出(可按类型过滤) |
| `os memory search --company <id> --q <词>` | FTS5 全文搜索,返回命中行 + snippet |

- 5.4 完整形态流程在节点结束时以 file-write 产出 + 可选 `memory add`(框架先行,lesson 由流程定义提供)。
- 人可用 `memory add` 沉淀公司级知识(客户洞察、教训、约定)。

## 5. Decision(5.2)

### 5.1 表设计(migration 0006)

```sql
CREATE TABLE decision (
    id          TEXT PRIMARY KEY,
    company_id  TEXT NOT NULL REFERENCES company(id),
    title       TEXT NOT NULL,
    kind        TEXT NOT NULL,   -- approval | goal | strategy | policy_change | capital | manual
    status      TEXT NOT NULL,   -- made | pending | executed
    body        TEXT NOT NULL DEFAULT '',
    decided_by  TEXT NOT NULL DEFAULT '',
    source      TEXT NOT NULL DEFAULT '',   -- approval:<id> / manual
    created_at  INTEGER NOT NULL,
    updated_at  INTEGER NOT NULL
);
```

### 5.2 自动记录(治理链闭环)

- **approval approve/reject/request_changes 时,自动插入一条 Decision**:kind=approval、source=approval:<id>、status=made、body=decision_note、decided_by=操作者。
- 治理链完整闭环:`Policy → Permission → Approval → Audit → Decision`(基线 §3.1 / 附录 C)。
- 人类还可 `decision add` 记录 goal / strategy / policy_change / capital 等公司级决策(方向基线 Phase 5「人负责 Vision / Strategy / Capital / Policy / Risk / Final Decisions」在 OS 中的留痕)。

### 5.3 CLI 契约

| 命令 | 说明 |
|---|---|
| `os decision add --company <id> --kind goal --title ... [--body ...]` | 记录一条人类决策 |
| `os decision list --company <id> [--kind approval]` | 列出 |
| `os decision show --id <id>` | 单条详情 |

## 6. 全景运营视图(5.3)

`os overview --company <id>` 单命令聚合(只读,join 现有表):

| 区块 | 内容 |
|---|---|
| Company | 名称 + vision |
| Capabilities | capability code × Agent 数量 |
| 运行中 Workflow | 最近 N 个 workflow + 各节点任务状态 |
| 待审批(醒目) | 所有 waiting_approval 任务 + approval 列表 |
| 最近 Decision | 最近 N 条 decision |
| 最近 Task | 最近 N 条 task(status/title/capability) |
| Memory 高亮 | 最近 N 条 lesson/knowledge 摘要 |

- 定位:让「一个人」一眼看清公司状态,并把注意力导向**真正需要人决策的事情**(待审批)。
- 实现:service 层聚合查询(复用现有 repository),无新表、无迁移。

## 7. 完整形态流程(5.4)

新品发布全流程 definition——跨全部 Capability 家族:

| step | capability | agent_role | 动作 | risk |
|---|---|---|---|---|
| idea | — | — | 输入新品创意(起始节点,skip) | — |
| research | research | market | 市场/竞品研究 → research.md | low |
| product | product | analyst | 产品分析/定位 → product.md | low |
| engineering | engineering | coding | 实现/原型 → engineering.md | low |
| qa | engineering | qa | 测试验证 → qa.md | low |
| marketing | marketing | campaign | 营销方案 → marketing.md | low |
| sales | sales | proposal | 报价/销售方案 → sales.md | low |
| approval | — | — | 审批门(发布前) | high |
| delivery | operations | quality | 交付(汇总 + git commit) | high |

- 示例定义存 `docs/phase5/examples/product-launch-workflow.json`(workspace 占位符,同 3.3/4.3)。
- 一键 `os workflow run <id>`:9 节点跨 6 Capability 家族(research / product / engineering / marketing / sales / operations)接力;审批门 approve/reject 语义沿用。
- **知识/决策沉淀**:approval 结果自动落 decision;流程结束人可 `memory add` 沉淀本次发布的 lesson(或节点内写入)。
- 冒烟环境需具备全部 Capability(沿用 2/3/4 的 agent 定义,一个 company 内建齐)。

## 8. 验收标准(按子阶段)

**5.1**
1. migration 0005 生效,`os memory add/list` 可用。
2. `os memory search` FTS5 命中中文/英文内容(unicode61),join 回 memory 完整行。

**5.2**
3. migration 0006 生效,`os decision add/list/show` 可用。
4. 一次 workflow 审批 approve + reject 后,`os decision list` 自动出现对应记录(source=approval:<id>)。

**5.3**
5. `os overview` 展示全部区块;含至少一个待审批项时区块醒目。

**5.4**
6. 新品发布 9 节点流程一键运行:跨 6 Capability 家族接力,归属正确。
7. 审批门 approve 续跑 / reject 中断;产出落 workspace + git 交付;decision 自动落库。

## 9. 已确认决策(待定稿确认)

- ✅ **Memory + Decision 为 Phase 5 主体**:补完基线 §1.4 第一阶段边界最后两个核心对象,使「Audit + Memory 沉淀公司资产」「自动记录 Decision」目标达成。
- ✅ **FTS5 可用**:modernc.org/sqlite v1.57.0 原生支持 FTS5(已用虚拟表 + MATCH 验证),`os memory search` 可行。
- ✅ **全景运营视图以 CLI 落**:`os overview` 聚合现有表,不新建表;前端/Dashboard 独立排期。
- ✅ **完整形态流程跨全部 Capability**:以新品发布为端到端载体,复用 2/3/4 的 Capability 与引擎,零代码新增(仅示例 definition)。
- ✅ **Memory 不接 LLM/RAG**:结构化 + FTS5 搜索,人用 CLI 查询;真实 RAG 留待后续。
- ✅ **向后兼容**:所有新表为增量迁移,不动已有表;新命令独立,不影响 0-4 冻结接口。
