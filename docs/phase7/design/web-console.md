# Phase 7 — 运营控制台数据通道(Web JSON API)设计

> 阶段方向级设计,定稿冻结后按它执行。子阶段:7.1 JSON API(本阶段);7.2 React+TS+AntD 控制台(未来)。
> 关联:愿景基线 `One-Person-Company-OS-Design-v1.0.md` §4.2(HTTP=net/http+Chi;Frontend=React+TypeScript+Ant Design)。

## 一、定位与范围

CLI `os overview` 目前是唯一全景入口,且只面向本地终端。要让「一人操作台」在浏览器/手机/脚本里可用,
先补一条 **`/api/v1` JSON 读/写数据通道**,复用 Phase 0–6 全部 `service` 方法(单一事实源,零语义改动);
再在其上长 React SPA(7.2)。

- **7.1 范围**:`os server`(chi)内新增 `/api/v1` 路由——overview/组织/任务/审批/决策/记忆/审计/
  模型端点池/通道 B 仓库账本的 JSON 读 + 核心写(审批决策、任务建、决策记、记忆沉淀、端点/仓库配)。
- **不做**:UI、分页、CORS(7.1 同源;7.2 SPA 若 dev 分离 origin 再加)、多用户/RBAC、
  `task run / queue work / workflow run`(长时阻塞,驱动留 CLI 与后台轮询)。
- **单二进制定位不破**:无新 go.mod 依赖(chi v5.2.5 已在)、无新迁移、无 node、引擎语义零改动。

## 二、认证与安全姿态

- `os server` 因收 GitHub webhook(通道 B)必须公网可达 → `/api/v1` 认证**不能**靠改绑 localhost。
- `/api/v1` 路由组挂 **bearer 中间件**:设 `OS_API_TOKEN` 环境变量 → 要求 `Authorization: Bearer <token>`
  (constant-time 比较);**未设 = 开放**。文档/日志明确:暴露到非回环地址时务必设 token。
  webhook(`/api/webhook/github`)不受影响(GitHub 侧 secret 已在 webhook 内校验)。
- token/secret(端点 token、飞书 secret 等)绝不入 JSON 响应与日志(端点 token 只回 `TokenEnc` 是否非空)。

## 三、JSON 约定(通用)

- 信封:成功 `{"ok":true,"data":<…>}`;错误 `{"ok":false,"error":{"code","message"}}` + 恰当 HTTP 状态
  (`sql.ErrNoRows→404 not_found`;其余→500,带服务层报文)。
- 时间:`int64` unix 秒(与存储模型一致;不发明 RFC3339 转换,前端自行格式化)。
- ID:完整 UUID(无 CLI 短 ID 歧义);路径/查询一律全量 ID。
- 字段:snake_case;形状 = 各域模型/服务视图原样(`data` 直接 json 序列化)。给被序列化结构体补
  json tag 作为唯一事实源(见 §六 结构体清单)。可空引用(如 `task.capability_id`)序列化为 `null`。

## 四、页面×接口矩阵(产品愿景 → 后端映射)

未来控制台页面(7.2 长 SPA 时逐页接此契约):

| 页面(未来控制台) | 数据需求 | 对应端点 | 状态 |
|---|---|---|---|
| 公司列表 / 选择 | 全公司列表、新建公司 | `GET/POST /api/v1/companies`;`GET /api/v1/companies/{id}` | ✅ |
| 全景 Dashboard | company / capabilities×agents / workflow 状态计数 / **RD 研发部块(熔断·待审批·子任务·账本)** / 待审批 / 决策 / 任务 / 记忆高亮 | `GET /api/v1/companies/{id}/overview` | ✅ |
| 能力域 / 组织 | capability 列表、agents | `GET …/capabilities`、`GET /api/v1/capabilities/{capID}/agents` | ✅ |
| 工作流 | workflow 列表(定义只读) | `GET …/workflows` | ✅(run ❌ 归 CLI,阻塞) |
| 任务中心 | 任务列表(筛选)/详情/执行记录/新建工程请求 | `GET /api/v1/tasks…`、`GET …/tasks/{id}[/executions]`、`POST /api/v1/tasks` | ✅(run/queue ❌ 归 CLI) |
| 审批中心(人的决策) | 全局待审批、单条、**决策操作** | `GET /api/v1/approvals?status=`、`GET …/approvals/{id}`、`POST …/approvals/{id}/decision` | ✅ |
| 决策中心 | 公司决策列表(按 kind)/详情/记决策 | `GET …/decisions?kind=`、`GET /api/v1/decisions/{id}`、`POST …/decisions` | ✅ |
| 知识库 | 记忆列表(按 type)/全文检索/沉淀 | `GET …/memories`、`GET …/memories/search?q=`、`POST …/memories` | ✅ |
| 审计 | 审计流(按实体) | `GET /api/v1/audit?entity=` | ✅ |
| 模型端点池(管理) | 端点列表/详情/新增/选模型/拉模型 | `GET …/endpoints`、`GET /api/v1/endpoints/{id}`、`POST /api/v1/endpoints`、`POST …/{id}/select`、`POST …/{id}/models` | ✅ |
| 通道 B(仓库账本) | 仓库列表/登记/手动同步 | `GET/POST …/repos`、`POST …/intake/sync` | ✅ |
| 任务驱动 / 队列 | 认领执行 | — | ❌ 不经 API(两条 CLI/后台路径:`os queue work` 手动排空 / server `--queue-work` 自驱,7.2 已提供后者) |
| 模型调用测试 | 直接对话 | — | ❌ 无(CLI/未来控制台单独子阶段) |

## 五、端点契约(7.1,逐字核对 service 签名)

> 字段形状 = 各资源模型字段(见 §六)。`[]` 中为 URL 路径参数。

### 5.1 公司 / 组织
- `GET /api/v1/companies` → `ListCompanies`(`[]company.Company`)
- `POST /api/v1/companies` `{name, vision}` → `CreateCompany` → 201
- `GET /api/v1/companies/{id}` → `GetCompany`
- `GET /api/v1/companies/{id}/overview` → `Overview`(**旗舰**,含 RD 块)
- `GET /api/v1/companies/{id}/capabilities` → `ListCapabilities`
- `GET /api/v1/capabilities/{capID}/agents` → `ListAgents`
- `GET /api/v1/companies/{id}/workflows` → `ListWorkflows`

### 5.2 任务 / 执行
- `GET /api/v1/tasks?company=&status=&risk=&attempt=` → `ListTasks`(`[]task.Task`)
- `GET /api/v1/tasks/{id}` → `GetTask`
- `GET /api/v1/tasks/{id}/executions` → `ListExecutions`(`[]execution.Execution`)
- `POST /api/v1/tasks` → `CreateTask(TaskParams)`。请求字段(对应 `service.TaskParams`):
  `company_id`(必填)`, capability_id?, workflow_id?, agent_id?, title`(必填)`, description?,
  tool_name`(默认 shell,可 engineering)`, risk?`(默认 low)`, max_attempts?, timeout_sec?, workspace?,
  parent_task_id?, writer_endpoint_id?, reviewer_endpoint_id?` → 201

### 5.3 审批(核心写)
- `GET /api/v1/approvals?status=` → `ListApprovals`(空 status = 全部;全局,不分公司)
- `GET /api/v1/approvals/{id}` → `GetApproval`
- `POST /api/v1/approvals/{id}/decision` `{decision: approve|reject|changes, note}` → `DecideApproval`
  → 放行任务回队 / 拒绝打回;自动落 decision(kind=approval),与 CLI 同语义

### 5.4 决策 / 记忆 / 审计
- `GET /api/v1/companies/{id}/decisions?kind=` · `GET /api/v1/decisions/{id}` ·
  `POST /api/v1/companies/{id}/decisions` `{kind?, status?, title, body?}`(kind/status 缺省同 CLI)
- `GET /api/v1/companies/{id}/memories?type=` · `GET /api/v1/companies/{id}/memories/search?q=`(必填 q)·
  `POST /api/v1/companies/{id}/memories` `{type?, title, content?, source?, tags?}`
- `GET /api/v1/audit?entity=` → `ListAudits`(`[]audit.Audit`)

### 5.5 模型端点池 / 通道 B(admin)
- `GET /api/v1/companies/{id}/endpoints` · `GET /api/v1/endpoints/{id}`
- `POST /api/v1/endpoints` `{company_id, name, base_url, token?, proto?=auto}`(token 加密落库,回包不含明文)
- `POST /api/v1/endpoints/{id}/select` `{model?, role?}`(model 空 → 清选定,语义同 CLI select)
- `POST /api/v1/endpoints/{id}/models` → `FetchEndpointModels`(网络触发,列出可用模型)
- `GET /api/v1/companies/{id}/repos` · `POST /api/v1/companies/{id}/repos` `{name, repo_url, workspace?}`
- `POST /api/v1/companies/{id}/intake/sync` → `SyncRepos`(`[]IntakeResult`,触发一次通道 B)

## 六、被序列化结构体(json tag 清单,snake_case)

给以下域/视图结构体补 `json` tag(返回原样序列化,单一事实源;CLI 用 fmt 输出不受影响):

| 包 / 类型 | 主要字段(→JSON) |
|---|---|
| `company.Company` | id, name, vision, created_at, updated_at |
| `capability.Capability` | id, company_id, code, name, description, created_at, updated_at |
| `agent.Agent` | id, capability_id, name, role, model_hint, created_at, updated_at |
| `workflow.Workflow` | id, company_id, name, description, definition, created_at, updated_at |
| `task.Task` | id, company_id, capability_id?, workflow_id?, agent_id?, title, description, tool_name, status, priority, attempt, risk, q_status, lease_worker_id, lease_until, max_attempts, timeout_sec, last_error, result, workspace_path, parent_task_id?, round_no, conflict_count, writer_endpoint_id?, reviewer_endpoint_id?, created_at, updated_at |
| `approval.Approval` | id, task_id, risk, reason, status, requested_by, decided_by, decision_note, created_at, decided_at? |
| `decision.Decision` | id, company_id, title, kind, status, body, decided_by, source, created_at, updated_at |
| `memory.Memory` | id, company_id, type, title, content, source, tags, created_at, updated_at |
| `audit.Audit` | id, entity_type, entity_id, action, actor, detail, created_at |
| `endpoint.Endpoint` | id, company_id, name, base_url, token_enc, proto, vendor, selected_model, role, status, models_cache, created_at, updated_at |
| `repo.Repo` / `repo.IssueSync` | repo: id, company_id, name, repo_url, workspace_path, created_at |
| `execution.Execution` | id, task_id, worker_id, attempt, status, started_at, finished_at?, result, error, created_at |
| `service.Overview` + 视图 | Overview{company, capabilities[], workflows[], pending_approvals[], recent_decisions[], recent_tasks[], memory_highlights[], rd?};CapabilityView{code,name,agent_count};WorkflowView{workflow,statuses};ApprovalView{approval,task_title};RDOverview{capability_id,total,by_status,fused[],waiting[],subtask_count,ledger,ledger_seen};RDTask{id,title,status,risk,round,conflict,parent_task_id?,reason} |

命名:`*string` → `null`(无 omitempty,诚实空态);map 原样(`by_status`/`ledger` 键为状态/处置字符串)。

## 七、实现要点

- 新 `internal/server/api.go`:包级信封 helper `writeAPI(w, code, data)` /
  `writeErr(w, code, errCode, msg)`,`sql.ErrNoRows → 404 not_found`;`bearer` 中间件读 `OS_API_TOKEN`
  (subtle.ConstantTimeCompare,未设直通);handler 全部 thin wrapper 直调 `svc`,按 §五 分组。
- `internal/server/server.go` `Handler()` 内加 `r.Route("/api/v1", func(r chi.Router){ r.Use(s.apiAuth); … })`;
  `/healthz` 与 `/api/webhook/github` 不动。`writeJSON`(既有)继续服务非 /api/v1 路径。
- `OS_API_TOKEN` 在 CLI root 读一次注入 Server 字段(`apiToken string`),便于测试直构 Server。
- 写请求用端点内联小 struct 解析(JSON strict;缺必填 → 400)。

## 八、子阶段

- **7.1(已完成,冻结)**:JSON API + 契约冻结 + httptest + 离线 curl 冒烟(stages/1.md)。
- **7.2(已完成,冻结)**:控制台 UI 设计定稿([console-ui.md](console-ui.md),方向级)+ 可交互高保真
  [原型](console-ui-prototype.html)。本轮**回填两处契约缺口**(stages/2.md):
  ① **actor 来源贯通** —— service 全套写方法出 `*As` 变体,/api/v1 写操作审计 actor 记
  `human:console`(CLI 保持 `human:cli`,互不污染);② **队列认领循环** —— `os server --queue-work`
  [默认关,开即每间隔认领排空 `LeaseAndExecute`,worker=server],控制台建的任务可被 server 自动消费
  (审计 `runtime:server`),不必再手动 `os queue work`。契约其余端点、语义、字段零改动。
- **7.3(未来)**:React + TypeScript + Ant Design 控制台(愿景 §4.2 基线),按 console-ui.md 交互规格与
  设计令牌实现,消费本契约;embed 进单二进制或分离部署由用户届时定;CORS 若 dev 分离 origin 再加。
  分页/更多图表/实时刷新届时按需扩展。

## 九、验收(7.1)

- `go build ./...`、`go test ./...` 全绿。
- httptest:真 svc(temp sqlite)断言 overview 含 RD 块、approval decision 写成功、token 401/200、404。
- 离线 curl 冒烟(OS_ENGINE_MODE=scripted + fixture):seed → 起 server → healthz / companies /
  overview(RD 块 fused/waiting/ledger)/ approvals decision(任务回队+decision 落库)/ memories search;
  `OS_API_TOKEN` 二次起:无头 401 / 带头 200。
- 归档 stages/1.md + 进度总表登记 + 使用说明补 `server` JSON 面与 curl 示例。
