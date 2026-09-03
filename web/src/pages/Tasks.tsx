// 任务中心:GET /tasks?company=&status=&risk=(attempt>0 才传)。服务端筛 status/risk;
// q(标题/描述)与 tool 客户端过滤;行点开 → TaskDrawer(全字段 KV + 执行回合)。POST /tasks 建单。
import { useMemo, useState } from 'react';
import { Button, Card, Input, Segmented, Select, Space, Table } from 'antd';
import { PlusOutlined } from '@ant-design/icons';
import type { Approval, Task } from '../api/types';
import { listTasks } from '../api/endpoints';
import { useApp } from '../store/AppContext';
import { useData } from '../hooks/useApi';
import { PageHead, RiskText, EmptyState } from '../components/common';
import StatusTag from '../components/StatusTag';
import TaskDrawer from '../components/TaskDrawer';
import DecideModal from '../components/DecideModal';
import { TaskCreateModal } from '../components/modals';
import { TASK_STATUS_KEYS, taskStatusLabel, taskStatusPreset } from '../utils/dicts';
import { ago, short } from '../utils/time';

const STATUS_FILTERS = [
  { label: '全部', value: 'all' },
  ...TASK_STATUS_KEYS.map((s) => ({ label: taskStatusLabel(s), value: s })),
];
const RISK_FILTERS = [
  { label: '全风险', value: 'all' },
  { label: '低', value: 'low' },
  { label: '中', value: 'medium' },
  { label: '高', value: 'high' },
];

export default function Tasks() {
  const { companyId, refreshKey, bump } = useApp();
  const [status, setStatus] = useState<string>('all');
  const [risk, setRisk] = useState<string>('all');
  const [q, setQ] = useState('');
  const [tool, setTool] = useState<string>('all');
  const [createOpen, setCreateOpen] = useState(false);
  const [drawerTask, setDrawerTask] = useState<string | null>(null);
  const [decideApproval, setDecideApproval] = useState<Approval | null>(null);

  const data = useData<Task[]>(
    () => listTasks({ company: companyId ?? undefined, status: status === 'all' ? undefined : status, risk: risk === 'all' ? undefined : risk }),
    { deps: [companyId, status, risk, refreshKey], intervalMs: 15000, enabled: !!companyId },
  );
  const rows = data.data ?? [];

  const toolOptions = useMemo(() => {
    const set = new Set<string>();
    rows.forEach((t) => t.tool_name && set.add(t.tool_name));
    return [{ value: 'all', label: '全部工具' }, ...[...set].sort().map((v) => ({ value: v, label: v }))];
  }, [rows]);

  const filtered = useMemo(() => {
    const qq = q.trim().toLowerCase();
    return rows.filter((t) => {
      if (tool !== 'all' && t.tool_name !== tool) return false;
      if (!qq) return true;
      return t.title.toLowerCase().includes(qq) || t.description.toLowerCase().includes(qq) || t.id.toLowerCase().includes(qq);
    });
  }, [rows, q, tool]);

  return (
    <div>
      <PageHead
        title="任务中心"
        sub="GET /api/v1/tasks?company=&status=&risk="
        actions={
          <Button type="primary" icon={<PlusOutlined />} onClick={() => setCreateOpen(true)}>
            新建工程请求
          </Button>
        }
      />

      <Space wrap style={{ marginBottom: 12 }}>
        <Segmented value={status} onChange={(v) => setStatus(v as string)} options={STATUS_FILTERS} />
        <Segmented value={risk} onChange={(v) => setRisk(v as string)} options={RISK_FILTERS} />
        <Select size="middle" style={{ width: 150 }} value={tool} onChange={setTool} options={toolOptions} />
        <Input.Search
          allowClear
          placeholder="搜标题/描述/ID"
          style={{ width: 240 }}
          onSearch={setQ}
          onChange={(e) => !e.target.value && setQ('')}
        />
      </Space>

      <Card size="small" styles={{ body: { padding: 0 } }}>
        <Table<Task>
          size="small"
          rowKey="id"
          dataSource={filtered}
          pagination={{ pageSize: 15, showSizeChanger: false }}
          locale={{ emptyText: <EmptyState text="无任务(执行队列由 os queue work 消费)" /> }}
          onRow={(t) => ({ onClick: () => setDrawerTask(t.id), className: 'row-click' })}
          columns={[
            {
              title: '标题',
              render: (_, t) => (
                <div style={{ minWidth: 0 }}>
                  <b>
                    {t.parent_task_id ? <span className="dim">⤷ </span> : null}
                    {t.title}
                  </b>
                  {t.round_no > 1 ? <span className="pill pill-warn" style={{ marginLeft: 6 }}>回合 {t.round_no}</span> : null}
                  {t.tool_name ? <span className="pill pill-muted" style={{ marginLeft: 6 }}>{t.tool_name}</span> : null}
                  <div className="mono dim" style={{ fontSize: 11 }}>
                    {short(t.id)} · {t.parent_task_id ? `parent ${short(t.parent_task_id)}` : '顶层'}
                    {t.workspace_path ? ` · ${t.workspace_path}` : ''}
                  </div>
                </div>
              ),
            },
            { title: '状态', width: 104, render: (_, t) => <StatusTag preset={taskStatusPreset(t.status)} label={taskStatusLabel(t.status)} /> },
            {
              title: 'q_status',
              width: 130,
              render: (_, t) => <span className="mono dim" style={{ fontSize: 11.5 }}>{t.q_status}</span>,
            },
            { title: '风险', width: 70, render: (_, t) => <RiskText risk={t.risk} /> },
            {
              title: '冲突',
              width: 64,
              render: (_, t) => (
                <span className="mono" style={{ fontSize: 12, color: t.conflict_count >= 3 ? 'var(--crit)' : undefined }}>
                  {t.conflict_count}
                </span>
              ),
            },
            {
              title: '认领',
              width: 90,
              render: (_, t) => <span className="mono dim" style={{ fontSize: 11.5 }}>{short(t.lease_worker_id) || '—'}</span>,
            },
            { title: '尝试', width: 74, render: (_, t) => <span className="mono" style={{ fontSize: 12 }}>{t.attempt}/{t.max_attempts}</span> },
            { title: '更新', width: 86, render: (_, t) => <span className="dim" style={{ fontSize: 12 }}>{ago(t.updated_at)}</span> },
          ]}
        />
      </Card>

      <div className="dim" style={{ fontSize: 11.5, marginTop: 10 }}>
        执行态只读展示(running/completed/failed/timeout);真正驱动在终端 os task run / os queue work,server --queue-work 可常驻自动认领。
      </div>

      <TaskCreateModal open={createOpen} onClose={() => setCreateOpen(false)} onDone={bump} />
      <TaskDrawer taskId={drawerTask} onClose={() => setDrawerTask(null)} onOpenDecide={(a) => setDecideApproval(a)} />
      <DecideModal
        approval={decideApproval}
        onClose={() => setDecideApproval(null)}
        onDone={() => {
          setDecideApproval(null);
          setDrawerTask(null);
          bump();
        }}
      />
    </div>
  );
}
