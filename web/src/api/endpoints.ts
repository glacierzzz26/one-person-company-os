// 每 API 端点薄 helper(全部走信封 client;路径与 internal/server/api.go 路由逐字对应)。
import { request } from './client';
import type {
  AddEndpointReq,
  AddRepoReq,
  Agent,
  Approval,
  Audit,
  Capability,
  Company,
  CreateCompanyReq,
  CreateDecisionReq,
  CreateMemoryReq,
  CreateTaskReq,
  DecideApprovalReq,
  Decision,
  Endpoint,
  Execution,
  IntakeResult,
  Memory,
  ModelInfo,
  Overview,
  Repo,
  Task,
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

export const listRepos = (companyId: string) =>
  request<Repo[]>(`/api/v1/companies/${companyId}/repos`);
export const addRepo = (companyId: string, body: AddRepoReq) =>
  request<Repo>(`/api/v1/companies/${companyId}/repos`, { method: 'POST', body });
export const intakeSync = (companyId: string) =>
  request<IntakeResult[]>(`/api/v1/companies/${companyId}/intake/sync`, { method: 'POST' });

