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
  Input,
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
  EditOutlined,
  FileTextOutlined,
  FolderOutlined,
  GithubOutlined,
  KeyOutlined,
  LinkOutlined,
  OrderedListOutlined,
  PlayCircleOutlined,
  PlusOutlined,
  ReloadOutlined,
  ScheduleOutlined,
  SyncOutlined,
} from '@ant-design/icons';
import { Link, useNavigate, useParams } from 'react-router-dom';
import type {
  Approval,
  IntakeResult,
  PatrolReport,
  Pipeline,
  Project,
  SecretMeta,
  Task,
} from '../api/types';
import {
  deletePipeline,
  deleteProject,
  deleteProjectSecret,
  getPatrolReport,
  getProject,
  listPipelines,
  listProjectSecrets,
  listProjectTasks,
  publishPullRequest,
  refreshProjectCodeSource,
  setProjectSecret,
  syncProjectIssues,
  updateProject,
} from '../api/endpoints';
import { useApp } from '../store/AppContext';
import { useData } from '../hooks/useApi';
import { PageHead, RiskText, EmptyState } from '../components/common';
import StatusTag from '../components/StatusTag';
import DecideModal from '../components/DecideModal';
import TaskDrawer from '../components/TaskDrawer';
import { PipelineCreateModal, PipelineRunModal, PipelineScheduleModal } from '../components/modals';
import {
  dispositionLabel,
  pipelineKindLabel,
  pipelineStatusLabel,
  pipelineStatusPreset,
  taskStatusLabel,
  taskStatusPreset,
} from '../utils/dicts';
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
  const [syncing, setSyncing] = useState(false); // 10.5:同步 issue(项目级 SyncProject = 只同步本项目绑定代码源)
  const [refreshing, setRefreshing] = useState(false); // D7:重认领代码源(remote 后补/更换后)
  const [editOpen, setEditOpen] = useState(false); // 10.5:编辑项目(名称/描述/GitHub 绑定地址)
  const [tokOpen, setTokOpen] = useState(false); // 10.5:项目 github_token 设/换/删
  const [publishingId, setPublishingId] = useState<string | null>(null); // 10.5:runs 行人工发 PR 进行中

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
  // 10.5:项目级机密元数据(github_token 掩码;值明文永不出 API)。
  const secrets = useData<SecretMeta[]>(() => listProjectSecrets(projectId), {
    deps: [projectId, refreshKey],
    enabled: !!projectId,
  });

  const p = project.data;
  const pplRows = pipelines.data ?? [];
  const runRows = runs.data ?? [];
  const pplById = new Map(pplRows.map((pl) => [pl.id, pl]));
  const ghToken = (secrets.data ?? []).find((m) => m.id === 'github_token');

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

  // Phase 10.5:通道 B 同步 — 项目级 SyncProject(只同步本项目绑定代码源;项目 token → 公司回退)。
  const syncIssues = async () => {
    if (!p) return;
    setSyncing(true);
    try {
      const r = await syncProjectIssues(p.id);
      toastIntake(message, r);
      bump();
    } catch (e) {
      message.error(e instanceof Error ? e.message : String(e));
    } finally {
      setSyncing(false);
    }
  };

  // 10.5:人工重试发收尾 PR(完成态 + 幂等;失败 → message 展示后端明确原因)。
  const publishPR = async (t: Task) => {
    setPublishingId(t.id);
    try {
      await publishPullRequest(t.id);
      message.success('PR 已发起(或此前已存在)');
      bump();
    } catch (e) {
      message.error(e instanceof Error ? e.message : String(e));
    } finally {
      setPublishingId(null);
    }
  };

  // D7:重新认领代码源 — 项目根 git 的 GitHub origin remote → repos 绑定行(建后补 remote / 换 remote)。
  const refreshCode = async () => {
    if (!p) return;
    setRefreshing(true);
    try {
      const cs = await refreshProjectCodeSource(p.id);
      message.success(cs.has_github ? `代码源已认领:${cs.owner}/${cs.repo}` : `代码源已认领:${cs.repo_url}`);
      bump();
    } catch (e) {
      message.error(e instanceof Error ? e.message : String(e));
    } finally {
      setRefreshing(false);
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
              <>
                <Button icon={<EditOutlined />} onClick={() => setEditOpen(true)}>
                  编辑
                </Button>
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
              </>
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

      {/* 代码源(D7:仓库收敛为项目的代码源绑定 —— 项目 git 的 GitHub origin 自动认领) */}
      <Card
        size="small"
        title={
          <Space size={6}>
            <GithubOutlined />
            代码源 · 通道 B
          </Space>
        }
        extra={
          <Space size={4}>
            <Button size="small" icon={<SyncOutlined />} loading={syncing} disabled={!p} onClick={syncIssues}>
              同步 issue
            </Button>
            <Button
              size="small"
              icon={<ReloadOutlined />}
              loading={refreshing}
              disabled={!p}
              onClick={refreshCode}
              title="按项目根 git origin remote (重)认领代码源"
            >
              重新认领
            </Button>
          </Space>
        }
        style={{ marginBottom: 14 }}
      >
        {p && p.code_source && p.code_source.has_github ? (
          <Flex vertical gap={8}>
            <Flex gap={22} wrap align="center">
              <span className="mono" style={{ fontSize: 13, color: 'var(--accent)' }}>
                {p.code_source.owner}/{p.code_source.repo}
              </span>
              <span className="mono dim" style={{ fontSize: 12 }}>{p.code_source.repo_url}</span>
              <span className="dim" style={{ fontSize: 12 }}>
                从本项目 git origin 认领;issue 接活(通道 B)→ 归此项目、用项目目录干;任务完成自动推 issue 分支并开 PR(需下方项目 token)
              </span>
            </Flex>
            <Flex align="center" justify="space-between" wrap gap={8}>
              <Space size={6} className="dim" style={{ fontSize: 12 }}>
                <KeyOutlined />
                <span>项目 GitHub token(写路径:push + 开 PR 唯一凭据):</span>
                {ghToken && ghToken.set ? (
                  <Tag color="success" style={{ marginInlineEnd: 0 }}>
                    已设置
                  </Tag>
                ) : (
                  <Tag style={{ marginInlineEnd: 0 }}>未设置</Tag>
                )}
              </Space>
              <Button size="small" icon={<KeyOutlined />} onClick={() => setTokOpen(true)}>
                {ghToken && ghToken.set ? '更换 / 清除 token' : '设置 token'}
              </Button>
            </Flex>
          </Flex>
        ) : (
          <Flex gap={12} wrap align="center" justify="space-between">
            <Space className="dim" style={{ fontSize: 12 }} wrap>
              <span>无 GitHub origin remote(通道 B 不适用;issue 仍可经手工 legacy 行同步)。</span>
              <span className="mono">git remote add origin &lt;github url&gt;</span>
            </Space>
            <span className="dim" style={{ fontSize: 12 }}>补上后点「重新认领」即自动建代码源</span>
          </Flex>
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
            {
              // 10.5 收尾 PR:有 url → 外链 Tag;completed + 项目绑 GitHub + 无 url → 「发起 PR」人工重试。
              title: 'PR',
              width: 128,
              render: (_, t) => (
                <PRCell
                  task={t}
                  hasGithub={!!p?.code_source?.has_github}
                  publishing={publishingId === t.id}
                  onPublish={() => publishPR(t)}
                />
              ),
            },
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
      <ProjectEditModal open={editOpen} project={p ?? null} onClose={() => setEditOpen(false)} onDone={bump} />
      <ProjectTokenModal open={tokOpen} project={p ?? null} token={ghToken ?? null} onClose={() => setTokOpen(false)} onDone={bump} />
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

// ---- 通道 B 同步结果 toast(D7;原 Repos.tsx toastIntake 迁入) ----

function toastIntake(messageApi: ReturnType<typeof App.useApp>['message'], r?: IntakeResult) {
  if (!r) {
    messageApi.warning('同步完成但无代码源返回(公司下没有 GitHub 代码源?)');
    return;
  }
  const parts = [`${r.repo}:看到 ${r.issues_seen} · 已见 ${r.already}`];
  for (const [k, n] of Object.entries(r.by_disp)) if (n) parts.push(`${dispositionLabel(k)}×${n}`);
  if (r.created_tasks.length) parts.push(`建单 ${r.created_tasks.length}`);
  if (r.asks.length) parts.push(`问人 ${r.asks.length}`);
  messageApi.success(parts.join(' · '));
}

// ---- PR 列(10.5 收尾 PR:有 url 外链;completed + 绑 GitHub 无 url → 人工「发起 PR」重试)----

function PRCell({
  task,
  hasGithub,
  publishing,
  onPublish,
}: {
  task: Task;
  hasGithub: boolean;
  publishing: boolean;
  onPublish: () => void;
}) {
  if (task.pull_request_url) {
    return (
      <a href={task.pull_request_url} target="_blank" rel="noreferrer">
        <Tag color="success" style={{ marginInlineEnd: 0 }}>
          {task.pull_request_number ? `PR #${task.pull_request_number}` : 'PR'}
        </Tag>
      </a>
    );
  }
  if (task.status === 'completed' && hasGithub) {
    return (
      <Button
        size="small"
        type="link"
        icon={<LinkOutlined />}
        loading={publishing}
        onClick={onPublish}
        style={{ paddingInline: 4 }}
        title="收尾自动发 PR 未成功/未设 token;完成态可人工重试(改 token 或 GitHub 抖动后)"
      >
        发起 PR
      </Button>
    );
  }
  return <span className="dim" style={{ fontSize: 12 }}>—</span>;
}

// ---- 项目编辑 modal(10.5:名称/描述 + GitHub 绑定地址;root_path 不可编辑)----

function ProjectEditModal({
  open,
  project,
  onClose,
  onDone,
}: {
  open: boolean;
  project: Project | null;
  onClose: () => void;
  onDone: () => void;
}) {
  const { message } = App.useApp();
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [repoURL, setRepoURL] = useState('');
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (open && project) {
      setName(project.name);
      setDescription(project.description);
      setRepoURL(project.code_source?.repo_url ?? '');
    }
  }, [open, project]);

  const save = async () => {
    if (!project) return;
    if (!name.trim()) {
      message.warning('名称不能为空');
      return;
    }
    setSaving(true);
    try {
      await updateProject(project.id, { name, description, repo_url: repoURL.trim() });
      message.success('项目已更新');
      onDone();
      onClose();
    } catch (e) {
      message.error(e instanceof Error ? e.message : String(e));
    } finally {
      setSaving(false);
    }
  };

  return (
    <Modal
      open={open}
      onCancel={onClose}
      onOk={save}
      confirmLoading={saving}
      okText="保存"
      cancelText="取消"
      title="编辑项目"
    >
      <Space direction="vertical" size={12} style={{ width: '100%' }}>
        <div>
          <Typography.Text strong>名称</Typography.Text>
          <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="项目名称" style={{ marginTop: 4 }} />
        </div>
        <div>
          <Typography.Text strong>描述</Typography.Text>
          <Input.TextArea
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            placeholder="项目描述(可空)"
            autoSize={{ minRows: 2, maxRows: 4 }}
            style={{ marginTop: 4 }}
          />
        </div>
        <div>
          <Typography.Text strong>GitHub 绑定地址(repo_url)</Typography.Text>
          <Input
            value={repoURL}
            onChange={(e) => setRepoURL(e.target.value)}
            placeholder="https://github.com/owner/repo(留空不改挂;改动见下约束)"
            className="mono"
            style={{ marginTop: 4 }}
          />
        </div>
        <Alert
          type="info"
          showIcon
          message="绑定地址改动约束"
          description={
            <span className="dim">
              root 目录空/不存在 → OS 自动 clone 认领;仅 git init 的零提交占位仓 → 删 .git 再 clone;
              老项目(带本地 git 历史 / 已指向其它远端)→ 拒绝改挂(暂不做迁移,请新建项目绑定)。root_path 不可编辑。
            </span>
          }
        />
      </Space>
    </Modal>
  );
}

// ---- 项目 github_token modal(10.5:写路径唯一凭据;值永不回显)----

function ProjectTokenModal({
  open,
  project,
  token,
  onClose,
  onDone,
}: {
  open: boolean;
  project: Project | null;
  token: SecretMeta | null | undefined;
  onClose: () => void;
  onDone: () => void;
}) {
  const { message } = App.useApp();
  const [val, setVal] = useState('');
  const [busy, setBusy] = useState(false);

  const save = async () => {
    if (!project) return;
    if (!val.trim()) {
      message.warning('请输入 GitHub token(不能留空)');
      return;
    }
    setBusy(true);
    try {
      await setProjectSecret(project.id, 'github_token', val.trim());
      message.success('项目 token 已设置(git push + 开 PR 走它;明文永不回显)');
      setVal('');
      onDone();
      onClose();
    } catch (e) {
      message.error(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  const clear = async () => {
    if (!project) return;
    setBusy(true);
    try {
      await deleteProjectSecret(project.id, 'github_token');
      message.success('项目 token 已清除(收尾不再自动发 PR;同步 issue 的读端回退公司 token)');
      setVal('');
      onDone();
      onClose();
    } catch (e) {
      message.error(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal
      open={open}
      onCancel={onClose}
      title="项目 GitHub token"
      footer={
        token && token.set ? (
          <Flex justify="space-between">
            <Button danger onClick={clear} loading={busy}>
              清除 token
            </Button>
            <Space>
              <Button onClick={onClose}>取消</Button>
              <Button type="primary" loading={busy} onClick={save}>
                保存
              </Button>
            </Space>
          </Flex>
        ) : (
          <Space>
            <Button onClick={onClose}>取消</Button>
            <Button type="primary" loading={busy} onClick={save}>
              设置
            </Button>
          </Space>
        )
      }
    >
      <Space direction="vertical" size={10} style={{ width: '100%' }}>
        <Alert
          type="info"
          showIcon
          message="写路径(token)与读路径回退"
          description={
            <span className="dim">
              本项目推送分支 + 开 PR 只认这个项目 token(公司级 github_token 只读,绝不用于 push/PR)。
              token 明文只进本地加密封存(enc:v2 AES-GCM),永不回显、不入库、不入日志。
            </span>
          }
        />
        <div>
          <Typography.Text strong>token(设置 / 更换即覆盖)</Typography.Text>
          <Input.Password
            value={val}
            onChange={(e) => setVal(e.target.value)}
            placeholder={token && token.set ? '粘贴新 token 以更换(留空保存无效)' : 'ghp_… / github_pat_…'}
            style={{ marginTop: 4 }}
            autoComplete="new-password"
          />
        </div>
        {token && token.set ? (
          <div className="dim" style={{ fontSize: 12 }}>
            当前已设置(掩码);更换 = 直接粘贴新值保存。
          </div>
        ) : null}
      </Space>
    </Modal>
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
