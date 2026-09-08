// 任务详情抽屉:全字段 KV + 执行回合时间线(GET /tasks/{id}/executions)。
// status=waiting_approval 且存在 pending 审批 → 底部「处理该任务的审批」交给页面 DecideModal。
import { useMemo } from 'react';
import { Button, Drawer, Empty, Spin, Tag } from 'antd';
import { getTask, listApprovals, listExecutions } from '../api/endpoints';
import type { Approval, Execution } from '../api/types';
import { useData } from '../hooks/useApi';
import { useApp } from '../store/AppContext';
import { KV, RiskText } from './common';
import StatusTag from './StatusTag';
import PlanBlock from './PlanBlock';
import { execStatusLabel, execStatusPreset, taskStatusLabel, taskStatusPreset } from '../utils/dicts';
import { fmtT, short } from '../utils/time';

function ExecutionBlock({ taskId }: { taskId: string }) {
  const execs = useData<Execution[]>(() => listExecutions(taskId), {
    deps: [taskId],
    intervalMs: 0,
  });
  if (execs.loading) {
    return (
      <div style={{ textAlign: 'center', padding: 20 }}>
        <Spin size="small" />
      </div>
    );
  }
  const list = execs.data ?? [];
  if (!list.length) {
    return (
      <div className="dim" style={{ fontSize: 12.5 }}>
        暂无执行记录 —— 队列由 os queue work 或 server --queue-work 认领
      </div>
    );
  }
  return (
    <div>
      {[...list]
        .sort((a, b) => a.attempt - b.attempt)
        .map((x: Execution) => (
          <div key={x.id} style={{ display: 'flex', gap: 12, padding: '8px 0' }}>
            <div style={{ color: x.status === 'failed' || x.status === 'timeout' ? 'var(--crit)' : 'var(--ok)', fontSize: 16, lineHeight: 1.4 }}>
              {x.status === 'failed' || x.status === 'timeout' ? '✗' : '✓'}
            </div>
            <div style={{ flex: 1 }}>
              <div>
                <b>第 {x.attempt} 次</b> <StatusTag preset={execStatusPreset(x.status)} label={execStatusLabel(x.status)} />{' '}
                <Tag className="mono">{x.worker_id}</Tag>
              </div>
              <div className="mono dim" style={{ fontSize: 11.5 }}>
                {fmtT(x.started_at)} → {fmtT(x.finished_at)}
              </div>
              {x.result ? <div className="muted" style={{ fontSize: 12.5, marginTop: 2 }}>{x.result}</div> : null}
              {x.error ? (
                <div className="mono" style={{ fontSize: 12, color: 'var(--crit)', background: 'var(--crit-soft)', borderRadius: 5, padding: '4px 8px', marginTop: 4 }}>
                  {x.error}
                </div>
              ) : null}
            </div>
          </div>
        ))}
    </div>
  );
}

export default function TaskDrawer({
  taskId,
  onClose,
  onOpenDecide,
}: {
  taskId: string | null;
  onClose: () => void;
  onOpenDecide: (a: Approval) => void;
}) {
  const { refreshKey } = useApp();
  const task = useData(() => (taskId ? getTask(taskId) : Promise.reject(new Error('no id'))), {
    deps: [taskId, refreshKey],
    enabled: !!taskId,
  });
  // 该任务若有待决审批(drawer 底部直达决策)
  const approvals = useData<Approval[]>(
    () => listApprovals('pending'),
    { deps: [refreshKey], enabled: !!taskId },
  );

  const pendingApproval = useMemo(() => {
    if (!taskId) return null;
    return (approvals.data ?? []).find((a) => a.task_id === taskId) ?? null;
  }, [approvals.data, taskId]);

  const t = task.data;
  const loading = task.loading;

  const kv: [string, React.ReactNode][] = !t
    ? []
    : [
        ['ID', <span className="mono" style={{ fontSize: 12 }} key="id">{t.id}</span>],
        ['工具', <Tag key="tool">{t.tool_name}</Tag>],
        ['状态', <StatusTag key="st" preset={taskStatusPreset(t.status)} label={taskStatusLabel(t.status)} />],
        ['队列 q_status', <span className="mono" style={{ fontSize: 12 }} key="q">{t.q_status}</span>],
        ['风险', <RiskText key="risk" risk={t.risk} />],
        [
          '回合 / 冲突',
          <span className="mono" key="rnd" style={{ fontSize: 12 }}>
            {t.round_no} / <b style={{ color: t.conflict_count >= 3 ? 'var(--crit)' : undefined }}>{t.conflict_count}</b>
          </span>,
        ],
        ['capability', <span className="mono" style={{ fontSize: 12 }} key="cap">{short(t.capability_id ?? '') || '—'}</span>],
        ['父任务', t.parent_task_id ? <span className="mono" style={{ fontSize: 12 }} key="p">{short(t.parent_task_id)}</span> : '—'],
        ['创建 / 更新', <span className="mono" style={{ fontSize: 12 }} key="cu">{fmtT(t.created_at)} / {fmtT(t.updated_at)}</span>],
        ['workspace', <span className="mono" style={{ fontSize: 12 }} key="ws">{t.workspace_path || '—'}</span>],
        ['attempt / max', <span className="mono" style={{ fontSize: 12 }} key="am">{t.attempt} / {t.max_attempts}</span>],
        ...(t.last_error ? ([['最后错误', <span className="mono tx-crit" style={{ fontSize: 12 }} key="le">{t.last_error}</span>]] as [string, React.ReactNode][]) : []),
        ...(t.result ? ([['结果', <span key="res">{t.result}</span>]] as [string, React.ReactNode][]) : []),
      ];

  return (
    <Drawer
      title={t?.title ?? '任务详情'}
      width={min(560, window.innerWidth * 0.92)}
      open={!!taskId}
      onClose={onClose}
      footer={
        pendingApproval ? (
          <Button type="primary" block onClick={() => onOpenDecide(pendingApproval)}>
            处理该任务的审批({short(pendingApproval.id)}) →
          </Button>
        ) : null
      }
    >
      {loading && !t ? (
        <div style={{ textAlign: 'center', padding: 40 }}>
          <Spin />
        </div>
      ) : !t ? (
        <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={task.error ?? '任务不存在'} />
      ) : (
        <>
          {t.description ? (
            <div className="muted" style={{ fontSize: 13, marginBottom: 14, whiteSpace: 'pre-wrap' }}>
              {t.description}
            </div>
          ) : null}
          <KV rows={kv} />
          <div className="sect-h">计划(Phase 10.3 · GET /tasks/&#123;id&#125;/plan)</div>
          <PlanBlock taskId={t.id} />
          <div className="sect-h">执行回合</div>
          <ExecutionBlock taskId={t.id} />
          <div className="dim mono" style={{ fontSize: 11.5, marginTop: 16 }}>
            API 不暴露执行;长时驱动走 os task run / os queue work / server --queue-work
          </div>
        </>
      )}
    </Drawer>
  );
}

const min = (a: number, b: number) => (a < b ? a : b);
