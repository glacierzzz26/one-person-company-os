# Phase 2 — Engineering Capability 设计

> 执行蓝本。定义 Phase 2 的首个业务能力域:Engineering——在真实代码库上运行软件工程流程(Issue→Delivery)。
> 状态:✅ 定稿(2026-09-01)
> 关联:[项目详解](../项目详解.md) · [进度总表](../../进度总表.md) · 方向基线 `One-Person-Company-OS-Design-v1.0.md`(v1.1) · [Phase 1 设计](../phase1/design/execution-kernel.md)

## 1. 背景与目标

Phase 1 让 OS 具备**受控执行能力**(Task Queue / Tool / Sandbox / Workflow / Approval / Permission)。Phase 2 建立第一个完整 Capability:

> OS 能承接软件工程任务(Issue→Delivery),由 Engineering Capability 内的 Agent 在受控环境执行,人只做关键决策(审批/最终交付)。

方向基线 Phase 2 定义:Engineering 含 Architect / Coding / QA / Review 四个 Agent;流程 Issue→Analysis→Plan→Implementation→Test→Review→Approval→Delivery。

## 2. 范围与边界

**做**(方向基线 Phase 2 交付物):
- Engineering Capability(4 Agent + 8 阶段流程)
- Engineering Tools(git / file)
- Model Provider 抽象接口与配置预留(本阶段**不内置 LLM 调用**)
- AI-Fix 外部能力接入接口(本阶段提供接口 + mock,不真实集成)

**明确不做**:
- 内置 LLM 调用(需 API key 与成本,后续)
- React 前端(方向基线技术选型列 Phase 2,但 CLI 已可完整交付,前端独立排期)
- Metrics / Tracing(Prometheus / OpenTelemetry,后续)
- 其他 Capability(Research / Product / Business,Phase 3 / 4)
- Phase 2 为业务能力域,不改执行引擎内部机制(1.1-1.4 冻结)

## 3. 子阶段拆分(每子阶段一个可独立验收交付物)

| 子阶段 | 交付物 | 验收方式 |
|---|---|---|
| **2.1 Engineering Tools** | git / file Tool(注册表 + 权限 + workspace 边界 + 沙箱策略) | git/file 在 workspace 内可操作真实文件/仓库,调用落 Audit;未授权/越界被拒 |
| **2.2 Engineering Workflow** | 8 阶段流程定义 + 运行(Issue→Delivery) | 一键运行按序完成各节点 Task;Approval 阶段 high-risk 触发人工审批;失败中断 |
| **2.3 4 Agent + Provider 接入** | 4 Agent 就位 + Model Provider 抽象/配置 + AI-Fix 接入接口(mock) | agent 带 model 关联;config providers 解析/展示;aifix Tool 可调用(mock) |

每子阶段完成后写 `stages/<n>.md` 归档并更新进度总表;分支按 `dev → phase-2-engineering-capability` 迭代,阶段结束合回。

## 4. 领域模型扩展

### 4.1 Agent 执行模型(方向基线 5.2)

- Agent = Capability 内的**执行角色**:`role` + 模型提示(`model_hint`)+ 工具权限(permission 管控)。
- **Agent ≠ Model**:`Agent → Runtime → Model`。Phase 2 引入 Model Provider 抽象,Agent 通过 model_hint/配置关联 Provider,但**不产生真实模型调用**(框架先行)。
- Agent 可用工具 = 其 role 在 permission 中被授予的 action/resource 集合(Phase 1 校验已实现,默认拒绝)。

### 4.2 新增项

| 项 | 说明 |
|---|---|
| Model Provider(config) | YAML 声明 `providers`(name/type/model/endpoint/api_key_env);本阶段仅声明与解析/展示,不调用 |
| AI-Fix Tool | 注册为 "aifix" Tool,执行标准 Tool 接口;本阶段 **mock 实现**(静态响应),文档记录真实接入约定 |
| Issue(轻量) | 8 阶段起点:用 workflow 输入(title+description)表达,不新建实体表 |

### 4.3 Task / Workflow 复用

- 8 阶段每个节点 = 一个 Task(挂 Engineering Workflow),字段沿用 Phase 1:description(command)、agent_role、risk、workspace。
- **节点新增 `tool` 字段**(定稿后澄清 2026-09-01):指定执行该节点的 Tool 名,默认 `shell`;落库为 `task.tool_name`(迁移 0004)。多 Tool 并存后,引擎按 `tool_name` 取用 Tool(Phase 1 硬编码 shell 的假设取消)。
- Approval 阶段复用 Phase 1 审批机制(risk=high → waiting_approval)。

## 5. 存储设计

- 复用现有表(company/capability/agent/workflow/task/execution/approval),**暂不引入新表**(迁移 0004 预留)。
- Model Provider 只存 config(YAML),不落库(本阶段无状态);若 2.3 需要 Agent↔Provider 绑定落库,再做迁移。

## 6. Engineering Tools(2.1)

每个 Tool 走 1.2 的 Tool 接口 + Registry(可热插拔),权限校验与 Audit 自动生效。

| Tool | Permission | Risk | 执行方式 | 说明 |
|---|---|---|---|---|
| shell | execute/shell | medium | Docker 无网络(现状) | 通用命令 |
| git | execute/git | low(本地)/high(网络) | **宿主** workspace 内 Git CLI wrapper | 白名单本地子命令 init/status/add/commit/diff/log/show/branch/config/rev-parse;clone/pull/push/fetch 等网络操作返回不支持(后续 + 审批) |
| file-read | read/file | low | **宿主** workspace 内读文件 | 受 workspace 边界约束,禁止越界 |
| file-write | write/file | low | **宿主** workspace 内写文件 | 受 workspace 边界约束,禁止越界 |

- file 按最小权限拆为 file-read / file-write(定稿后澄清 2026-09-01):读只读角色(如 review)只需 read/file,不给写权限;Tool 接口 Permission 为单 action/resource,无法在单 Tool 内表达 read+write。
- git/file 在**宿主**执行(workspace 即宿主机路径),绕过 Docker 无网络限制;shell 保持无网络沙箱。
- **file tool 强制 workspace 边界**:目标路径经 filepath.Clean + filepath.Rel 校验,必须位于 task.workspace_path 之下,防目录穿越。

## 7. 8 阶段 Workflow(2.2)

Engineering 流程 definition(节点:agent_role / command / risk):

| 阶段 | Agent role | 动作(框架先行:命令由定义提供) | risk |
|---|---|---|---|
| issue | — | 输入需求(title/description,起始节点不作执行) | — |
| analysis | architect | 分析任务、读上下文、产出说明 | low |
| plan | architect | 生成实现计划,写入 workspace | low |
| implementation | coding | 在 workspace 改代码 + git 操作 | medium |
| test | qa | 运行测试/校验 | medium |
| review | review | 读取 diff,产出评审 | low |
| approval | — | 审批节点(high-risk 触发,人 approve/reject) | high |
| delivery | coding | 交付产物(git commit / tag) | high(需审批) |

- 复用 Phase 1 引擎:顺序执行、节点失败默认中断、审批触发、全程审计。
- 验收载体:真实本地仓库 + Engineering workflow,一条工程任务从 issue 跑到 delivery。

## 8. Model Provider + AI-Fix 接入(2.3)

- **Provider 抽象**(Go 接口):`Generate(ctx, request) (response, error)`;本阶段所有 provider 为 stub(返回 not implemented 或 mock 内容)。
- **config/providers**:YAML 声明 provider(name/type/model/endpoint/api_key_env);CLI `os provider list` 展示。
- **Agent 关联**:CLI `os agent add --model <model>` 存 model_hint;provider 解析自 config。
- **AI-Fix 接入**:注册 "aifix" Tool,契约 = 标准 Tool 接口;本阶段 mock 实现,文档记录真实接入端点约定(留待后续真实集成)。

## 9. 验收标准(按子阶段)

**2.1**
1. git/file Tool 注册可见(`os tool list`),在 workspace 内可操作真实文件/本地仓库,结果落 Audit。
2. 未授权 role 调用 git/file 被拒;file 越界路径被拒(目录穿越防护)。

**2.2**
3. 定义 Engineering 8 阶段 workflow,一键运行:Issue→Delivery 各节点按序完成;Approval 阶段 high-risk 触发人工审批,approve 后继续、reject 后中断。
4. 节点失败默认中断(承 1.3)。

**2.3**
5. `os agent add --model` 生效;config providers 解析并 `os provider list` 展示。
6. aifix Tool 注册可调用(mock);接入约定文档化。

## 10. CLI 命令契约(在 Phase 1 之上新增/增强)

| 命令 | 说明 |
|---|---|
| `os tool list` | 展示 shell / git / file / aifix 等 Tool |
| `os agent add --model <model>` | Agent 关联模型标识 |
| `os provider list` | 展示 Model Provider 配置(解析自 YAML) |
| `os workflow run <id>` | 运行 Engineering 流程(承 1.3) |

通用约定沿用 Phase 0/1(对齐文本表格、错误非 0 退出码、`config/os.yaml` + `--db` 覆盖)。

## 11. 已确认决策(2026-09-01 定稿)

- ✅ Agent 智能:**框架先行,不接 LLM**(本阶段);Model Provider 接口预留,后续接模型/AI-Fix。
- ✅ **保持 CLI**,不引入 React 前端。
- ✅ **子阶段拆分**:2.1 Engineering Tools → 2.2 Engineering Workflow → 2.3 4 Agent + Provider 接入。
