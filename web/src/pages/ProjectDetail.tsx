// 项目详情(Phase 10.1):GET /projects/{id} + /projects/{id}/pipelines + /projects/{id}/tasks(最近 runs)。
// 三 fetcher 并拉:最近 runs 带轮询(10s)让 run 状态活着;流水线行「运行」→ request Modal → POST run。
import { useState } from 'react';
import { App, Button, Card, Flex, Popconfirm, Space, Table, Tag, Typography } from 'antd';
import { ArrowLeftOutlined, DeleteOutlined, FolderOutlined, PlayCircleOutlined, PlusOutlined } from '@ant-design/icons';
import { Link, useNavigate, useParams } from 'react-router-dom';
import type { Pipeline, Project, Task } from '../api/types';
import { deletePipeline, deleteProject, getProject, listPipelines, listProjectTasks } from '../api/endpoints';
import { useApp } from '../store/AppContext';
import { useData } from '../hooks/useApi';
import { PageHead, RiskText, EmptyState } from '../components/common';
import StatusTag from '../components/StatusTag';
import { PipelineCreateModal, PipelineRunModal } from '../components/modals';
import { pipelineKindLabel, pipelineStatusLabel, pipelineStatusPreset, taskStatusLabel, taskStatusPreset } from '../utils/dicts';
import { fmtT } from '../utils/time';

export default function ProjectDetail() {
  const { id } = useParams();
  const projectId = id ?? '';
  const { message } = App.useApp();
  const { refreshKey, bump } = useApp();
  const nav = useNavigate();

  const [createOpen, setCreateOpen] = useState(false);
  const [runPipeline, setRunPipeline] = useState<Pipeline | null>(null);

  const project = useData<Project>(() => getProject(projectId), {
    deps: [projectId, refreshKey],
    enabled: !!projectId,
  });
  const pipelines = useData<Pipeline[]>(() => listPipelines(projectId), {
    deps: [projectId, refreshKey],
    enabled: !!projectId,
  });
  // 最近 runs(该项目工程任务):轮询让 running/waiting_approval 状态活着刷新。
  const runs = useData<Task[]>(() => listProjectTasks(projectId, 20), {
    deps: [projectId, refreshKey],
    intervalMs: 10000,
    enabled: !!projectId,
  });

  const p = project.data;
  const pplRows = pipelines.data ?? [];
  const runRows = runs.data ?? [];

  const delProject = async () => {
    try {
      await deleteProject(projectId);
      message.success('项目已删(元数据;磁盘未动)');
      nav('/projects');
    } catch (e) {
      message.error(e instanceof Error ? e.message : String(e));
    }
  };

  const delPipeline = async (pl: Pipeline) => {
    try {
      await deletePipeline(pl.id);
      message.success(`流水线 ${pl.name} 已删(其历史 run 任务保留)`);
      bump();
    } catch (e) {
      message.error(e instanceof Error ? e.message : String(e));
    }
  };

  return (
    <div>
      <PageHead
        title={p ? p.name : '项目'}
        sub="GET /api/v1/projects/{id} · /projects/{id}/pipelines · /projects/{id}/tasks"
        actions={
          <>
            <Link to="/projects">
              <Button icon={<ArrowLeftOutlined />}>返回</Button>
            </Link>
            {p ? (
              <Popconfirm
                title="删除项目?"
                description="须无活跃 run;历史任务断关联保留、流水线级联删,磁盘目录绝不碰。"
                okText="删除"
                okButtonProps={{ danger: true }}
                onConfirm={delProject}
              >
                <Button danger icon={<DeleteOutlined />}>
                  删除项目
                </Button>
              </Popconfirm>
            ) : null}
          </>
        }
      />

      {/* 项目信息 */}
      <Card size="small" style={{ marginBottom: 14 }}>
        {p ? (
          <Flex gap={26} wrap align="center">
            <Space size={8}>
              <FolderOutlined style={{ color: 'var(--text-3)' }} />
              <Typography.Text strong>{p.name}</Typography.Text>
            </Space>
            <div className="mono dim" style={{ fontSize: 12 }}>
              {p.root_path}
            </div>
            <div className="dim" style={{ fontSize: 12, maxWidth: 420 }}>{p.description || '—'}</div>
            <div className="dim" style={{ fontSize: 12 }}>创建于 {fmtT(p.created_at)}</div>
            <div className="dim mono" style={{ fontSize: 11 }}>{p.id}</div>
          </Flex>
        ) : (
          <EmptyState text="加载项目…" />
        )}
      </Card>

      {/* 流水线 */}
      <Card
        size="small"
        title="流水线(声明式意图)"
        extra={
          <Button size="small" type="primary" icon={<PlusOutlined />} onClick={() => setCreateOpen(true)}>
            新建流水线
          </Button>
        }
        styles={{ body: { padding: 0 } }}
      >
        <Table<Pipeline>
          size="small"
          rowKey="id"
          dataSource={pplRows}
          pagination={{ pageSize: 8, showSizeChanger: false }}
          locale={{ emptyText: <EmptyState text="无流水线 → 新建:写清意图(kind+risk),run 即可驱动 driver" /> }}
          columns={[
            {
              title: '流水线',
              render: (_, pl) => (
                <div>
                  <Space size={6}>
                    <Tag style={{ marginInlineEnd: 0 }}>{pipelineKindLabel(pl.kind)}</Tag>
                    <Typography.Text strong>{pl.name}</Typography.Text>
                    <StatusTag preset={pipelineStatusPreset(pl.status)} label={pipelineStatusLabel(pl.status)} />
                  </Space>
                  <div className="dim" style={{ fontSize: 11.5, marginTop: 2 }}>{pl.description}</div>
                </div>
              ),
            },
            {
              title: '风险',
              width: 90,
              render: (_, pl) => <RiskText risk={pl.risk} />,
            },
            { title: '创建', width: 120, render: (_, pl) => <span className="dim" style={{ fontSize: 12 }}>{fmtT(pl.created_at)}</span> },
            {
              title: '操作',
              width: 170,
              render: (_, pl) => (
                <Space size={4}>
                  <Button size="small" type="primary" ghost icon={<PlayCircleOutlined />} onClick={() => setRunPipeline(pl)}>
                    运行
                  </Button>
                  <Popconfirm
                    title="删除流水线?"
                    description="仅删元数据;其历史 run 任务保留,项目与磁盘不动。"
                    okText="删除"
                    okButtonProps={{ danger: true }}
                    onConfirm={() => delPipeline(pl)}
                  >
                    <Button size="small" danger type="text" icon={<DeleteOutlined />} />
                  </Popconfirm>
                </Space>
              ),
            },
          ]}
        />
      </Card>

      {/* 最近 runs */}
      <Card size="small" title="最近 runs(该项目工程任务,10s 轮询)" style={{ marginTop: 14 }} styles={{ body: { padding: 0 } }}>
        <Table<Task>
          size="small"
          rowKey="id"
          dataSource={runRows}
          pagination={{ pageSize: 8, showSizeChanger: false }}
          locale={{ emptyText: <EmptyState text="尚无 run → 点流水线「运行」建单(异步入队,worker 认领执行)" /> }}
          columns={[
            {
              title: '任务',
              render: (_, t) => (
                <div>
                  <Typography.Text strong>{t.title}</Typography.Text>
                  <div className="mono dim" style={{ fontSize: 11 }}>{t.id}</div>
                </div>
              ),
            },
            {
              title: '意图',
              ellipsis: true,
              render: (_, t) => <span className="dim" style={{ fontSize: 12 }}>{t.description || '—'}</span>,
            },
            { title: '状态', width: 110, render: (_, t) => <StatusTag preset={taskStatusPreset(t.status)} label={taskStatusLabel(t.status)} /> },
            { title: '风险', width: 80, render: (_, t) => <RiskText risk={t.risk} /> },
            { title: '回合', width: 80, render: (_, t) => <span className="mono dim">{t.round_no}</span> },
            { title: '开始', width: 120, render: (_, t) => <span className="dim" style={{ fontSize: 12 }}>{fmtT(t.created_at)}</span> },
          ]}
        />
      </Card>

      <PipelineCreateModal open={createOpen} projectId={projectId} onClose={() => setCreateOpen(false)} onDone={bump} />
      <PipelineRunModal open={!!runPipeline} pipeline={runPipeline} onClose={() => setRunPipeline(null)} onDone={bump} />
    </div>
  );
}
