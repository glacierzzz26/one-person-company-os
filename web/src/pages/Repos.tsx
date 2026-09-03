// 研发仓库(通道 B):GET /companies/{id}/repos(只含登记元数据,无 issue 计数——账本在
// overview.rd.ledger)。行内「立即同步」POST …/intake/sync(IntakeResult toast;离线需
// OS_ISSUE_SOURCE=fixture + OS_ENGINE_MODE=scripted,否则如实报错)。登记 POST …/repos。
import { useState } from 'react';
import { App, Button, Card, Flex, Space, Table } from 'antd';
import { PlusOutlined, SyncOutlined } from '@ant-design/icons';
import type { IntakeResult, Repo } from '../api/types';
import { listRepos, intakeSync } from '../api/endpoints';
import { useApp } from '../store/AppContext';
import { useData } from '../hooks/useApi';
import { PageHead, EmptyState } from '../components/common';
import { RepoCreateModal } from '../components/modals';
import { dispositionLabel } from '../utils/dicts';
import { fmtT } from '../utils/time';

export default function Repos() {
  const { message } = App.useApp();
  const { companyId, refreshKey, bump } = useApp();
  const [createOpen, setCreateOpen] = useState(false);
  const [syncingId, setSyncingId] = useState<string | null>(null);

  const data = useData<Repo[]>(() => (companyId ? listRepos(companyId) : Promise.reject()), {
    deps: [companyId, refreshKey],
    enabled: !!companyId,
  });
  const rows = data.data ?? [];

  const sync = async (repo: Repo) => {
    if (!companyId) return;
    setSyncingId(repo.id);
    try {
      const results = await intakeSync(companyId);
      const mine = results.find((r) => r.repo === repo.name);
      toastIntake(message, mine ?? results[0]);
      bump();
    } catch (err) {
      message.error(err instanceof Error ? err.message : String(err));
    } finally {
      setSyncingId(null);
    }
  };

  const toastIntake = (messageApi: ReturnType<typeof App.useApp>['message'], r?: IntakeResult) => {
    if (!r) {
      messageApi.warning('同步完成但无仓库返回(该仓库未登记?)');
      return;
    }
    const parts = [`${r.repo}:看到 ${r.issues_seen} · 已见 ${r.already}`];
    for (const [k, n] of Object.entries(r.by_disp)) if (n) parts.push(`${dispositionLabel(k)}×${n}`);
    if (r.created_tasks.length) parts.push(`建单 ${r.created_tasks.length}`);
    if (r.asks.length) parts.push(`问人 ${r.asks.length}`);
    messageApi.success(parts.join(' · '));
  };

  return (
    <div>
      <PageHead
        title="研发仓库 · 通道 B"
        sub="GET /api/v1/companies/{id}/repos · issue 账本在 总览→RD 卡"
        actions={
          <Button type="primary" icon={<PlusOutlined />} onClick={() => setCreateOpen(true)}>
            登记仓库
          </Button>
        }
      />

      <Card size="small" styles={{ body: { padding: 0 } }}>
        <Table<Repo>
          size="small"
          rowKey="id"
          dataSource={rows}
          pagination={{ pageSize: 12, showSizeChanger: false }}
          locale={{ emptyText: <EmptyState text="无仓库 → 登记首个 GitHub 仓库,webhook 或轮询将 intake 其 issue" /> }}
          columns={[
            {
              title: '仓库',
              render: (_, r) => (
                <div>
                  <b>{r.name}</b>
                  <div className="mono dim" style={{ fontSize: 11.5 }}>{r.repo_url}</div>
                </div>
              ),
            },
            {
              title: 'Workspace',
              render: (_, r) => <span className="mono dim" style={{ fontSize: 12 }}>{r.workspace_path || '—'}</span>,
            },
            { title: '登记时间', width: 130, render: (_, r) => <span className="dim" style={{ fontSize: 12 }}>{fmtT(r.created_at)}</span> },
            {
              title: '操作',
              width: 140,
              render: (_, r) => (
                <Button size="small" icon={<SyncOutlined />} loading={syncingId === r.id} onClick={() => sync(r)}>
                  立即同步
                </Button>
              ),
            },
          ]}
        />
      </Card>

      <Flex justify="space-between" align="center" wrap gap={8} style={{ marginTop: 10 }}>
        <Space className="dim" style={{ fontSize: 11.5 }} wrap>
          <span>处置账本(按 disposition)不在此页:GET overview → data.rd.ledger(direct_work/merge/skip/ask)。</span>
          <span>webhook:POST /api/webhook/github(公网回调)与轮询 intake 共用 UNIQUE 去重。</span>
        </Space>
      </Flex>

      <RepoCreateModal open={createOpen} onClose={() => setCreateOpen(false)} onDone={bump} />
    </div>
  );
}
