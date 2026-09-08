// 每 API 端点薄 helper(全部走信封 client;路径与 internal/server/api.go 路由逐字对应)。
import { request } from './client';
import type {
  AddEndpointReq,
  Agent,
  Approval,
  Audit,
  Capability,
  CodeSource,
  Company,
  CompanySettings,
  CreateCompanyReq,
  CreateDecisionReq,
  CreateMemoryReq,
  CreatePipelineReq,
  CreateProjectReq,
  CreateTaskReq,
  DecideApprovalReq,
  Decision,
  Endpoint,
  Execution,
  GlobalSettings,
  IntakeResult,
  Memory,
  ModelInfo,
  Overview,
  PatrolReport,
  Pipeline,
  Project,
  RotateConsoleTokenReq,
  RotateConsoleTokenResult,
  RotateMasterKeyResult,
  RunPipelineReq,
  RunPipelineResult,
  SecretMeta,
  SetupReq,
  SetupResult,
  SetupStatus,
  Task,
  TaskPlanResponse,
  UpdateCompanySettingsReq,
  UpdateGlobalSettingsReq,
  UpdatePipelineScheduleReq,
  Workflow,
} from './types';

// ---- 公司 ----
export const listCompanies = () => request<Company[]>('/api/v1/companies');
export const createCompany = (body: CreateCompanyReq) =>
  request<Company>('/api/v1/companies', { method: 'POST', body });

// ---- 总览 / capability / workflow / agents ----
export const getOverview = (companyId: string) =>
  request<Overview>(`/api/v1/companies/${companyId}/overview`);
export const listCapabilities = (companyId: string) =>
  request<Capability[]>(`/api/v1/companies/${companyId}/capabilities`);
export const listAgents = (capId: string) => request<Agent[]>(`/api/v1/capabilities/${capId}/agents`);
export const listWorkflows = (companyId: string) =>
  request<Workflow[]>(`/api/v1/companies/${companyId}/workflows`);

// ---- 任务 / 执行 ----
export interface TaskListParams {
  company?: string;
  status?: string;
  risk?: string;
  attempt?: number;
}
export const listTasks = (p: TaskListParams = {}) => {
  const qs = new URLSearchParams();
  if (p.company) qs.set('company', p.company);
  if (p.status) qs.set('status', p.status);
  if (p.risk) qs.set('risk', p.risk);
  if (p.attempt !== undefined && p.attempt > 0) qs.set('attempt', String(p.attempt));
  const s = qs.toString();
  return request<Task[]>(`/api/v1/tasks${s ? `?${s}` : ''}`);
};
export const getTask = (id: string) => request<Task>(`/api/v1/tasks/${id}`);
export const listExecutions = (taskId: string) =>
  request<Execution[]>(`/api/v1/tasks/${taskId}/executions`);
export const getTaskPlan = (taskId: string) =>
  request<TaskPlanResponse>(`/api/v1/tasks/${taskId}/plan`); // 10.3 只读;plan 可 null
export const createTask = (body: CreateTaskReq) => request<Task>('/api/v1/tasks', { method: 'POST', body });

// ---- 审批 ----
export const listApprovals = (status?: string) => {
  const s = status && status !== 'all' ? `?status=${encodeURIComponent(status)}` : '';
  return request<Approval[]>(`/api/v1/approvals${s}`);
};
export const getApproval = (id: string) => request<Approval>(`/api/v1/approvals/${id}`);
export const decideApproval = (id: string, body: DecideApprovalReq) =>
  request<Approval>(`/api/v1/approvals/${id}/decision`, { method: 'POST', body });

// ---- 决策 / 记忆 / 审计 ----
export const listDecisions = (companyId: string, kind?: string) => {
  const s = kind ? `?kind=${encodeURIComponent(kind)}` : '';
  return request<Decision[]>(`/api/v1/companies/${companyId}/decisions${s}`);
};
export const createDecision = (companyId: string, body: CreateDecisionReq) =>
  request<Decision>(`/api/v1/companies/${companyId}/decisions`, { method: 'POST', body });
export const listMemories = (companyId: string, type?: string) => {
  const s = type ? `?type=${encodeURIComponent(type)}` : '';
  return request<Memory[]>(`/api/v1/companies/${companyId}/memories${s}`);
};
export const searchMemories = (companyId: string, q: string) =>
  request<Memory[]>(`/api/v1/companies/${companyId}/memories/search?q=${encodeURIComponent(q)}`);
export const createMemory = (companyId: string, body: CreateMemoryReq) =>
  request<Memory>(`/api/v1/companies/${companyId}/memories`, { method: 'POST', body });
export const listAudits = (entity?: string) => {
  const s = entity ? `?entity=${encodeURIComponent(entity)}` : '';
  return request<Audit[]>(`/api/v1/audit${s}`);
};

// ---- 模型端点池 / 通道 B ----
export const listEndpoints = (companyId: string) =>
  request<Endpoint[]>(`/api/v1/companies/${companyId}/endpoints`);
export const getEndpoint = (id: string) => request<Endpoint>(`/api/v1/endpoints/${id}`);
export const addEndpoint = (body: AddEndpointReq) =>
  request<Endpoint>('/api/v1/endpoints', { method: 'POST', body });
export const selectEndpointModel = (id: string, model: string, role?: string, tier?: string) =>
  request<Endpoint>(`/api/v1/endpoints/${id}/select`, {
    method: 'POST',
    body: { model, role: role || undefined, tier: tier || undefined },
  });
export const fetchEndpointModels = (id: string) =>
  request<ModelInfo[]>(`/api/v1/endpoints/${id}/models`, { method: 'POST' });

// D7:手工 repos 登记口已去(代码源 = 项目 git origin 自动认领);同步入口保留(遍历 = 各项目代码源 + legacy)。
export const intakeSync = (companyId: string) =>
  request<IntakeResult[]>(`/api/v1/companies/${companyId}/intake/sync`, { method: 'POST' });

// ---- 项目 · 流水线(Phase 10.1,契约 project-pipeline-foundation.md §3.4)----
export const listProjects = (companyId: string) =>
  request<Project[]>(`/api/v1/companies/${companyId}/projects`);
export const createProject = (companyId: string, body: CreateProjectReq) =>
  request<Project>(`/api/v1/companies/${companyId}/projects`, { method: 'POST', body });
export const getProject = (id: string) => request<Project>(`/api/v1/projects/${id}`);
export const deleteProject = (id: string) =>
  request<{ id: string; deleted: boolean }>(`/api/v1/projects/${id}`, { method: 'DELETE' });
// D7:建后补/换 GitHub origin remote → 重认领/刷新项目代码源;无 remote → 400。
export const refreshProjectCodeSource = (id: string) =>
  request<CodeSource>(`/api/v1/projects/${id}/code-source/refresh`, { method: 'POST' });

export const listPipelines = (projectId: string) =>
  request<Pipeline[]>(`/api/v1/projects/${projectId}/pipelines`);
export const createPipeline = (projectId: string, body: CreatePipelineReq) =>
  request<Pipeline>(`/api/v1/projects/${projectId}/pipelines`, { method: 'POST', body });
export const listProjectTasks = (projectId: string, limit = 20) =>
  request<Task[]>(`/api/v1/projects/${projectId}/tasks?limit=${limit}`);
export const getPipeline = (id: string) => request<Pipeline>(`/api/v1/pipelines/${id}`);
export const updatePipelineSchedule = (id: string, body: UpdatePipelineScheduleReq) =>
  request<Pipeline>(`/api/v1/pipelines/${id}`, { method: 'PUT', body });
export const deletePipeline = (id: string) =>
  request<{ id: string; deleted: boolean }>(`/api/v1/pipelines/${id}`, { method: 'DELETE' });
export const runPipeline = (id: string, body?: RunPipelineReq) =>
  request<RunPipelineResult>(`/api/v1/pipelines/${id}/run`, { method: 'POST', body });
// 10.2:巡检报告正文读端点(仅已完成 patrol run;正文纯文本,限长截断带 truncated)。
export const getPatrolReport = (projectId: string, taskId: string) =>
  request<PatrolReport>(`/api/v1/projects/${projectId}/patrol/${taskId}`);

// ---- setup / console token(Phase 9.2,契约 console-access.md §3.6)----
export const setupStatus = () => request<SetupStatus>('/api/v1/setup/status');
export const runSetup = (body: SetupReq) => request<SetupResult>('/api/v1/setup', { method: 'POST', body });
export const rotateConsoleToken = (body: RotateConsoleTokenReq) =>
  request<RotateConsoleTokenResult>('/api/v1/settings/console-token', { method: 'PUT', body });

// ---- settings(Phase 9.3,契约 runtime-knobs-web.md §3.6/§3.7)----
export const getGlobalSettings = () => request<GlobalSettings>('/api/v1/settings');
export const updateGlobalSettings = (body: UpdateGlobalSettingsReq) =>
  request<GlobalSettings>('/api/v1/settings', { method: 'PUT', body });
export const rotateMasterKey = () =>
  request<RotateMasterKeyResult>('/api/v1/settings/rotate-master-key', { method: 'POST' });

export const getCompanySettings = (companyId: string) =>
  request<CompanySettings>(`/api/v1/companies/${companyId}/settings`);
export const updateCompanySettings = (companyId: string, body: UpdateCompanySettingsReq) =>
  request<CompanySettings>(`/api/v1/companies/${companyId}/settings`, { method: 'PUT', body });
export const resetCompanySettings = (companyId: string) =>
  request<{ reset: boolean }>(`/api/v1/companies/${companyId}/settings`, { method: 'DELETE' });

export const listSecrets = (companyId: string) =>
  request<SecretMeta[]>(`/api/v1/companies/${companyId}/secrets`);
export const setSecret = (companyId: string, secretId: string, value: string) =>
  request<{ id: string; set: boolean }>(`/api/v1/companies/${companyId}/secrets/${secretId}`, {
    method: 'PUT',
    body: { value },
  });
export const deleteSecret = (companyId: string, secretId: string) =>
  request<{ id: string; deleted: boolean }>(`/api/v1/companies/${companyId}/secrets/${secretId}`, {
    method: 'DELETE',
  });

