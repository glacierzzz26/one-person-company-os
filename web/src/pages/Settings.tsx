// 设置(Phase 9.3,契约 runtime-knobs-web.md §3.7):六卡自上而下 ——
//  1 控制台访问(既有卡,轮换令牌)/ 2 全局默认引擎 / 3 Server 参数(重启生效)/
//  4 公司覆盖(当前公司)/ 5 公司机密(当前公司)/ 6 更换主密钥(re-key 一次性动作)。
// engine_mode Web 只读 live(scripted 仅 CLI/env 测试线);机密值明文永不回显(list 只出掩码)。
// 写操作成功后 bump()(useData deps 含 refreshKey)整体重拉;401 走全局 AuthModal。
import { useState } from 'react';
import {
  Alert,
  App,
  Button,
  Card,
  Form,
  Input,
  InputNumber,
  Modal,
  Select,
  Space,
  Spin,
  Switch,
  Tag,
  Typography,
} from 'antd';
import { CheckCircleFilled, ExclamationCircleFilled } from '@ant-design/icons';
import {
  deleteSecret,
  getCompanySettings,
  getGlobalSettings,
  listSecrets,
  resetCompanySettings,
  rotateConsoleToken,
  rotateMasterKey,
  setSecret,
  updateCompanySettings,
  updateGlobalSettings,
} from '../api/endpoints';
import type {
  CompanySettings,
  GlobalSettings,
  SecretMeta,
  UpdateCompanySettingsReq,
  UpdateGlobalSettingsReq,
} from '../api/types';
import { setToken } from '../api/client';
import { useApp } from '../store/AppContext';
import { useData } from '../hooks/useApi';
import { PageHead } from '../components/common';
import { fmtT } from '../utils/time';

const { Paragraph, Text } = Typography;
const INHERIT = '__inherit__'; // 下拉哨兵:清该维度覆盖(回退继承 global)

const AGENT_OPTIONS = [
  { value: 'claude', label: 'claude(Claude Code)' },
  { value: 'codex', label: 'codex' },
];
const ISSUE_OPTIONS = [
  { value: INHERIT, label: '继承默认(github)' },
  { value: 'github', label: 'github(GitHub REST)' },
  { value: 'fixture', label: 'fixture(离线文件;需路径)' },
];
const SECRET_META: Record<string, { label: string; desc: string }> = {
  github_token: {
    label: 'github_token',
    desc: 'GitHub REST token —— 通道 B issue 源(公司 issue_source=github 时必配)',
  },
  feishu_webhook: {
    label: 'feishu_webhook',
    desc: '飞书自定义机器人 webhook —— 审批即时通知 + 每日日报 sink',
  },
  feishu_secret: {
    label: 'feishu_secret',
    desc: '飞书加签 secret(可选;机器人开启加签校验时配)',
  },
};
const secretMetaOf = (id: string) =>
  SECRET_META[id] ?? { label: id, desc: '白名单外残留(手工/历史),可移除' };

export default function Settings() {
  const { companyId, refreshKey } = useApp();

  // 全局生效行(app_setting;缺行后端返回内置默认,console_token_set=false)。
  const gs = useData<GlobalSettings>(getGlobalSettings, { deps: [refreshKey] });
  // 公司覆盖行 + 公司机密(随当前公司;未选公司不拉)。
  const cs = useData<CompanySettings>(
    () => (companyId ? getCompanySettings(companyId) : Promise.reject(new Error('未选择公司'))),
    { deps: [companyId, refreshKey], enabled: !!companyId },
  );
  const secs = useData<SecretMeta[]>(
    () => (companyId ? listSecrets(companyId) : Promise.reject(new Error('未选择公司'))),
    { deps: [companyId, refreshKey], enabled: !!companyId },
  );

  const initialized = gs.data?.console_token_set ?? false;

  return (
    <div style={{ maxWidth: 880 }}>
      <PageHead
        title="设置"
        sub="配置治理:控制台访问 · 全局默认 · server 参数 · 公司覆盖/机密 · 换主密钥"
      />
      {gs.error && (
        <Alert
          type="error"
          showIcon
          style={{ marginBottom: 16 }}
          message="读取全局设置失败"
          description={gs.error}
        />
      )}

      <ConsoleTokenCard initialized={initialized} />

      {gs.data && (
        <>
          <GlobalDefaultsCard gs={gs.data} />
          <ServerParamsCard gs={gs.data} />
        </>
      )}

      {companyId ? (
        <>
          {cs.data ? (
            <CompanyOverrideCard key={`${companyId}:${cs.data.updated_at}`} companyId={companyId} cs={cs.data} />
          ) : (
            <Spin style={{ display: 'block', margin: '24px auto' }} />
          )}
          {secs.data ? (
            <CompanySecretsCard companyId={companyId} secrets={secs.data} />
          ) : (
            <Spin style={{ display: 'block', margin: '24px auto' }} />
          )}
        </>
      ) : (
        <Card title="公司覆盖 / 机密(作用于当前公司)" style={{ marginBottom: 16 }}>
          <Alert
            type="info"
            showIcon
            message="尚未选择公司"
            description="请先在页首公司选择器选定一家公司,再配置该公司覆盖与机密(通知/issue 源等按公司解析)。"
          />
        </Card>
      )}

      <RotateMasterKeyCard initialized={initialized} />
    </div>
  );
}

// ---- 卡 1:控制台访问(Phase 9.2 既有;init 来源改为全局设置 console_token_set)----

function ConsoleTokenCard({ initialized }: { initialized: boolean }) {
  const { message } = App.useApp();
  const { bump } = useApp();
  const [busy, setBusy] = useState(false);
  const [form] = Form.useForm<{ new_token: string }>();

  const rotate = async (v: { new_token: string }) => {
    setBusy(true);
    try {
      const token = v.new_token.trim(); // 与后端 trim 后哈希语义对齐
      await rotateConsoleToken({ new_token: token });
      setToken(token); // 旧令牌已失效,当前会话无缝切到新令牌
      form.resetFields();
      message.success('控制台令牌已轮换(旧令牌立即失效)');
      bump();
    } catch (e) {
      message.error(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Card title="控制台访问" style={{ marginBottom: 16 }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 12 }}>
        状态
        {initialized ? (
          <Tag color="success" icon={<CheckCircleFilled />}>
            已初始化(令牌哈希已设)
          </Tag>
        ) : (
          <Tag>未初始化(开放)</Tag>
        )}
      </div>

      <Paragraph type="secondary" style={{ fontSize: 13, lineHeight: 1.9 }}>
        <Text code>/api/v1</Text> 以 <Text code>Bearer &lt;令牌&gt;</Text> 鉴权,数据库只存该令牌的
        sha256 哈希(<Text code>console_token_hash</Text>)。轮换 = 换新哈希,<b>旧令牌立即失效</b>;
        成功后请同步更新你各处保存的令牌。
      </Paragraph>

      <Form
        form={form}
        layout="vertical"
        onFinish={rotate}
        disabled={busy || !initialized}
        style={{ marginTop: 4 }}
      >
        <Form.Item
          name="new_token"
          label="新控制台令牌"
          rules={[
            { required: true, message: '必填' },
            { min: 8, message: '至少 8 个字符' },
          ]}
        >
          <Input.Password placeholder="新令牌(≥8 字符)…" autoComplete="new-password" />
        </Form.Item>
        <Button type="primary" htmlType="submit" loading={busy}>
          轮换令牌
        </Button>
        {!initialized && (
          <Alert style={{ marginTop: 12 }} type="warning" showIcon message="尚未完成 /setup,令牌为空无需轮换。" />
        )}
      </Form>
    </Card>
  );
}

// ---- 卡 2:全局默认 / 引擎(engine_mode 只读 live;agent_cli 可改)----

function GlobalDefaultsCard({ gs }: { gs: GlobalSettings }) {
  const { message } = App.useApp();
  const { bump } = useApp();
  const [busy, setBusy] = useState(false);
  const [form] = Form.useForm<{ agent_cli_default: 'claude' | 'codex' }>();

  const submit = async (v: { agent_cli_default: 'claude' | 'codex' }) => {
    setBusy(true);
    try {
      const body: UpdateGlobalSettingsReq = { agent_cli_default: v.agent_cli_default };
      await updateGlobalSettings(body);
      message.success('全局默认已保存');
      bump();
    } catch (e) {
      message.error(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  const scripted = gs.engine_mode_default === 'scripted';

  return (
    <Card title="全局默认 / 引擎" style={{ marginBottom: 16 }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 4 }}>
        引擎模式(默认)
        <Tag color={scripted ? 'warning' : 'success'} icon={!scripted ? <CheckCircleFilled /> : undefined}>
          {scripted ? 'scripted(手工 DB/env)' : 'live'}
        </Tag>
      </div>
      <Paragraph type="secondary" style={{ fontSize: 12.5, lineHeight: 1.9, marginBottom: 12 }}>
        engine_mode 只读:<Text code>live</Text> 为生产引擎(执行委派/判读走真实模型);
        <Text code>scripted</Text>(离线确定性)仅保留为 CLI/env 测试线,不在本页设置(决策③:Web 仅 live)。
      </Paragraph>

      <Form form={form} layout="inline" onFinish={submit} disabled={busy} initialValues={{ agent_cli_default: gs.agent_cli_default }}>
        <Form.Item name="agent_cli_default" label="委派 agent CLI(默认)">
          <Select style={{ width: 240 }} options={AGENT_OPTIONS} />
        </Form.Item>
        <Button type="primary" htmlType="submit" loading={busy}>
          保存
        </Button>
      </Form>
      <Paragraph type="secondary" style={{ fontSize: 12.5, marginTop: 8, marginBottom: 0 }}>
        影响 <Text code>os task</Text> 执行委派默认工具族;公司未覆盖时用此默认。
      </Paragraph>
    </Card>
  );
}

// ---- 卡 3:Server 参数(DB 权威;重启 os server 生效)----

interface ServerVals {
  http_port: number | null;
  poll_min: number | null;
  queue_interval_sec: number | null;
  queue_work: boolean;
  digest_time: string;
  schedule_poll_sec: number | null;
}

function ServerParamsCard({ gs }: { gs: GlobalSettings }) {
  const { message } = App.useApp();
  const { bump } = useApp();
  const [busy, setBusy] = useState(false);
  const [form] = Form.useForm<ServerVals>();

  const submit = async (v: ServerVals) => {
    setBusy(true);
    try {
      const body: UpdateGlobalSettingsReq = {
        http_port: Number(v.http_port),
        poll_min: Number(v.poll_min),
        queue_interval_sec: Number(v.queue_interval_sec),
        queue_work: v.queue_work,
        digest_time: (v.digest_time ?? '').trim(),
        schedule_poll_sec: v.schedule_poll_sec == null ? 0 : Math.max(0, Number(v.schedule_poll_sec)),
      };
      await updateGlobalSettings(body);
      message.success('Server 参数已保存 —— 重启 os server 生效');
      bump();
    } catch (e) {
      message.error(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Card title="Server 参数(重启 os server 生效)" style={{ marginBottom: 16 }}>
      <Paragraph type="secondary" style={{ fontSize: 12.5, lineHeight: 1.9 }}>
        存 <Text code>app_setting</Text>,是 <Text code>os server</Text> 零参数启动的权威源
        (boot 读一次,非热更;改后重启生效)。原 <Text code>--port/--poll/--queue-*/--digest</Text> flags
        仅作显式覆盖并标记 deprecated。
      </Paragraph>
      <Form
        form={form}
        layout="vertical"
        onFinish={submit}
        disabled={busy}
        style={{ maxWidth: 360 }}
        initialValues={{
          http_port: gs.http_port,
          poll_min: gs.poll_min,
          queue_interval_sec: gs.queue_interval_sec,
          queue_work: gs.queue_work,
          digest_time: gs.digest_time || '',
          schedule_poll_sec: gs.schedule_poll_sec,
        }}
      >
        <Form.Item
          name="http_port"
          label="HTTP 端口"
          rules={[{ required: true, message: '必填' }]}
          extra="1..65535"
        >
          <InputNumber min={1} max={65535} style={{ width: '100%' }} />
        </Form.Item>
        <Form.Item
          name="poll_min"
          label="GitHub 轮询间隔(分钟)"
          rules={[{ required: true, message: '必填' }]}
          extra="≥ 1"
        >
          <InputNumber min={1} style={{ width: '100%' }} />
        </Form.Item>
        <Form.Item
          name="queue_interval_sec"
          label="队列认领间隔(秒,queue_work 开时用)"
          rules={[{ required: true, message: '必填' }]}
          extra="≥ 1"
        >
          <InputNumber min={1} style={{ width: '100%' }} />
        </Form.Item>
        <Form.Item name="queue_work" label="server 自消费任务队列(queue work)" valuePropName="checked">
          <Switch />
        </Form.Item>
        <Form.Item name="digest_time" label="每日摘要时刻" extra={'留空/"off" = 关;否则 HH:MM'}>
          <Input placeholder="09:00" allowClear />
        </Form.Item>
        <Form.Item
          name="schedule_poll_sec"
          label="流水线定时调度轮询间隔(秒,0 = 关)"
          rules={[{ required: true, message: '必填' }]}
          extra={'调度总开关:>0 时到点 cron 自动拉单(system:schedule);0 = 关。与 queue_work 正交:只开调度 = 拉单不自动执行,执行仍需后台队列。'}
        >
          <InputNumber min={0} style={{ width: '100%' }} placeholder="0 = 关" />
        </Form.Item>
        <Button type="primary" htmlType="submit" loading={busy}>
          保存
        </Button>
      </Form>
    </Card>
  );
}

// ---- 卡 4:公司覆盖(作用于当前公司;null 维度 = 继承 global)----

interface CompanyVals {
  agent_cli: 'claude' | 'codex' | string;
  issue_source: string;
  issue_fixture_path: string;
}

function CompanyOverrideCard({ companyId, cs }: { companyId: string; cs: CompanySettings }) {
  const { message, modal } = App.useApp();
  const { bump } = useApp();
  const [busy, setBusy] = useState(false);
  const [form] = Form.useForm<CompanyVals>();
  const source = Form.useWatch('issue_source', form);

  const submit = async (v: CompanyVals) => {
    if (v.issue_source === 'fixture' && !String(v.issue_fixture_path ?? '').trim()) {
      message.error('issue_source=fixture 需填 issue_fixture_path');
      return;
    }
    setBusy(true);
    try {
      const body: UpdateCompanySettingsReq = {
        agent_cli: v.agent_cli === INHERIT ? '' : v.agent_cli,
        issue_source: v.issue_source === INHERIT ? '' : v.issue_source,
        issue_fixture_path: String(v.issue_fixture_path ?? '').trim(),
      };
      await updateCompanySettings(companyId, body);
      message.success('公司覆盖已保存(空维度回退继承)');
      bump();
    } catch (e) {
      message.error(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  const reset = () => {
    modal.confirm({
      title: '重置该公司设置?',
      icon: <ExclamationCircleFilled />,
      content: '删除该公司覆盖行,全部维度回退继承全局默认(引擎 live / agent claude / issue github)。不可撤销。',
      okText: '重置',
      okButtonProps: { danger: true },
      cancelText: '取消',
      onOk: async () => {
        await resetCompanySettings(companyId);
        message.success('公司设置已重置(全继承)');
        bump();
      },
    });
  };

  const scriptedOverride = cs.engine_mode === 'scripted';

  return (
    <Card title="公司覆盖(作用于当前公司)" style={{ marginBottom: 16 }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 4 }}>
        引擎模式
        <Tag color={scriptedOverride ? 'warning' : 'success'}>
          {scriptedOverride ? 'scripted(仅 CLI/env 可设)' : 'live'}
        </Tag>
      </div>
      <Paragraph type="secondary" style={{ fontSize: 12.5, lineHeight: 1.9, marginBottom: 12 }}>
        引擎只读:Web 仅 live(scripted 离线线不经此页);公司未覆盖(继承)时即全局默认 live。
      </Paragraph>
      <Form
        form={form}
        layout="vertical"
        onFinish={submit}
        disabled={busy}
        style={{ maxWidth: 400 }}
        initialValues={{
          agent_cli: cs.agent_cli ?? INHERIT,
          issue_source: cs.issue_source ?? INHERIT,
          issue_fixture_path: cs.issue_fixture_path ?? '',
        }}
      >
        <Form.Item name="agent_cli" label="委派 agent CLI" extra="清空(=继承全局)保存即回退继承">
          <Select options={[{ value: INHERIT, label: `继承全局(${cs.agent_cli ?? 'claude'})` }, ...AGENT_OPTIONS]} />
        </Form.Item>
        <Form.Item name="issue_source" label="通道 B issue 源" extra="github 需配 github_token 机密;fixture 需路径">
          <Select options={ISSUE_OPTIONS} />
        </Form.Item>
        <Form.Item
          name="issue_fixture_path"
          label="issue fixture 路径"
          style={source === 'fixture' ? {} : { display: 'none' }}
        >
          <Input placeholder="fixture JSON 绝对路径" />
        </Form.Item>
        <Space>
          <Button type="primary" htmlType="submit" loading={busy}>
            保存
          </Button>
          <Button danger onClick={reset}>
            重置公司设置(全继承)
          </Button>
        </Space>
      </Form>
    </Card>
  );
}

// ---- 卡 5:公司机密(当前公司;白名单三枚,值明文只进设置框,list 只出掩码)----

function CompanySecretsCard({ companyId, secrets }: { companyId: string; secrets: SecretMeta[] }) {
  const { message, modal } = App.useApp();
  const { bump } = useApp();
  const [editing, setEditing] = useState<SecretMeta | null>(null);
  const [value, setValue] = useState('');
  const [busy, setBusy] = useState(false);

  const save = async () => {
    if (!editing) return;
    const v = value.trim();
    if (!v) {
      message.error('机密值不能为空');
      return;
    }
    setBusy(true);
    try {
      await setSecret(companyId, editing.id, v);
      message.success(`secret ${editing.id} 已保存(服务端加密存储,明文不落库)`);
      setEditing(null);
      setValue('');
      bump();
    } catch (e) {
      message.error(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  const remove = (s: SecretMeta) => {
    modal.confirm({
      title: `移除 secret ${s.id}?`,
      icon: <ExclamationCircleFilled />,
      content: '删除该公司该机密(如 feishu_webhook 被删,该公司不再发通知/日报)。',
      okText: '移除',
      okButtonProps: { danger: true },
      cancelText: '取消',
      onOk: async () => {
        await deleteSecret(companyId, s.id);
        message.success(`secret ${s.id} 已移除`);
        bump();
      },
    });
  };

  return (
    <Card title="公司机密(当前公司)" style={{ marginBottom: 16 }}>
      <Paragraph type="secondary" style={{ fontSize: 12.5, lineHeight: 1.9 }}>
        白名单:<Text code>github_token</Text>(issue 源)、<Text code>feishu_webhook</Text> /{' '}
        <Text code>feishu_secret</Text>(通知/日报)。主密钥加密落库;此页永不回显明文,仅标已设/更新时刻。
      </Paragraph>
      {secrets.map((s) => {
        const meta = secretMetaOf(s.id);
        return (
          <div
            key={s.id}
            style={{
              display: 'flex',
              justifyContent: 'space-between',
              alignItems: 'center',
              gap: 12,
              padding: '10px 2px',
              borderBottom: '1px solid rgba(128,128,128,.15)',
            }}
          >
            <div style={{ minWidth: 0 }}>
              <Space size={8}>
                <Text strong code>{meta.label}</Text>
                {s.set ? (
                  <Tag color="success" icon={<CheckCircleFilled />}>
                    已设置
                  </Tag>
                ) : (
                  <Tag>未设置</Tag>
                )}
              </Space>
              <div style={{ fontSize: 12.5, color: 'var(--text-dim)', lineHeight: 1.6 }}>{meta.desc}</div>
              {s.set && (
                <div style={{ fontSize: 11.5, color: 'var(--text-dim)' }}>更新于 {fmtT(s.updated_at)}</div>
              )}
            </div>
            <Space wrap>
              <Button
                size="small"
                onClick={() => {
                  setEditing(s);
                  setValue('');
                }}
              >
                {s.set ? '更换' : '设置'}
              </Button>
              {s.set && (
                <Button size="small" danger onClick={() => remove(s)}>
                  移除
                </Button>
              )}
            </Space>
          </div>
        );
      })}

      <Modal
        open={!!editing}
        title={editing ? `设置 ${secretMetaOf(editing.id).label}` : ''}
        onOk={save}
        confirmLoading={busy}
        onCancel={() => {
          setEditing(null);
          setValue('');
        }}
        okText="保存"
      >
        {editing && (
          <>
            <Paragraph type="secondary" style={{ fontSize: 12.5, lineHeight: 1.8 }}>
              {secretMetaOf(editing.id).desc}
            </Paragraph>
            <Input.Password
              value={value}
              onChange={(e) => setValue(e.target.value)}
              placeholder="输入机密值(主密钥加密存库)"
              autoComplete="off"
            />
          </>
        )}
      </Modal>
    </Card>
  );
}

// ---- 卡 6:更换主密钥(danger;re-key 后新 key 仅此一次展示)----

interface RotateResult {
  masterKey: string;
  ep: number;
  sec: number;
}

function RotateMasterKeyCard({ initialized }: { initialized: boolean }) {
  const { message, modal } = App.useApp();
  const [result, setResult] = useState<RotateResult | null>(null);
  const [copied, setCopied] = useState(false);

  const copyKey = async () => {
    if (!result) return;
    try {
      await navigator.clipboard.writeText(result.masterKey);
      setCopied(true);
      setTimeout(() => setCopied(false), 1600);
    } catch {
      message.warning('复制失败,请手动全选复制');
    }
  };

  const rotate = () => {
    modal.confirm({
      title: '更换主密钥?',
      icon: <ExclamationCircleFilled />,
      content:
        '将对全部端点令牌与公司机密做 re-key(旧 key 立即失效),并覆盖 <db>.key(0600)。' +
        '服务端无取回通道 —— 成功后新主密钥仅在本页展示一次,请先备好保存位置。',
      okText: '确认更换',
      okButtonProps: { danger: true },
      cancelText: '取消',
      onOk: async () => {
        try {
          const r = await rotateMasterKey();
          setResult({ masterKey: r.master_key, ep: r.endpoints_rekeyed, sec: r.secrets_rekeyed });
        } catch (e) {
          message.error(e instanceof Error ? e.message : String(e));
        }
      },
    });
  };

  return (
    <Card
      title="更换主密钥"
      style={{ marginBottom: 16, borderColor: 'rgba(255,77,79,.45)' }}
    >
      <Paragraph type="secondary" style={{ fontSize: 12.5, lineHeight: 1.9 }}>
        定期轮换主密钥:re-key 全部端点令牌与公司机密到新 key,覆盖落盘文件 <Text code>&lt;db&gt;.key</Text>(0600)。
        操作期间进程内自动切换新 key,无需重启;任何一步失败即整体回滚零写(校验错误不破坏存量)。
        <b> 谨慎:</b>新 key 只返回一次,请立即保存(建议记入密码管理器)。
      </Paragraph>
      <Button danger disabled={!initialized} onClick={rotate}>
        更换主密钥…
      </Button>
      {!initialized && (
        <div style={{ marginTop: 8 }}>
          <Text type="secondary" style={{ fontSize: 12.5 }}>
            尚未完成 /setup(无主密钥),先完成首启设置。
          </Text>
        </div>
      )}

      <Modal
        open={!!result}
        title="主密钥已更换"
        footer={null}
        closable={false}
      >
        {result && (
          <>
            <Alert
              type="warning"
              showIcon
              message="新主密钥只显示这一次"
              description="服务端无再次取回通道;请复制保存后关闭。旧 key 已失效(存量端点令牌与机密均已 re-key)。"
              style={{ marginBottom: 12 }}
            />
            <div
              className="mono"
              style={{
                background: 'var(--code-bg, #0f1621)',
                color: '#e8eef7',
                padding: '12px 14px',
                borderRadius: 8,
                fontSize: 12.5,
                wordBreak: 'break-all',
                userSelect: 'all',
                marginBottom: 12,
              }}
            >
              {result.masterKey}
            </div>
            <Paragraph type="secondary" style={{ fontSize: 12.5, lineHeight: 1.8 }}>
              已 re-key:端点令牌 {result.ep} · 公司机密 {result.sec};新主密钥已覆盖 <Text code>&lt;db&gt;.key</Text>(0600)。
            </Paragraph>
            <Button block onClick={copyKey} style={{ marginBottom: 8 }}>
              {copied ? '已复制 ✓' : '复制主密钥'}
            </Button>
            <Button type="primary" block onClick={() => setResult(null)}>
              我已保存
            </Button>
          </>
        )}
      </Modal>
    </Card>
  );
}
