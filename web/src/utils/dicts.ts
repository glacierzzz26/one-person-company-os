// 枚举 → 中文 + antd Tag 预设色。色值只在 Tag/图表出现处消费;图表取 antd token(随主题)。
// 枚举值与后端逐字一致(task.status / execution.status / approval.status / decision / memory…)。
import type { ApprovalStatus, DecisionStatus, ExecutionStatus, Risk, TaskStatus } from '../api/types';

// antd Tag preset color 别名(受 ConfigProvider 语义 token 驱动,随明暗切换)
export type Preset = 'default' | 'processing' | 'success' | 'warning' | 'error';

export const TASK_STATUS_KEYS: TaskStatus[] = ['pending', 'running', 'waiting_approval', 'completed', 'failed'];

export const TASK_STATUS_META: Record<TaskStatus, { t: string; c: Preset }> = {
  pending: { t: '排队', c: 'default' },
  running: { t: '执行中', c: 'processing' },
  waiting_approval: { t: '待审批', c: 'warning' },
  completed: { t: '完成', c: 'success' },
  failed: { t: '失败', c: 'error' },
};
export const taskStatusLabel = (s: string): string => TASK_STATUS_META[s as TaskStatus]?.t ?? s;
export const taskStatusPreset = (s: string): Preset => TASK_STATUS_META[s as TaskStatus]?.c ?? 'default';

// 任务状态 → 堆积条/图例点 CSS 色类(色值在 styles.css 随 data-theme 明暗切换)
export const TASK_STATUS_BAR: Record<TaskStatus, string> = {
  pending: 'sb-muted',
  running: 'sb-accent',
  waiting_approval: 'sb-warn',
  completed: 'sb-ok',
  failed: 'sb-crit',
};

export const EXEC_STATUS_META: Record<ExecutionStatus, { t: string; c: Preset }> = {
  running: { t: '执行中', c: 'processing' },
  completed: { t: '完成', c: 'success' },
  failed: { t: '失败', c: 'error' },
  timeout: { t: '超时', c: 'warning' },
};
export const execStatusLabel = (s: string): string => EXEC_STATUS_META[s as ExecutionStatus]?.t ?? s;
export const execStatusPreset = (s: string): Preset => EXEC_STATUS_META[s as ExecutionStatus]?.c ?? 'default';

export const APPROVAL_STATUS_META: Record<ApprovalStatus, { t: string; c: Preset }> = {
  pending: { t: '待决策', c: 'warning' },
  approved: { t: '已放行', c: 'success' },
  rejected: { t: '已驳回', c: 'error' },
  changes: { t: '要求修改', c: 'processing' },
};
export const approvalStatusLabel = (s: string): string => APPROVAL_STATUS_META[s as ApprovalStatus]?.t ?? s;
export const approvalStatusPreset = (s: string): Preset => APPROVAL_STATUS_META[s as ApprovalStatus]?.c ?? 'default';

export const RISK_META: Record<Risk, { t: string }> = { low: { t: '低' }, medium: { t: '中' }, high: { t: '高' } };
export const riskLabel = (r: string): string => RISK_META[r as Risk]?.t ?? r;
export const riskColor = (r: string): string => (r === 'high' ? 'var(--crit)' : r === 'medium' ? 'var(--warn)' : 'var(--text-3)');

export const DECISION_KIND_CN: Record<string, string> = {
  approval: '审批',
  goal: '目标',
  strategy: '策略',
  policy_change: '政策变更',
  capital: '资金',
  manual: '手动',
};
export const decisionKindLabel = (k: string): string => DECISION_KIND_CN[k] ?? k;

export const DECISION_STATUS_CN: Record<DecisionStatus, string> = { made: '已决策', pending: '待执行', executed: '已执行' };
export const decisionStatusLabel = (s: string): string => DECISION_STATUS_CN[s as DecisionStatus] ?? s;
export const decisionStatusPreset = (s: string): Preset =>
  s === 'made' ? 'success' : s === 'pending' ? 'warning' : 'default';

export const MEMORY_TYPE_CN: Record<string, string> = {
  lesson: '教训',
  knowledge: '知识',
  project_context: '项目背景',
  decision_ref: '决策引用',
  architecture: '架构',
  convention: '约定',
  task_history: '任务史',
};
export const memoryTypeLabel = (t: string): string => MEMORY_TYPE_CN[t] ?? t;

// issue_sync 处置(账本键):direct_work | merge | skip | ask
export const DISPOSITION_CN: Record<string, string> = {
  direct_work: '直接做 → engineering 任务',
  merge: '合并批次(planner)',
  skip: '跳过',
  ask: '问人 → 审批',
};
export const dispositionLabel = (k: string): string => DISPOSITION_CN[k] ?? k;

export const ENDPOINT_ROLE_CN: Record<string, string> = { pool: '通用池 pool', planner: '规划 planner', standby: '热备 standby' };
export const endpointRoleLabel = (r: string): string => ENDPOINT_ROLE_CN[r] ?? r;
