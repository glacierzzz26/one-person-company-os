// 决策档案:GET /companies/{id}/decisions?kind=(真枚举);kind chips 过滤;POST 记一笔。
import { useState } from 'react';
import { Button, Card, Space, Table } from 'antd';
import { PlusOutlined } from '@ant-design/icons';
import type { Decision } from '../api/types';
import { DECISION_KINDS } from '../api/types';
import { listDecisions } from '../api/endpoints';
import { useApp } from '../store/AppContext';
import { useData } from '../hooks/useApi';
import { PageHead, EmptyState } from '../components/common';
import StatusTag from '../components/StatusTag';
import { DecisionCreateModal } from '../components/modals';
import { decisionKindLabel, decisionStatusLabel, decisionStatusPreset } from '../utils/dicts';
import { fmtT, ago, short } from '../utils/time';

const KIND_FILTERS: { label: string; value: string }[] = [
  { label: '全部', value: 'all' },
  ...DECISION_KINDS.map((k) => ({ label: `${decisionKindLabel(k)} · ${k}`, value: k })),
];

export default function Decisions() {
  const { companyId, refreshKey, bump } = useApp();
  const [kind, setKind] = useState('all');
  const [createOpen, setCreateOpen] = useState(false);

  const data = useData<Decision[]>(() => (companyId ? listDecisions(companyId, kind === 'all' ? undefined : kind) : Promise.reject()), {
    deps: [companyId, kind, refreshKey],
    enabled: !!companyId,
  });
  const rows = data.data ?? [];

  return (
    <div>
      <PageHead
        title="决策档案"
        sub="GET /api/v1/companies/{id}/decisions"
        actions={
          <Button type="primary" icon={<PlusOutlined />} onClick={() => setCreateOpen(true)}>
            记一笔决策
          </Button>
        }
      />

      <Space wrap style={{ marginBottom: 12 }}>
        {KIND_FILTERS.map((f) => (
          <Button
            key={f.value}
            size="small"
            type={kind === f.value ? 'primary' : 'default'}
            onClick={() => setKind(f.value)}
          >
            {f.label}
          </Button>
        ))}
      </Space>

      <Card size="small" styles={{ body: { padding: 0 } }}>
        <Table<Decision>
          size="middle"
          rowKey="id"
          dataSource={rows}
          pagination={{ pageSize: 15, showSizeChanger: false }}
          locale={{ emptyText: <EmptyState text="暂无决策(终端 os decision add 亦写同一表)" /> }}
          columns={[
            {
              title: '标题 / 正文',
              render: (_, d) => (
                <div>
                  <b>{d.title}</b>
                  {d.body ? (
                    <div className="muted" style={{ fontSize: 12, marginTop: 2, whiteSpace: 'pre-wrap', maxWidth: 520 }}>
                      {d.body}
                    </div>
                  ) : null}
                </div>
              ),
            },
            {
              title: '类别',
              width: 90,
              render: (_, d) => (
                <span className="pill pill-muted">
                  {decisionKindLabel(d.kind)} · {d.kind}
                </span>
              ),
            },
            { title: '状态', width: 90, render: (_, d) => <StatusTag preset={decisionStatusPreset(d.status)} label={decisionStatusLabel(d.status)} /> },
            {
              title: '决策者',
              width: 130,
              render: (_, d) => (
                <span className="mono" style={{ fontSize: 12 }}>
                  {d.decided_by}
                </span>
              ),
            },
            { title: '来源', width: 140, render: (_, d) => <span className="mono dim" style={{ fontSize: 11.5 }}>{short(d.source)}</span> },
            {
              title: '时间',
              width: 150,
              render: (_, d) => (
                <div style={{ fontSize: 12 }}>
                  <div>{fmtT(d.created_at)}</div>
                  <div className="dim">{d.updated_at !== d.created_at ? `更新 ${ago(d.updated_at)}` : ''}</div>
                </div>
              ),
            },
          ]}
        />
      </Card>

      <DecisionCreateModal open={createOpen} onClose={() => setCreateOpen(false)} onDone={bump} />
    </div>
  );
}
