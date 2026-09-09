// 新建类弹窗(任务/决策/记忆/端点/仓库 + 端点选模型),POST 写 + actor=human:console。
// 提交成功后统一 onDone():调用方 bump(refreshKey)+ 关窗 + 局部刷新。
import { useEffect, useMemo, useState } from 'react';
import { Alert, App, Button, Form, Input, Modal, Select } from 'antd';
import {
  addEndpoint,
  createDecision,
  createMemory,
  createPipeline,
  createProject,
  createTask,
  listCapabilities,
  runPipeline,
  selectEndpointModel,
  updatePipelineSchedule,
} from '../api/endpoints';
import type { Capability, DecisionKind, Endpoint, MemoryType, Pipeline, PlanPolicy, Risk } from '../api/types';
import { useData } from '../hooks/useApi';
import { useApp } from '../store/AppContext';
import { DECISION_KINDS, MEMORY_TYPES } from '../api/types';
import { decisionKindLabel, memoryTypeLabel } from '../utils/dicts';
import { validateCron } from '../utils/cron';
import { parseModelsCache } from '../utils/parseModelsCache';

interface ModalProps {
  open: boolean;
  onClose: () => void;
  onDone: () => void;
}

function Footer({ busy, label, onCancel }: { busy: boolean; label: string; onCancel: () => void }) {
  return [
    <Button key="cancel" onClick={onCancel} disabled={busy}>
      取消
    </Button>,
    <Button key="ok" type="primary" htmlType="submit" loading={busy}>
      {label}
    </Button>,
  ];
}

/** 新建工程请求:company_id + title 必填;risk=high → 先过审批门;actor=human:console */
export function TaskCreateModal({ open, onClose, onDone }: ModalProps) {
  const { message } = App.useApp();
  const { companyId } = useApp();
  const [busy, setBusy] = useState(false);
  const caps = useData<Capability[]>(() => (companyId ? listCapabilities(companyId) : Promise.reject()), {
    enabled: !!companyId && open,
    deps: [companyId, open],
  });

  const capOptions = useMemo(
    () =>
      (caps.data ?? []).map((c) => ({
        value: c.id,
        label: `${c.code}(${c.name})`,
      })),
    [caps.data],
  );

  const submit = async (v: { title: string; description?: string; tool_name?: string; risk?: Risk; capability_id?: string }) => {
    if (!companyId) return;
    setBusy(true);
    try {
      await createTask({
        company_id: companyId,
        title: v.title,
        description: v.description,
        tool_name: v.tool_name || undefined,
        risk: v.risk,
        capability_id: v.capability_id,
      });
      message.success(v.risk === 'high' ? '已建单:risk=high → 先过审批门' : '已建单入队:等待认领');
      onClose();
      onDone();
    } catch (e) {
      message.error(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal open={open} title="新建工程请求" onCancel={onClose} footer={null} width={520}>
      <Form layout="vertical" onFinish={submit}>
        <Form.Item name="title" label="标题 *" rules={[{ required: true, message: '必填' }]}>
          <Input placeholder="例:支持导出 PDF" maxLength={120} />
        </Form.Item>
        <Form.Item name="description" label="描述">
          <Input.TextArea rows={3} placeholder="背景 / 验收标准。engineering 工具会由 planner 拆解、driver 回合执行" />
        </Form.Item>
        <Form.Item name="tool_name" label="工具" initialValue="engineering">
          <Select
            options={['engineering', 'file-write', 'file-read', 'shell', 'git'].map((t) => ({ value: t, label: t }))}
          />
        </Form.Item>
        <Form.Item name="risk" label="风险" initialValue="medium">
          <Select
            options={[
              { value: 'low', label: 'low' },
              { value: 'medium', label: 'medium' },
              { value: 'high', label: 'high(将卡审批门)' },
            ]}
          />
        </Form.Item>
        <Form.Item name="capability_id" label="Capability">
          <Select allowClear placeholder="—" options={capOptions} loading={caps.loading} />
        </Form.Item>
        <Alert
          type="info"
          showIcon
          message="POST /api/v1/tasks → 入队;执行认领:os queue work 或 server --queue-work 自动消费"
          style={{ marginBottom: 12 }}
        />
        <div style={{ textAlign: 'right' }}>
          <Footer busy={busy} label="创建并入队" onCancel={onClose} />
        </div>
      </Form>
    </Modal>
  );
}

/** 记一笔决策:kind 真枚举 + title;status 服务端默认 made */
export function DecisionCreateModal({ open, onClose, onDone }: ModalProps) {
  const { message } = App.useApp();
  const { companyId } = useApp();
  const [busy, setBusy] = useState(false);

  const submit = async (v: { kind: DecisionKind; title: string; body?: string }) => {
    if (!companyId) return;
    setBusy(true);
    try {
      await createDecision(companyId, { kind: v.kind, title: v.title, body: v.body });
      message.success('决策已落档');
      onClose();
      onDone();
    } catch (e) {
      message.error(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal open={open} title="记一笔决策" onCancel={onClose} footer={null} width={480}>
      <Form layout="vertical" onFinish={submit}>
        <Form.Item name="kind" label="类别" initialValue="manual">
          <Select options={DECISION_KINDS.map((k) => ({ value: k, label: `${k} · ${decisionKindLabel(k)}` }))} />
        </Form.Item>
        <Form.Item name="title" label="标题 *" rules={[{ required: true, message: '必填' }]}>
          <Input placeholder="例:Q3 主打国内市场" maxLength={120} />
        </Form.Item>
        <Form.Item name="body" label="正文">
          <Input.TextArea rows={4} placeholder="背景 / 备选 / 决定 / 复盘时间" />
        </Form.Item>
        <div style={{ textAlign: 'right' }}>
          <Footer busy={busy} label="落档" onCancel={onClose} />
        </div>
      </Form>
    </Modal>
  );
}

/** 沉淀一条记忆:type 真枚举;tags 空格分隔(后端原样) */
export function MemoryCreateModal({ open, onClose, onDone }: ModalProps) {
  const { message } = App.useApp();
  const { companyId } = useApp();
  const [busy, setBusy] = useState(false);

  const submit = async (v: { type: MemoryType; title: string; content?: string; tags?: string }) => {
    if (!companyId) return;
    setBusy(true);
    try {
      await createMemory(companyId, {
        type: v.type,
        title: v.title,
        content: v.content,
        tags: v.tags?.trim() || undefined,
      });
      message.success('已沉淀进知识库');
      onClose();
      onDone();
    } catch (e) {
      message.error(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal open={open} title="沉淀一条记忆" onCancel={onClose} footer={null} width={520}>
      <Form layout="vertical" onFinish={submit}>
        <Form.Item name="type" label="类型" initialValue="knowledge">
          <Select options={MEMORY_TYPES.map((t) => ({ value: t, label: `${t} · ${memoryTypeLabel(t)}` }))} />
        </Form.Item>
        <Form.Item name="title" label="标题 *" rules={[{ required: true, message: '必填' }]}>
          <Input maxLength={120} placeholder="例:熔断不是失败信号,是升级信号" />
        </Form.Item>
        <Form.Item name="content" label="内容">
          <Input.TextArea rows={4} />
        </Form.Item>
        <Form.Item name="tags" label="标签(空格分隔)">
          <Input placeholder="例:熔断 评审" />
        </Form.Item>
        <div style={{ textAlign: 'right' }}>
          <Footer busy={busy} label="沉淀" onCancel={onClose} />
        </div>
      </Form>
    </Modal>
  );
}

/** 新增模型端点:token 仅密文落库(OS_ENDPOINT_KEY 加解密,fail-closed) */
export function EndpointCreateModal({ open, onClose, onDone }: ModalProps) {
  const { message } = App.useApp();
  const { companyId } = useApp();
  const [busy, setBusy] = useState(false);

  const submit = async (v: { name: string; base_url: string; proto?: string; token?: string }) => {
    if (!companyId) return;
    setBusy(true);
    try {
      await addEndpoint({
        company_id: companyId,
        name: v.name,
        base_url: v.base_url,
        token: v.token,
        proto: v.proto || undefined,
      });
      message.success('端点已保存(role=pool,active):拉一次模型即可选定');
      onClose();
      onDone();
    } catch (e) {
      message.error(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal open={open} title="新增模型端点" onCancel={onClose} footer={null} width={520}>
      <Form layout="vertical" onFinish={submit}>
        <Form.Item name="name" label="名称 *" rules={[{ required: true, message: '必填' }]}>
          <Input placeholder="例:claude-backup" />
        </Form.Item>
        <Form.Item name="base_url" label="Base URL *" rules={[{ required: true, message: '必填' }]}>
          <Input placeholder="https://…" className="mono" />
        </Form.Item>
        <Form.Item name="proto" label="协议" initialValue="auto">
          <Select
            options={[
              { value: 'auto', label: 'auto(按域名识别厂商)' },
              { value: 'anthropic', label: 'anthropic' },
              { value: 'openai', label: 'openai' },
            ]}
          />
        </Form.Item>
        <Form.Item name="token" label="Token">
          <Input.Password placeholder="AES-GCM 密文落库(enc:v1:);响应永不回明文" />
        </Form.Item>
        <div style={{ textAlign: 'right' }}>
          <Footer busy={busy} label="保存" onCancel={onClose} />
        </div>
      </Form>
    </Modal>
  );
}

/** 新建项目(Phase 10.1 + 自动取码):root 就绪策略 —— 已 git 直接用;空/不存在 + repo_url → OS 自动 clone;
 *  空/不存在无 repo_url → OS mkdir+git init;非空非 git → 400 */
export function ProjectCreateModal({ open, onClose, onDone }: ModalProps) {
  const { message } = App.useApp();
  const { companyId } = useApp();
  const [busy, setBusy] = useState(false);

  const submit = async (v: {
    name: string;
    root_path: string;
    description?: string;
    repo_url?: string;
  }) => {
    if (!companyId) return;
    setBusy(true);
    try {
      await createProject(companyId, {
        name: v.name,
        root_path: v.root_path.trim(),
        description: v.description,
        repo_url: (v.repo_url ?? '').trim(),
      });
      message.success('项目已建(root 已就绪为 git 仓库)');
      onClose();
      onDone();
    } catch (e) {
      message.error(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal open={open} title="新建项目" onCancel={onClose} footer={null} width={560}>
      <Form layout="vertical" onFinish={submit}>
        <Form.Item name="name" label="名称 *" rules={[{ required: true, message: '必填' }]}>
          <Input placeholder="例:acme-web(整目录一个 git 仓库的容器)" maxLength={120} />
        </Form.Item>
        <Form.Item name="root_path" label="Root 目录(绝对路径)*" rules={[{ required: true, message: '必填' }]}>
          <Input placeholder="/srv/projects/acme-web" className="mono" />
        </Form.Item>
        <Form.Item
          name="repo_url"
          label="GitHub 地址(可空)"
          tooltip="填了且 root 为空/不存在 → OS 自动把代码 clone 进项目目录并绑定代码源;留空 → 本地 mkdir + git init"
        >
          <Input placeholder="https://github.com/owner/repo(.git)" className="mono" />
        </Form.Item>
        <Form.Item name="description" label="描述">
          <Input.TextArea rows={2} placeholder="这个项目(目录)是做什么的" />
        </Form.Item>
        <Alert
          type="info"
          showIcon
          message="Root 就绪:指向已有 git 仓库直接用;空/不存在目录 + GitHub 地址 → OS clone 代码,否则 mkdir + git init。存在且非空但非 git → 400。删除项目绝不碰磁盘目录。"
          style={{ marginBottom: 12 }}
        />
        <div style={{ textAlign: 'right' }}>
          <Footer busy={busy} label="创建" onCancel={onClose} />
        </div>
      </Form>
    </Modal>
  );
}

/** 新建流水线(绑项目):kind(D6 三形态)+ 名称 + 意图描述 + 风险护栏 */
export function PipelineCreateModal({
  open,
  onClose,
  onDone,
  projectId,
}: ModalProps & { projectId: string }) {
  const { message } = App.useApp();
  const [busy, setBusy] = useState(false);

  const submit = async (v: {
    kind: Pipeline['kind'];
    name: string;
    description?: string;
    risk?: Risk;
    schedule?: string;
    plan_policy?: PlanPolicy;
  }) => {
    const cron = validateCron(v.schedule ?? '');
    if (!cron.ok) {
      message.error(`调度格式不对:${cron.message}`);
      return;
    }
    setBusy(true);
    try {
      await createPipeline(projectId, {
        name: v.name,
        kind: v.kind,
        description: v.description,
        risk: v.risk,
        schedule: cron.value || undefined,
        plan_policy: v.plan_policy || undefined,
      });
      message.success('流水线已建:run 会在项目目录内建 engineering 任务执行');
      onClose();
      onDone();
    } catch (e) {
      message.error(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal open={open} title="新建流水线" onCancel={onClose} footer={null} width={560}>
      <Form layout="vertical" onFinish={submit}>
        <Form.Item name="kind" label="形态" initialValue="bugfix">
          <Select
            options={[
              { value: 'bugfix', label: 'bugfix · 缺陷修复' },
              { value: 'develop', label: 'develop · 特性开发' },
              { value: 'ops_patrol', label: 'ops_patrol · 运维巡检(10.2 形态)' },
            ]}
          />
        </Form.Item>
        <Form.Item name="name" label="名称 *" rules={[{ required: true, message: '必填' }]}>
          <Input placeholder="例:fix-login-flaky" maxLength={120} />
        </Form.Item>
        <Form.Item name="description" label="意图描述(自然语言)" rules={[{ required: true, message: '流水线需意图文本(run 缺省即用它)' }]}>
          <Input.TextArea rows={3} placeholder="例:登录接口偶发 500,先复现、修根因并补回归" />
        </Form.Item>
        <Form.Item name="risk" label="风险护栏" initialValue="medium">
          <Select
            options={[
              { value: 'low', label: 'low' },
              { value: 'medium', label: 'medium' },
              { value: 'high', label: 'high(run 将卡审批门)' },
            ]}
          />
        </Form.Item>
        <Form.Item
          name="plan_policy"
          label="计划策略"
          initialValue="adaptive"
          extra={'自适应 = 执行中生成记账(默认);合成 = 首次认领时 frontier 先行合成整段计划 → 先审后干;合成失败自动降级自适应。巡检形态沿用例行模板,策略字段存而不用。'}
        >
          <Select
            options={[
              { value: 'adaptive', label: '自适应 · 执行中生成(grow 记账)' },
              { value: 'synthesize', label: '合成 · frontier 先合成再执行(先审后干)' },
            ]}
          />
        </Form.Item>
        <Form.Item
          name="schedule"
          label="调度(可选)"
          extra={'留空 = 不调度;cron 5 段 `分 时 日 月 周`:每天9点 `0 9 * * *` · 工作日9点 `0 9 * * 1-5` · 每30分 `*/30 * * * *`'}
        >
          <Input placeholder="0 9 * * *" className="mono" allowClear />
        </Form.Item>
        <Alert
          type="info"
          showIcon
          message="到点自动拉起需:设置页开启 定时调度轮询(>0 秒)+ 后台执行(queue work / os queue work)。到点仅自动拉单,不自动干活。"
          style={{ marginBottom: 12 }}
        />
        <div style={{ textAlign: 'right' }}>
          <Footer busy={busy} label="创建" onCancel={onClose} />
        </div>
      </Form>
    </Modal>
  );
}

/** 改流水线调度(cron 五段;清空 = 停调度):PUT /pipelines/{id} {schedule} */
export function PipelineScheduleModal({
  open,
  onClose,
  onDone,
  pipeline,
}: ModalProps & { pipeline: Pipeline | null }) {
  const { message } = App.useApp();
  const [busy, setBusy] = useState(false);
  const [value, setValue] = useState('');

  useEffect(() => {
    if (open) setValue(pipeline?.schedule ?? '');
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, pipeline?.id]);

  const save = async () => {
    if (!pipeline) return;
    const cron = validateCron(value);
    if (!cron.ok) {
      message.error(`调度格式不对:${cron.message}`);
      return;
    }
    setBusy(true);
    try {
      await updatePipelineSchedule(pipeline.id, { schedule: cron.value });
      message.success(cron.value ? `调度已更新:${cron.value}` : '调度已清除(不再定时)');
      onClose();
      onDone();
    } catch (e) {
      message.error(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal
      open={open}
      title={pipeline ? `改调度 · ${pipeline.name}` : '改调度'}
      onCancel={onClose}
      onOk={save}
      confirmLoading={busy}
      okText="保存"
      width={520}
    >
      <div className="dim" style={{ fontSize: 12.5, marginBottom: 8 }}>
        当前:cron 5 段 <span className="mono">{pipeline?.schedule || '(不调度)'}</span>(三种 kind 都支持到点 run = 定时跑一遍意图)。
      </div>
      <Input
        className="mono"
        placeholder="0 9 * * *"
        value={value}
        onChange={(e) => setValue(e.target.value)}
        allowClear
      />
      <Alert
        type="info"
        showIcon
        message="清空 = 停调度。格式:`分 时 日 月 周`(留空/off = 不调度)。到点自动拉起需设置页开启 定时调度轮询,且执行仍需后台队列(queue work)。"
        style={{ marginTop: 10 }}
      />
    </Modal>
  );
}

/** 运行流水线:请求文本(缺省 = 流水线意图)确认后 POST run → 建单入队 */
export function PipelineRunModal({
  open,
  onClose,
  onDone,
  pipeline,
}: ModalProps & { pipeline: Pipeline | null }) {
  const { message } = App.useApp();
  const [busy, setBusy] = useState(false);
  const [request, setRequest] = useState('');

  // 每次打开重置为流水线描述(本次可改;清空 = 缺省仍用描述)。
  useEffect(() => {
    if (open) setRequest(pipeline?.description ?? '');
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, pipeline?.id]);

  const submit = async () => {
    if (!pipeline) return;
    setBusy(true);
    try {
      const res = await runPipeline(pipeline.id, { request: request.trim() || undefined });
      message.success(`run 已入队:task ${res.task_id.slice(0, 8)}(${res.task.status})`);
      onClose();
      onDone();
    } catch (e) {
      message.error(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal
      open={open}
      title={pipeline ? `运行流水线 · ${pipeline.name}` : '运行流水线'}
      onCancel={onClose}
      onOk={submit}
      confirmLoading={busy}
      okText="运行"
      cancelText="取消"
      width={560}
    >
      <div className="dim" style={{ fontSize: 12, marginBottom: 8 }}>
        kind: {pipeline?.kind} · risk: {pipeline?.risk} · 每次 run = 项目目录内一条 engineering 任务(driver 回合)。
      </div>
      <Input.TextArea
        rows={4}
        value={request}
        onChange={(e) => setRequest(e.target.value)}
        placeholder="本次运行请求文本;留空 = 用流水线意图描述"
      />
      <Alert
        type="info"
        showIcon
        message="异步建单入队(qstatus ready);同项目同时仅一条活跃 run(串行守卫)。执行:os queue work 或 server 队列自驱。"
        style={{ marginTop: 10 }}
      />
    </Modal>
  );
}

/** 选择模型:POST /endpoints/{id}/select {model,role?};models_cache 防御 parse 成快选 chips */
export function EndpointSelectModelModal({
  endpoint,
  onClose,
  onDone,
}: {
  endpoint: Endpoint | null;
  onClose: () => void;
  onDone: () => void;
}) {
  const { message } = App.useApp();
  const [busy, setBusy] = useState(false);
  const [model, setModel] = useState('');
  const [role, setRole] = useState<string | undefined>(undefined);
  const [tier, setTier] = useState<string | undefined>(undefined);

  useEffect(() => {
    if (endpoint) {
      setModel(endpoint.selected_model || '');
      setRole(undefined);
      setTier(undefined);
    }
  }, [endpoint]);

  const cached = useMemo(() => parseModelsCache(endpoint?.models_cache), [endpoint]);
  if (!endpoint) return null;

  const submit = async () => {
    const m = model.trim();
    if (!m) {
      message.warning('模型不可为空');
      return;
    }
    setBusy(true);
    try {
      await selectEndpointModel(endpoint.id, m, role, tier);
      message.success(`${endpoint.name} → ${m}${role ? `(role ${role})` : ''}${tier ? `(tier ${tier})` : ''}`);
      onClose();
      onDone();
    } catch (e) {
      message.error(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal open title={`选择模型 · ${endpoint.name}`} onCancel={onClose} onOk={submit} confirmLoading={busy} width={480}>
      <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
        <div>
          <div style={{ fontSize: 12.5, color: 'var(--text-2)', marginBottom: 5, fontWeight: 500 }}>模型 *</div>
          <Input value={model} onChange={(e) => setModel(e.target.value)} placeholder="例:claude-opus-5" className="mono" />
          {cached.length ? (
            <div style={{ marginTop: 6 }}>
              <span className="dim" style={{ fontSize: 11.5 }}>已缓存:</span>{' '}
              {cached.map((m) => (
                <Button key={m} size="small" type="dashed" style={{ margin: '2px 4px 2px 0' }} onClick={() => setModel(m)}>
                  {m}
                </Button>
              ))}
            </div>
          ) : (
            <div className="dim" style={{ fontSize: 11.5, marginTop: 4 }}>
              尚无缓存 → 建议先「拉取模型」;select 不校验模型名,直填亦可
            </div>
          )}
        </div>
        <div>
          <div style={{ fontSize: 12.5, color: 'var(--text-2)', marginBottom: 5, fontWeight: 500 }}>
            角色(可选:pool|planner|standby)
          </div>
          <Select
            style={{ width: '100%' }}
            allowClear
            placeholder="不改"
            value={role}
            onChange={setRole}
            options={[
              { value: 'pool', label: 'pool · 通用池' },
              { value: 'planner', label: 'planner · 规划' },
              { value: 'standby', label: 'standby · 热备' },
            ]}
          />
        </div>
        <div>
          <div style={{ fontSize: 12.5, color: 'var(--text-2)', marginBottom: 5, fontWeight: 500 }}>
            档位(可选:frontier|standard|cheap · 8.4,与角色正交)
          </div>
          <Select
            style={{ width: '100%' }}
            allowClear
            placeholder="不改"
            value={tier}
            onChange={setTier}
            options={[
              { value: 'frontier', label: 'frontier · 高智' },
              { value: 'standard', label: 'standard · 均衡' },
              { value: 'cheap', label: 'cheap · 经济' },
            ]}
          />
        </div>
      </div>
    </Modal>
  );
}
