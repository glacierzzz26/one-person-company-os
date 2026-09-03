// 审计流水:GET /audit?entity=(全局)。entity chips 从已载行去重(后端实体集合无固定枚举,
// 不臆造);表只读。actor human:console 高亮(控制台写操作都会留痕)。详情含审批备注/建单理由。
import { useMemo, useState } from 'react';
import { Button, Card, Space, Table } from 'antd';
import type { Audit as AuditRow } from '../api/types';
import { listAudits } from '../api/endpoints';
import { useApp } from '../store/AppContext';
import { useData } from '../hooks/useApi';
import { PageHead, EmptyState } from '../components/common';
import { fmtFull, ago, short } from '../utils/time';

export default function Audit() {
  const { refreshKey } = useApp();
  const [entity, setEntity] = useState('all');
  const [actorQ, setActorQ] = useState('');

  const data = useData<AuditRow[]>(() => listAudits(), {
    deps: [refreshKey, actorQ],
    intervalMs: 20000,
  });
  const rows = data.data ?? [];

  const entities = useMemo(() => {
    const m = new Map<string, number>();
    rows.forEach((a) => m.set(a.entity_type, (m.get(a.entity_type) ?? 0) + 1));
    return [...m.entries()].sort((a, b) => b[1] - a[1]);
  }, [rows]);

  const filtered = useMemo(
    () =>
      rows.filter((a) => {
        if (entity !== 'all' && a.entity_type !== entity) return false;
        const q = actorQ.trim().toLowerCase();
        if (q && !(a.actor.toLowerCase().includes(q) || a.action.toLowerCase().includes(q) || a.detail.toLowerCase().includes(q))) return false;
        return true;
      }),
    [rows, entity, actorQ],
  );

  return (
    <div>
      <PageHead title="审计流水" sub="GET /api/v1/audit · 全局 · 只读" />

      <Space wrap style={{ marginBottom: 12 }}>
        <Button size="small" type={entity === 'all' ? 'primary' : 'default'} onClick={() => setEntity('all')}>
          全部 {rows.length}
        </Button>
        {entities.map(([e, n]) => (
          <Button key={e} size="small" type={entity === e ? 'primary' : 'default'} onClick={() => setEntity(e)}>
            {e} {n}
          </Button>
        ))}
        <Button
          size="small"
          onClick={() => setActorQ((v) => (v === 'human:console' ? '' : 'human:console'))}
          type={actorQ === 'human:console' ? 'primary' : 'default'}
        >
          {actorQ === 'human:console' ? '✕ 仅 console' : 'console 写入'}
        </Button>
      </Space>

      <Card size="small" styles={{ body: { padding: 0 } }}>
        <Table<AuditRow>
          size="small"
          rowKey="id"
          dataSource={filtered}
          pagination={{ pageSize: 20, showSizeChanger: false }}
          locale={{ emptyText: <EmptyState text="暂无审计流水" /> }}
          columns={[
            {
              title: '时间',
              width: 160,
              render: (_, a) => (
                <div style={{ fontSize: 12 }}>
                  <div>{fmtFull(a.created_at)}</div>
                  <div className="dim">{ago(a.created_at)}</div>
                </div>
              ),
            },
            {
              title: '实体',
              width: 170,
              render: (_, a) => (
                <div>
                  <span className="pill pill-accent mono" style={{ fontSize: 11 }}>{a.entity_type}</span>
                  <div className="mono dim" style={{ fontSize: 11, marginTop: 2 }}>{short(a.entity_id)}</div>
                </div>
              ),
            },
            { title: '动作', width: 110, render: (_, a) => <span className="mono" style={{ fontSize: 12 }}>{a.action}</span> },
            {
              title: 'Actor',
              width: 140,
              render: (_, a) => (
                <span
                  className="mono"
                  style={{
                    fontSize: 11.5,
                    color: a.actor.startsWith('human:') ? 'var(--accent)' : undefined,
                  }}
                >
                  {a.actor}
                </span>
              ),
            },
            {
              title: '详情',
              render: (_, a) => (
                <span className="muted" style={{ fontSize: 12.5, whiteSpace: 'pre-wrap' }}>
                  {a.detail || '—'}
                </span>
              ),
            },
          ]}
        />
      </Card>
    </div>
  );
}
