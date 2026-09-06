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

// ===================== repo (研发仓库 · 通道 B) =====================
export interface Repo {
  id: string;
  company_id: string;
  name: string;
  repo_url: string;
  workspace_path: string;
  created_at: number;
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

export interface AddRepoReq {
  name: string;
  repo_url: string;
  workspace?: string;
}

export interface DecideApprovalReq {
  decision: 'approve' | 'reject' | 'changes';
  note?: string;
}
