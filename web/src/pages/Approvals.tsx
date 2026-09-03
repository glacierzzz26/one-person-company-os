// 审批中心:GET /approvals?status=(全局,无 company 参数)。pending 行 → DecideModal 三态;
// 标题懒取 GET /tasks/{id}(审批行本身不带 task_title);已决只读。决策后 bump → 全页刷新。
import { useState } from 'react';
import { Button, Card, Segmented, Table } from 'antd';
import type { Approval } from '../api/types';
import { listApprovals, getTask } from '../api/endpoints';
import { useApp } from '../store/AppContext';
import { useData } from '../hooks/useApi';
import { PageHead, RiskText, EmptyState } from '../components/common';
import StatusTag from '../components/StatusTag';
import DecideModal from '../components/DecideModal';
import TaskDrawer from '../components/TaskDrawer';
import { approvalStatusPreset, approvalStatusLabel } from '../utils/dicts';
import { fmtT, ago, short } from '../utils/time';

const FILTERS = [
  { label: '待决策', value: 'pending' },
  { label: '已放行', value: 'approved' },
  { label: '已驳回', value: 'rejected' },
  { label: '要修改', value: 'changes' },
  { label: '全部', value: 'all' },
];

/** 审批行不带标题 → 懒取任务标题(按 task_id 组件实例做渲染级缓存) */
function ApprovalTitle({ taskId }: { taskId: string }) {
  const t = useData(() => getTask(taskId), { deps: [taskId], enabled: !!taskId });
  if (t.loading && !t.data) return <span className="dim" style={{ fontSize: 12 }}>载入标题…</span>;
  if (!t.data) return <span className="mono dim" style={{ fontSize: 12 }}>{short(taskId)}</span>;
  return <b>{t.data.title}</b>;
}

export default function Approvals() {
  const { refreshKey, bump } = useApp();
  const [status, setStatus] = useState('pending');
  const [deciding, setDeciding] = useState<Approval | null>(null);
  const [drawerTask, setDrawerTask] = useState<string | null>(null);

  const data = useData<Approval[]>(() => listApprovals(status), {
    deps: [status, refreshKey],
    intervalMs: 15000,
  });
  const rows = data.data ?? [];

  const openDecide = (a: Approval) => setDeciding(a);

  return (
    <div>
      <PageHead
        title="审批中心"
        sub="GET /api/v1/approvals · 全局(不含 company 参数)"
        actions={
          <Segmented
            value={status}
            onChange={(v) => setStatus(v as string)}
            options={FILTERS}
          />
        }
      />

      {data.loading && !rows.length ? (
        <EmptyState text="载入中…" />
      ) : !rows.length ? (
        <Card size="small">
          <EmptyState text="该状态下暂无审批" />
        </Card>
      ) : (
        <Card size="small" styles={{ body: { padding: 0 } }}>
          <Table<Approval>
            size="middle"
            rowKey="id"
            dataSource={rows}
            pagination={{ pageSize: 20, showSizeChanger: false }}
            locale={{ emptyText: <EmptyState text="该状态下暂无审批" /> }}
            columns={[
              {
                title: '任务',
                render: (_, a) => (
                  <div style={{ minWidth: 0 }}>
                    <a onClick={() => setDrawerTask(a.task_id)} style={{ textDecoration: 'none' }}>
                      <ApprovalTitle taskId={a.task_id} />
                    </a>
                    <div className="mono dim" style={{ fontSize: 11 }}>task {short(a.task_id)}</div>
                  </div>
                ),
              },
              {
                title: '状态',
                width: 110,
                render: (_, a) => <StatusTag preset={approvalStatusPreset(a.status)} label={approvalStatusLabel(a.status)} />,
              },
              { title: '风险', width: 70, render: (_, a) => <RiskText risk={a.risk} /> },
              {
                title: '原因',
                render: (_, a) => <span className="dim" style={{ fontSize: 12.5 }}>{a.reason}</span>,
              },
              {
                title: '请求 / 决策',
                width: 170,
                render: (_, a) => (
                  <div style={{ fontSize: 12 }}>
                    <div className="mono">{a.requested_by}</div>
                    <div className="dim">{a.decided_by ? `→ ${a.decided_by}` : '—'}</div>
                  </div>
                ),
              },
              {
                title: '时间',
                width: 120,
                render: (_, a) => (
                  <div style={{ fontSize: 12 }}>
                    <div className="dim">{fmtT(a.created_at)}</div>
                    <div className="dim">{a.decided_at ? `${ago(a.decided_at)}决` : ''}</div>
                  </div>
                ),
              },
              {
                title: '操作',
                width: 90,
                render: (_, a) =>
                  a.status === 'pending' ? (
                    <Button size="small" type="primary" onClick={() => openDecide(a)}>
                      决策
                    </Button>
                  ) : (
                    <span className="dim" style={{ fontSize: 12 }}>只读</span>
                  ),
              },
            ]}
          />
        </Card>
      )}

      <DecideModal
        approval={deciding}
        onClose={() => setDeciding(null)}
        onDone={() => {
          setDeciding(null);
          bump();
        }}
      />
      <TaskDrawer taskId={drawerTask} onClose={() => setDrawerTask(null)} onOpenDecide={(a) => setDeciding(a)} />
    </div>
  );
}
