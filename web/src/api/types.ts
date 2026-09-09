// 与后端 Go 域结构体 json tag 逐字对齐(数据诚实:不照原型 mock 发明字段)。
// 时间 = int64 unix 秒;可空字段:指针型 → string|null / number|null;空字符串字符串字段如实为 string。

// ===================== 公司 =====================
export interface Company {
  id: string;
  name: string;
  vision: string;
  created_at: number;
  updated_at: number;
}

// ===================== capability / agent / workflow =====================
export interface Capability {
  id: string;
  company_id: string;
  code: string;
  name: string;
  description: string;
  created_at: number;
  updated_at: number;
}

export interface Agent {
  id: string;
  capability_id: string;
  name: string;
  role: string;
  model_hint: string;
  created_at: number;
  updated_at: number;
}

export interface Workflow {
  id: string;
  company_id: string;
  name: string;
  description: string;
  definition: string;
  created_at: number;
  updated_at: number;
}

// overview 视图封装
export interface CapabilityView {
  code: string;
  name: string;
  agent_count: number;
}

export interface WorkflowView {
  workflow: Workflow;
  statuses: Record<string, number>; // task status → count
}

// ===================== task / execution =====================
export type TaskStatus = 'pending' | 'running' | 'waiting_approval' | 'completed' | 'failed';
export type Risk = 'low' | 'medium' | 'high';

export interface Task {
  id: string;
  company_id: string;
  capability_id: string | null;
  workflow_id: string | null;
  agent_id: string | null;
  title: string;
  description: string;
  tool_name: string;
  status: TaskStatus;
  priority: number;
  attempt: number;
  risk: Risk;
  q_status: string; // ready | leased | running | waiting_approval | completed | failed
  lease_worker_id: string;
  lease_until: number;
  max_attempts: number;
  timeout_sec: number;
  last_error: string;
  result: string;
  workspace_path: string;
  parent_task_id: string | null;
  round_no: number;
  conflict_count: number;
  writer_endpoint_id: string | null;
  reviewer_endpoint_id: string | null;
  test_endpoint_id: string | null; // test 判读槽(8.4 分槽;默认 standard 档)
  project_id: string | null; // Phase 10.1:流水线 run 产物挂项目;空 = 非流水线任务
  pipeline_id: string | null; // Phase 10.2:run 反链流水线(形态/裁决展示);空 = 非流水线 run 产物
  created_at: number;
  updated_at: number;
}

export type ExecutionStatus = 'running' | 'completed' | 'failed' | 'timeout';

export interface Execution {
  id: string;
  task_id: string;
  worker_id: string;
  attempt: number;
  status: ExecutionStatus;
  started_at: number;
  finished_at: number | null;
  result: string;
  error: string;
  created_at: number;
}

// ===================== approval =====================
export type ApprovalStatus = 'pending' | 'approved' | 'rejected' | 'changes';

export interface Approval {
  id: string;
  task_id: string;
  risk: Risk;
  reason: string;
  status: ApprovalStatus;
  requested_by: string;
  decided_by: string;
  decision_note: string;
  created_at: number;
  decided_at: number | null;
}

// overview.pending_approvals 封装(带任务标题)
export interface ApprovalView {
  approval: Approval;
  task_title: string;
}

// ===================== decision =====================
export const DECISION_KINDS = ['approval', 'goal', 'strategy', 'policy_change', 'capital', 'manual'] as const;
export type DecisionKind = (typeof DECISION_KINDS)[number];

export const DECISION_STATUSES = ['made', 'pending', 'executed'] as const;
export type DecisionStatus = (typeof DECISION_STATUSES)[number];

export interface Decision {
  id: string;
  company_id: string;
  title: string;
  kind: DecisionKind;
  status: DecisionStatus;
  body: string;
  decided_by: string;
  source: string;
  created_at: number;
  updated_at: number;
}

// ===================== memory =====================
export const MEMORY_TYPES = [
  'lesson',
  'knowledge',
  'project_context',
  'decision_ref',
  'architecture',
  'convention',
  'task_history',
] as const;
export type MemoryType = (typeof MEMORY_TYPES)[number];

export interface Memory {
  id: string;
  company_id: string;
  type: MemoryType;
  title: string;
  content: string;
  source: string; // 血缘: workflow:<id> / task:<id> / approval:<id> / manual
  tags: string; // 空格分隔
  created_at: number;
  updated_at: number;
}

// ===================== audit =====================
export interface Audit {
  id: string;
  entity_type: string;
  entity_id: string;
  action: string;
  actor: string;
  detail: string;
  created_at: number;
}

// ===================== endpoint =====================
export interface Endpoint {
  id: string;
  company_id: string;
  name: string;
  base_url: string;
  token_enc: string; // 密文(enc:v1:);空 = 无鉴权 —— 永不回显明文
  proto: string; // auto | anthropic | openai
  vendor: string;
  selected_model: string;
  role: string; // pool | planner | standby
  tier: string; // frontier | standard | cheap(8.4 档位;与 role 正交)
  status: string; // active | disabled
  models_cache: string; // json:最近一次 /v1/models 原始返回(对象/数组,可空)
  created_at: number;
  updated_at: number;
}

export interface ModelInfo {
  id: string;
}

// ===================== code source(D7:仓库收敛为项目的代码源绑定) =====================
// server /projects/{id} 响应 projectView.code_source(可 null);refresh-code 端点返回同一形状。
export interface CodeSource {
  repo_id: string;
  repo_url: string;
  owner: string;
  repo: string;
  bound: boolean; // 绑到项目(非 legacy)
  has_github: boolean; // remote 可解析 GitHub owner/repo(通道 B 可路由)
}

export interface IntakeAsk {
  owner: string;
  name: string;
  number: number;
  note: string;
}

// 单仓库一次同步结果
export interface IntakeResult {
  repo: string;
  issues_seen: number;
  already: number;
  by_disp: Record<string, number>; // disposition → 条数
  created_tasks: string[];
  asks: IntakeAsk[];
}

// ===================== project / pipeline(Phase 10.1) =====================
export interface Project {
  id: string;
  company_id: string;
  name: string;
  root_path: string; // 整项目 git 仓库根(layout A);删除不碰磁盘
  description: string;
  created_at: number;
  updated_at: number;
  // D7 加性富化(server projectViewOf):代码源 = 项目 git remote 自动认领的 repos 绑定行;无 GitHub origin → null。
  code_source: CodeSource | null;
}

export const PIPELINE_KINDS = ['bugfix', 'develop', 'ops_patrol'] as const;
export type PipelineKind = (typeof PIPELINE_KINDS)[number];

export type PipelineStatus = 'active' | 'disabled';

export interface Pipeline {
  id: string;
  project_id: string;
  name: string;
  kind: PipelineKind;
  description: string; // 意图(自然语言;run 无 request 时即 writer 请求)
  risk: Risk;
  status: PipelineStatus;
  schedule: string; // 10.2 才解析;本期恒 ''
  plan_policy: PlanPolicy; // 10.4:adaptive(缺省=执行中生成)| synthesize(合成=frontier 先行合成)
  created_at: number;
  updated_at: number;
}

// ===================== overview 聚合 =====================
export interface RDTask {
  id: string;
  title: string;
  status: TaskStatus;
  risk: Risk;
  round: number;
  conflict: number;
  parent_task_id: string | null;
  reason: string; // waiting_approval 原因(熔断/ask/高险门)
}

export interface RDOverview {
  capability_id: string;
  total: number;
  by_status: Record<string, number>; // status → count
  fused: RDTask[];
  waiting: RDTask[];
  subtask_count: number;
  ledger: Record<string, number>; // issue_sync disposition 分布
  ledger_seen: number; // 账本总条数
}

export interface Overview {
  company: Company;
  capabilities: CapabilityView[];
  workflows: WorkflowView[];
  pending_approvals: ApprovalView[];
  recent_decisions: Decision[];
  recent_tasks: Task[];
  memory_highlights: Memory[];
  rd: RDOverview | null; // 无 engineering capability → null
}

// ===================== 请求体 =====================
export interface CreateCompanyReq {
  name: string;
  vision: string;
}

export interface CreateTaskReq {
  company_id: string;
  capability_id?: string;
  workflow_id?: string;
  agent_id?: string;
  title: string;
  description?: string;
  tool_name?: string;
  risk?: Risk;
  max_attempts?: number;
  timeout_sec?: number;
  workspace?: string;
  parent_task_id?: string;
  writer_endpoint_id?: string;
  reviewer_endpoint_id?: string;
  test_endpoint_id?: string;
}

export interface CreateDecisionReq {
  kind: DecisionKind;
  title: string;
  body?: string;
  status?: DecisionStatus; // 空 = made
}

export interface CreateMemoryReq {
  type: MemoryType;
  title: string;
  content?: string;
  source?: string;
  tags?: string; // 空格分隔(后端原样存)
}

export interface AddEndpointReq {
  company_id: string;
  name: string;
  base_url: string;
  token?: string;
  proto?: string;
}

export interface DecideApprovalReq {
  decision: 'approve' | 'reject' | 'changes';
  note?: string;
}

export interface CreateProjectReq {
  name: string;
  root_path: string; // 绝对路径:须已 git,或空/不存在(OS git init 或 clone);非空非 git → 400
  description?: string;
  repo_url?: string; // 可空:root 空/不存在时 OS 自动 clone 该 GitHub 地址并绑定代码源
}

export interface CreatePipelineReq {
  name: string;
  kind?: PipelineKind; // 空 = bugfix
  description?: string; // 意图
  risk?: Risk; // 空 = medium
  schedule?: string; // 10.2:cron 五段 分时日月周;空/off = 不调度
  plan_policy?: PlanPolicy; // 10.4:adaptive(缺省)| synthesize;空 = adaptive
}

export const PLAN_POLICIES = ['adaptive', 'synthesize'] as const;
export type PlanPolicy = (typeof PLAN_POLICIES)[number];

export interface RunPipelineReq {
  request?: string; // 本次运行意图;缺省 = pipeline.description
}

// POST /pipelines/{id}/run 响应:{task_id, project_id, pipeline_id, task}
export interface RunPipelineResult {
  task_id: string;
  project_id: string | null;
  pipeline_id: string;
  task: Task;
}

// PUT /pipelines/{id} 改调度请求/响应:schedule 空/off = 停调度(非法 cron → 400)。
export interface UpdatePipelineScheduleReq {
  schedule: string;
}

// GET /projects/{projectID}/patrol/{taskID} 巡检报告正文读端点响应(仅已完成 patrol run 可读)。
export interface PatrolReport {
  task_id: string;
  path: string; // 相对项目根 patrol/<taskID>.md
  truncated: boolean; // 超 64KiB 截断标记
  content: string; // 纯文本正文(限长)
}

// ===================== run 计划账本(Phase 10.3,契约 phase-plan-contract.md §3.4)=====================
// GET /tasks/{id}/plan 只读端点响应:{task_id} + plan(可 null = 非流水线 run / 历史 run 无计划)。
export interface PlanPhase {
  seq: number;
  kind: 'do' | 'accept' | 'dispose'; // 阶段类型
  title: string; // 人类可读阶段目标(round/报告名等)
  allocator: 'delegate' | 'os' | 'judge' | 'planner' | 'manual'; // 阶段执行者
  status: 'pending' | 'running' | 'ok' | 'fail' | 'skipped';
  evidence: string; // 产出引用/摘要(报告 rel/verdict/exec id/commit;不整存大产出)
  note: string;
  started_at: number | null;
  finished_at: number | null;
}

export interface TaskPlan {
  kind: 'patrol' | 'engineering'; // plan 形态
  materialized: 'upfront' | 'grow'; // 落账时机
  plan_policy?: PlanPolicy; // 10.4:run 所属流水线 adaptive|synthesize;流水线已删 → 缺省(undefined)
  created_at: number;
  updated_at: number;
  phases: PlanPhase[]; // 有序阶段(seq 升序;grow 计划执行中追加)
}

export interface TaskPlanResponse {
  task_id: string;
  plan: TaskPlan | null;
}

// ===================== setup / console token(Phase 9.2)=====================
export interface SetupStatus {
  initialized: boolean; // console_token_hash != '' = 已首启
}

export interface SetupReq {
  console_token: string; // ≥8 字符;服务端只存 sha256 哈希
  old_endpoint_key?: string; // 可选:存量 enc:v1 端点的 OS_ENDPOINT_KEY,首启一次性 re-key
}

export interface SetupResult {
  initialized: boolean;
  master_key: string; // 仅此一次返回(服务端不落库可读通道);本地保存后即消失
}

export interface RotateConsoleTokenReq {
  new_token: string;
}

export interface RotateConsoleTokenResult {
  rotated: boolean;
}

// ===================== settings(Phase 9.3 配置治理,契约 runtime-knobs-web.md §3.6/§3.7)=====================
// 与后端 settings 包 json tag 逐字对齐;console_token_hash 永不回传(只给 console_token_set 掩码)。

export interface GlobalSettings {
  engine_mode_default: string; // Web 恒 'live'(scripted 仅 CLI/env 测试 seam,Web/API 拒写)
  agent_cli_default: 'claude' | 'codex';
  digest_time: string; // '' = 关;否则 "HH:MM"
  http_port: number;
  poll_min: number;
  queue_work: boolean;
  queue_interval_sec: number;
  schedule_poll_sec: number; // 10.2:流水线到点触发轮询间隔(秒;0 = 关,与 queue_work 正交)
  console_token_set: boolean; // 掩码:console_token_hash 非空(初始化已设令牌)
  updated_at: number;
}

// 全局部分更新体(字段缺省 = 不改该维度)。engine_mode_default 只读 live,**不进请求**。
export interface UpdateGlobalSettingsReq {
  agent_cli_default?: 'claude' | 'codex';
  digest_time?: string; // '' / "off" = 关;否则 "HH:MM"
  http_port?: number;
  poll_min?: number;
  queue_work?: boolean;
  queue_interval_sec?: number;
  schedule_poll_sec?: number; // >=0;0 = 关
}

// company 覆盖行:指针字段 null = 继承 global(无覆盖行 = 全 null)。与后端 CompanySetting 对齐。
export interface CompanySettings {
  company_id: string;
  engine_mode: 'live' | 'scripted' | null; // 只读展示(Web 不写 engine_mode);null=继承
  agent_cli: 'claude' | 'codex' | null;
  issue_source: 'github' | 'fixture' | null;
  issue_fixture_path: string | null;
  updated_at: number;
}

// company 部分覆盖(显式空串 = 清该维度覆盖回退继承)。
export interface UpdateCompanySettingsReq {
  agent_cli?: string; // '' = 清覆盖
  issue_source?: string; // '' = 清覆盖(默认 github);'github' | 'fixture'
  issue_fixture_path?: string; // '' = 清
}

// 公司机密元数据(值明文永不出 API;只出 {id,set,updated_at})。
export interface SecretMeta {
  id: string; // github_token | feishu_webhook | feishu_secret(白名单,后端 KnownSecretIDs)
  set: boolean;
  updated_at: number;
}

export interface RotateMasterKeyResult {
  master_key: string; // 新主密钥,仅此一次返回
  endpoints_rekeyed: number;
  secrets_rekeyed: number;
}
