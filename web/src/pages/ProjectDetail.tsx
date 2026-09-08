// 项目详情(Phase 10.1 + 10.2):GET /projects/{id} + /projects/{id}/pipelines + /projects/{id}/tasks(最近 runs)。
// 10.2 增:流水线「调度」列(cron/—)+ 改调度 Modal(PUT schedule);runs 表「形态」列(pipeline_id → kind
// 徽标)+ ops_patrol 完成行「裁决」(result 解析 ok/severity/action)+ 「查看报告」→ 受控读端点纯文本抽屉。
import { useEffect, useState } from 'react';
import {
  Alert,
  App,
  Button,
  Card,
  Flex,
  Modal,
  Popconfirm,
  Space,
  Spin,
  Table,
  Tag,
  Typography,
} from 'antd';
import {
  ArrowLeftOutlined,
  DeleteOutlined,
  FileTextOutlined,
  FolderOutlined,
  OrderedListOutlined,
  PlayCircleOutlined,
  PlusOutlined,
  ScheduleOutlined,
} from '@ant-design/icons';
import { Link, useNavigate, useParams } from 'react-router-dom';
import type { Approval, PatrolReport, Pipeline, Project, Task } from '../api/types';
import {
  deletePipeline,
  deleteProject,
  getPatrolReport,
  getProject,
  listPipelines,
  listProjectTasks,
} from '../api/endpoints';
import { useApp } from '../store/AppContext';
import { useData } from '../hooks/useApi';
import { PageHead, RiskText, EmptyState } from '../components/common';
import StatusTag from '../components/StatusTag';
import DecideModal from '../components/DecideModal';
import TaskDrawer from '../components/TaskDrawer';
import { PipelineCreateModal, PipelineRunModal, PipelineScheduleModal } from '../components/modals';
import { pipelineKindLabel, pipelineStatusLabel, pipelineStatusPreset, taskStatusLabel, taskStatusPreset } from '../utils/dicts';
import { parsePatrolResult } from '../utils/patrol';
import { fmtT } from '../utils/time';

export default function ProjectDetail() {
  const { id } = useParams();
  const projectId = id ?? '';
  const { message } = App.useApp();
  const { refreshKey, bump } = useApp();
  const nav = useNavigate();

  const [createOpen, setCreateOpen] = useState(false);
  const [runPipeline, setRunPipeline] = useState<Pipeline | null>(null);
  const [editSched, setEditSched] = useState<Pipeline | null>(null);
  const [repTask, setRepTask] = useState<Task | null>(null);
  const [planTask, setPlanTask] = useState<string | null>(null); // 10.3:runs 行「计划」→ TaskDrawer
  const [decideApproval, setDecideApproval] = useState<Approval | null>(null);

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
  const pplById = new Map(pplRows.map((pl) => [pl.id, pl]));

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
        sub="GET /api/v1/projects/{id} · /projects/{id}/pipelines · /projects/{id}/tasks · /projects/{id}/patrol/{taskID}"
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
              title: '调度 · 策略',
              width: 186,
              render: (_, pl) => (
                <div>
                  {pl.schedule ? (
                    <span className="mono" style={{ fontSize: 12 }}>{pl.schedule}</span>
                  ) : (
                    <span className="dim" style={{ fontSize: 12 }}>—</span>
                  )}
                  {pl.plan_policy === 'synthesize' ? (
                    <div>
                      <span className="pill pill-accent" style={{ fontSize: 11 }}>合成计划</span>
                    </div>
                  ) : null}
                </div>
              ),
            },
            {
              title: '风险',
              width: 90,
              render: (_, pl) => <RiskText risk={pl.risk} />,
            },
            { title: '创建', width: 110, render: (_, pl) => <span className="dim" style={{ fontSize: 12 }}>{fmtT(pl.created_at)}</span> },
            {
              title: '操作',
              width: 226,
              render: (_, pl) => (
                <Space size={4}>
                  <Button size="small" type="primary" ghost icon={<PlayCircleOutlined />} onClick={() => setRunPipeline(pl)}>
                    运行
                  </Button>
                  <Button size="small" icon={<ScheduleOutlined />} onClick={() => setEditSched(pl)}>
                    改调度
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
              title: '形态',
              width: 142,
              render: (_, t) => {
                if (!t.pipeline_id) return <span className="dim" style={{ fontSize: 12 }}>—</span>;
                const pl = pplById.get(t.pipeline_id);
                if (pl) {
                  return (
                    <div>
                      <Tag>{pipelineKindLabel(pl.kind)}</Tag>
                      {pl.plan_policy === 'synthesize' ? (
                        <div>
                          <span className="pill pill-accent" style={{ fontSize: 11 }}>合成</span>
                        </div>
                      ) : null}
                    </div>
                  );
                }
                return <Tag style={{ opacity: 0.6 }}>run(流水线已删)</Tag>;
              },
            },
            {
              title: '意图',
              ellipsis: true,
              render: (_, t) => <span className="dim" style={{ fontSize: 12 }}>{t.description || '—'}</span>,
            },
            { title: '状态', width: 104, render: (_, t) => <StatusTag preset={taskStatusPreset(t.status)} label={taskStatusLabel(t.status)} /> },
            { title: '风险', width: 76, render: (_, t) => <RiskText risk={t.risk} /> },
            {
              title: '裁决(巡检)',
              width: 212,
              render: (_, t) => <PatrolVerdictCell task={t} onOpen={() => setRepTask(t)} />,
            },
            { title: '开始', width: 110, render: (_, t) => <span className="dim" style={{ fontSize: 12 }}>{fmtT(t.created_at)}</span> },
            {
              title: '计划',
              width: 88,
              render: (_, t) => (
                <Button
                  size="small"
                  type="link"
                  icon={<OrderedListOutlined />}
                  style={{ paddingInline: 4 }}
                  onClick={() => setPlanTask(t.id)}
                >
                  计划
                </Button>
              ),
            },
          ]}
        />
      </Card>

      <PipelineCreateModal open={createOpen} projectId={projectId} onClose={() => setCreateOpen(false)} onDone={bump} />
      <PipelineRunModal open={!!runPipeline} pipeline={runPipeline} onClose={() => setRunPipeline(null)} onDone={bump} />
      <PipelineScheduleModal open={!!editSched} pipeline={editSched} onClose={() => setEditSched(null)} onDone={bump} />
      <PatrolReportModal task={repTask} projectId={projectId} onClose={() => setRepTask(null)} />
      <TaskDrawer taskId={planTask} onClose={() => setPlanTask(null)} onOpenDecide={(a) => setDecideApproval(a)} />
      <DecideModal
        approval={decideApproval}
        onClose={() => setDecideApproval(null)}
        onDone={() => {
          setDecideApproval(null);
          setPlanTask(null);
          bump();
        }}
      />
    </div>
  );
}

// ---- 裁决列 ----

function PatrolVerdictCell({ task, onOpen }: { task: Task; onOpen: () => void }) {
  const v = task.status === 'completed' ? parsePatrolResult(task.result) : null;
  if (!v) {
    // 非巡检产物(非 completed 或 result 无 patrol: 前缀)→ 无裁决可看。
    return <span className="dim" style={{ fontSize: 12 }}>—</span>;
  }
  return (
    <Space size={6} wrap>
      <Tag color={v.ok ? 'success' : 'error'} style={{ marginInlineEnd: 0 }}>
        {v.ok ? '通过' : `发现 · ${v.severity}`}
      </Tag>
      <Tag color={v.action === 'fix' ? 'processing' : 'default'} style={{ marginInlineEnd: 0 }}>
        {v.action === 'fix' ? '自动处置(fix)' : v.action === 'none' ? '仅通知' : v.action}
      </Tag>
      <Button size="small" type="link" icon={<FileTextOutlined />} onClick={onOpen} style={{ paddingInline: 4 }}>
        查看报告
      </Button>
    </Space>
  );
}

// ---- 报告抽屉(受控读端点 GET /projects/{id}/patrol/{taskID};纯文本,限长截断带标记) ----

function PatrolReportModal({ task, projectId, onClose }: { task: Task | null; projectId: string; onClose: () => void }) {
  const [loading, setLoading] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const [rep, setRep] = useState<PatrolReport | null>(null);

  useEffect(() => {
    if (!task) return;
    let live = true;
    setLoading(true);
    setErr(null);
    setRep(null);
    getPatrolReport(projectId, task.id)
      .then((r) => {
        if (live) setRep(r);
      })
      .catch((e) => {
        if (live) setErr(e instanceof Error ? e.message : String(e));
      })
      .finally(() => {
        if (live) setLoading(false);
      });
    return () => {
      live = false;
    };
  }, [task, projectId]);

  const v = task ? parsePatrolResult(task.result) : null;

  return (
    <Modal
      open={!!task}
      onCancel={onClose}
      footer={
        <Button type="primary" onClick={onClose}>
          关闭
        </Button>
      }
      title={task ? `巡检报告 · ${task.title}` : '巡检报告'}
      width={760}
    >
      {task && (
        <div>
          {v && (
            <Alert
              type={v.ok ? 'success' : 'warning'}
              showIcon
              style={{ marginBottom: 10 }}
              message={`裁决:${v.ok ? '通过' : `发现 ${v.severity}`} · 处置:${v.action === 'fix' ? '自动拉起处置链(fix)' : '仅通知(none)'}`}
              description={<span className="dim">{v.summary}</span>}
            />
          )}
          {loading ? (
            <Flex justify="center" style={{ padding: 24 }}>
              <Spin />
            </Flex>
          ) : err ? (
            <Alert type="error" showIcon message="读取报告失败" description={err} />
          ) : rep ? (
            <div>
              <div className="mono dim" style={{ fontSize: 11, marginBottom: 8 }}>
                {rep.path}
              </div>
              {rep.truncated && (
                <Alert
                  type="warning"
                  showIcon
                  style={{ marginBottom: 8 }}
                  message="报告已超过显示上限,正文截断"
                  description={`后端读端点单文件上限 ${Math.round((64 * 1024) / 1024)} KiB,完整内容见项目目录该文件。`}
                />
              )}
              <pre
                style={{
                  maxHeight: 480,
                  overflow: 'auto',
                  whiteSpace: 'pre-wrap',
                  wordBreak: 'break-word',
                  margin: 0,
                  padding: 12,
                  background: 'var(--bg-2, rgba(127,127,127,0.06))',
                  borderRadius: 6,
                  fontSize: 12,
                  lineHeight: 1.6,
                }}
              >
                {rep.content || '(空正文)'}
              </pre>
            </div>
          ) : null}
        </div>
      )}
    </Modal>
  );
}
