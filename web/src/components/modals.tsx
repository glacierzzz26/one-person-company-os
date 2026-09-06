// 新建类弹窗(任务/决策/记忆/端点/仓库 + 端点选模型),POST 写 + actor=human:console。
// 提交成功后统一 onDone():调用方 bump(refreshKey)+ 关窗 + 局部刷新。
import { useEffect, useMemo, useState } from 'react';
import { Alert, App, Button, Form, Input, Modal, Select } from 'antd';
import {
  addEndpoint,
  addRepo,
  createDecision,
  createMemory,
  createTask,
  listCapabilities,
  selectEndpointModel,
} from '../api/endpoints';
import type { Capability, DecisionKind, Endpoint, MemoryType, Risk } from '../api/types';
import { useData } from '../hooks/useApi';
import { useApp } from '../store/AppContext';
import { DECISION_KINDS, MEMORY_TYPES } from '../api/types';
import { decisionKindLabel, memoryTypeLabel } from '../utils/dicts';
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

/** 登记仓库(通道 B):登记后 webhook / 轮询 intake */
export function RepoCreateModal({ open, onClose, onDone }: ModalProps) {
  const { message } = App.useApp();
  const { companyId } = useApp();
  const [busy, setBusy] = useState(false);

  const submit = async (v: { name: string; repo_url: string; workspace?: string }) => {
    if (!companyId) return;
    setBusy(true);
    try {
      await addRepo(companyId, { name: v.name, repo_url: v.repo_url, workspace: v.workspace });
      message.success('仓库已登记:点「立即同步」拉首批 issue');
      onClose();
      onDone();
    } catch (e) {
      message.error(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal open={open} title="登记仓库(通道 B)" onCancel={onClose} footer={null} width={520}>
      <Form layout="vertical" onFinish={submit}>
        <Form.Item name="name" label="名称 *" rules={[{ required: true, message: '必填' }]}>
          <Input placeholder="例:acme/web" />
        </Form.Item>
        <Form.Item name="repo_url" label="Repo URL *" rules={[{ required: true, message: '必填' }]}>
          <Input placeholder="https://github.com/acme/app" className="mono" />
        </Form.Item>
        <Form.Item name="workspace" label="Workspace">
          <Input placeholder="/srv/ws/app" className="mono" />
        </Form.Item>
        <Alert
          type="info"
          showIcon
          message="POST /api/webhook/github(公网)与轮询都会 intake;GitHub secret 与 OS_API_TOKEN 无关"
          style={{ marginBottom: 12 }}
        />
        <div style={{ textAlign: 'right' }}>
          <Footer busy={busy} label="登记" onCancel={onClose} />
        </div>
      </Form>
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
