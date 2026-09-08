// 项目列表(Phase 10.1):company 下目录容器(整目录一个 git 仓库)。建项目 root 就绪:
// 已 git 直接用 / 空或不存在 → OS mkdir+git init / 非空非 git → 400。行点开 → /projects/:id。
import { useEffect, useState } from 'react';
import { App, Button, Card, Flex, Popconfirm, Space, Table, Typography } from 'antd';
import { FolderOutlined, PlusOutlined } from '@ant-design/icons';
import type { Project } from '../api/types';
import { deleteProject, listPipelines, listProjects } from '../api/endpoints';
import { useApp } from '../store/AppContext';
import { useData } from '../hooks/useApi';
import { PageHead, EmptyState } from '../components/common';
import { ProjectCreateModal } from '../components/modals';
import { fmtT } from '../utils/time';
import { useNavigate } from 'react-router-dom';

export default function Projects() {
  const { message } = App.useApp();
  const { companyId, refreshKey, bump } = useApp();
  const [createOpen, setCreateOpen] = useState(false);
  const nav = useNavigate();

  const data = useData<Project[]>(() => (companyId ? listProjects(companyId) : Promise.reject()), {
    deps: [companyId, refreshKey],
    enabled: !!companyId,
  });
  const rows = data.data ?? [];

  // 每行流水线数(轻量并行;公司级项目数少,够用)。
  const [counts, setCounts] = useState<Record<string, number>>({});
  useEffect(() => {
    let alive = true;
    if (!rows.length) {
      setCounts({});
      return;
    }
    Promise.all(
      rows.map((p) => listPipelines(p.id).then((pls) => [p.id, pls.length] as const).catch(() => [p.id, 0] as const)),
    ).then((entries) => {
      if (alive) setCounts(Object.fromEntries(entries));
    });
    return () => {
      alive = false;
    };
  }, [rows]);

  const del = async (p: Project) => {
    try {
      await deleteProject(p.id);
      message.success(`项目 ${p.name} 已删(元数据;磁盘目录未动)`);
      bump();
    } catch (e) {
      message.error(e instanceof Error ? e.message : String(e));
    }
  };

  return (
    <div>
      <PageHead
        title="项目 · 流水线"
        sub="GET /api/v1/companies/{id}/projects · 每项目 = 一个 git 目录容器"
        actions={
          <Button type="primary" icon={<PlusOutlined />} onClick={() => setCreateOpen(true)}>
            新建项目
          </Button>
        }
      />

      <Card size="small" styles={{ body: { padding: 0 } }}>
        <Table<Project>
          size="small"
          rowKey="id"
          dataSource={rows}
          onRow={(p) => ({ onClick: () => nav(`/projects/${p.id}`), style: { cursor: 'pointer' } })}
          pagination={{ pageSize: 12, showSizeChanger: false }}
          locale={{ emptyText: <EmptyState text="无项目 → 新建:root 指向 git 仓库,或空/不存在目录由 OS git init" /> }}
          columns={[
            {
              title: '项目',
              render: (_, p) => (
                <div>
                  <Space size={6}>
                    <FolderOutlined style={{ color: 'var(--text-3)' }} />
                    <Typography.Text strong>{p.name}</Typography.Text>
                  </Space>
                  <div className="mono dim" style={{ fontSize: 11.5 }}>{p.root_path}</div>
                </div>
              ),
            },
            {
              // D7:代码源徽标 = 项目 git origin 自动认领(通道 B 依 GitHub remote 路由)
              title: '代码源',
              width: 190,
              render: (_, p) =>
                p.code_source && p.code_source.has_github ? (
                  <span className="mono" style={{ fontSize: 11.5, color: 'var(--accent)' }}>
                    {p.code_source.owner}/{p.code_source.repo}
                  </span>
                ) : (
                  <span className="dim" style={{ fontSize: 11.5 }}>—</span>
                ),
            },
            {
              title: '描述',
              ellipsis: true,
              render: (_, p) => <span className="dim" style={{ fontSize: 12 }}>{p.description || '—'}</span>,
            },
            {
              title: '流水线',
              width: 90,
              render: (_, p) => <span className="mono dim">{counts[p.id] ?? '…'}</span>,
            },
            { title: '创建', width: 120, render: (_, p) => <span className="dim" style={{ fontSize: 12 }}>{fmtT(p.created_at)}</span> },
            {
              title: '',
              width: 90,
              render: (_, p) => (
                <Popconfirm
                  title="删除项目?"
                  description="须无活跃 run;历史任务断关联保留、流水线级联删,磁盘目录绝不碰。"
                  okText="删除"
                  okButtonProps={{ danger: true }}
                  onConfirm={(e) => {
                    e?.stopPropagation?.();
                    del(p);
                  }}
                  onCancel={(e) => e?.stopPropagation?.()}
                >
                  <Button size="small" danger type="text" onClick={(e) => e.stopPropagation()}>
                    删除
                  </Button>
                </Popconfirm>
              ),
            },
          ]}
        />
      </Card>

      <Flex justify="space-between" align="center" wrap gap={8} style={{ marginTop: 10 }}>
        <Space className="dim" style={{ fontSize: 11.5 }} wrap>
          <span>run 语义:每条流水线 run = 项目目录内一条 engineering 任务(driver 回合;8.4 档位解析)。</span>
          <span>串行:同项目同时至多一条活跃 run(409)。删除仅断元数据引用,目录是 OS 资产。</span>
          <span>代码源:项目 git 的 GitHub origin remote 自动认领(通道 B issue 接活 → 归此项目)。</span>
        </Space>
      </Flex>

      <ProjectCreateModal open={createOpen} onClose={() => setCreateOpen(false)} onDone={bump} />
    </div>
  );
}
