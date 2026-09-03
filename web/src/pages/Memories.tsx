// 知识库:q 非空 → GET …/memories/search?q=(对 title+content LIKE),否则 GET …/memories?type=。
// type chips 用真枚举(lesson|knowledge|project_context|decision_ref|architecture|convention|task_history),
// tags 空格分隔字符串原样展示。命中处 <mark> 高亮(q 原样,不做分词)。POST …/memories 沉淀。
import { useState } from 'react';
import type { ReactNode } from 'react';
import { Button, Card, Empty, Input, Space } from 'antd';
import { PlusOutlined, SearchOutlined } from '@ant-design/icons';
import type { Memory } from '../api/types';
import { MEMORY_TYPES } from '../api/types';
import { listMemories, searchMemories } from '../api/endpoints';
import { useApp } from '../store/AppContext';
import { useData } from '../hooks/useApi';
import { PageHead, EmptyState } from '../components/common';
import { MemoryCreateModal } from '../components/modals';
import { memoryTypeLabel } from '../utils/dicts';
import { fmtT, ago, short } from '../utils/time';

const esc = (s: string) => s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');

/** q 中命中词 → <mark> 高亮;空/无命中原样返回 */
function hi(text: string, q: string): ReactNode {
  const qq = q.trim();
  if (!qq || !text) return text;
  const re = new RegExp(`(${esc(qq)})`, 'gi');
  const parts = text.split(re);
  return parts.map((p, i) =>
    p.toLowerCase() === qq.toLowerCase() ? (
      <mark key={i} style={{ background: 'var(--warn-soft)', color: 'var(--warn)', padding: '0 2px', borderRadius: 3 }}>
        {p}
      </mark>
    ) : (
      p
    ),
  );
}

const TYPE_FILTERS: { label: string; value: string }[] = [
  { label: '全部', value: 'all' },
  ...MEMORY_TYPES.map((t) => ({ label: `${memoryTypeLabel(t)} · ${t}`, value: t })),
];

export default function Memories() {
  const { companyId, refreshKey, bump } = useApp();
  const [type, setType] = useState<string>('all');
  const [q, setQ] = useState('');
  const [createOpen, setCreateOpen] = useState(false);

  const data = useData<Memory[]>(
    () => {
      if (!companyId) return Promise.reject(new Error('no company'));
      const qq = q.trim();
      return qq ? searchMemories(companyId, qq) : listMemories(companyId, type === 'all' ? undefined : type);
    },
    { deps: [companyId, q, type, refreshKey], enabled: !!companyId },
  );
  const rows = data.data ?? [];

  return (
    <div>
      <PageHead
        title="知识库"
        sub="GET …/memories?type= · 搜 GET …/memories/search?q="
        actions={
          <Button type="primary" icon={<PlusOutlined />} onClick={() => setCreateOpen(true)}>
            沉淀一条记忆
          </Button>
        }
      />

      <Space wrap style={{ marginBottom: 12 }}>
        <Input.Search
          allowClear
          placeholder="搜标题/正文(搜索即弃 type 过滤)"
          prefix={<SearchOutlined />}
          style={{ width: 320 }}
          onSearch={(v) => setQ(v)}
        />
        {TYPE_FILTERS.map((f) => (
          <Button key={f.value} size="small" type={!q.trim() && type === f.value ? 'primary' : 'default'} onClick={() => setType(f.value)}>
            {f.label}
          </Button>
        ))}
      </Space>

      {q.trim() ? (
        <div className="dim" style={{ fontSize: 12, marginBottom: 8 }}>
          搜索结果({rows.length}):命中按 LIKE %q% 返回;type 过滤在此模式不生效。
        </div>
      ) : null}

      {data.loading && !rows.length ? (
        <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="载入中…" />
      ) : !rows.length ? (
        <Card size="small">
          <EmptyState text="暂无记忆 —— agents 的 lesson/复盘会沉淀于此;也可手动记一条" />
        </Card>
      ) : (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
          {rows.map((m) => (
            <Card key={m.id} size="small" styles={{ body: { padding: '12px 16px' } }}>
              <Space wrap style={{ fontSize: 12 }} className="dim">
                <span className="pill pill-accent">{memoryTypeLabel(m.type)}</span>
                <span className="mono" style={{ fontSize: 11 }}>{short(m.source) || 'manual'}</span>
                <span>{ago(m.created_at)}</span>
              </Space>
              <div style={{ marginTop: 6 }}>
                <b>{hi(m.title, q)}</b>
              </div>
              {m.content ? (
                <div className="muted" style={{ fontSize: 13, marginTop: 4, whiteSpace: 'pre-wrap' }}>
                  {hi(m.content, q)}
                </div>
              ) : null}
              {m.tags.trim() ? (
                <Space size={[4, 4]} wrap style={{ marginTop: 8 }}>
                  {m.tags.split(/\s+/).filter(Boolean).map((t) => (
                    <span key={t} className="pill pill-muted mono" style={{ fontSize: 11 }}>
                      #{t}
                    </span>
                  ))}
                </Space>
              ) : null}
              <div className="mono dim" style={{ fontSize: 10.5, marginTop: 8 }}>
                {m.id} · created {fmtT(m.created_at)} · updated {fmtT(m.updated_at)}
              </div>
            </Card>
          ))}
        </div>
      )}

      <MemoryCreateModal open={createOpen} onClose={() => setCreateOpen(false)} onDone={bump} />
    </div>
  );
}
