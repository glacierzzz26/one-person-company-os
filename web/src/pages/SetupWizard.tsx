// 首启向导(Phase 9.2 /setup):主密钥 Web 生成-显示一次-落盘 0600 + 可选旧 OS_ENDPOINT_KEY re-key
// + console 令牌哈希化。POST /api/v1/setup → {master_key} 仅此一次返回;成功后令牌存 localStorage。
import { useState } from 'react';
import { Alert, Button, Card, Form, Input, Steps, Typography, message } from 'antd';
import { runSetup } from '../api/endpoints';
import type { SetupReq } from '../api/types';
import { setToken } from '../api/client';

const { Paragraph, Text } = Typography;

type Step2State = { token: string; masterKey: string };

export default function SetupWizard({ onDone }: { onDone: () => void }) {
  const [step, setStep] = useState(0);
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState<Step2State | null>(null);
  const [copied, setCopied] = useState(false);

  const submit = async (v: SetupReq) => {
    setBusy(true);
    try {
      const token = v.console_token.trim(); // 后端存的是 trim 后哈希,存储/发送须同值,否则全程 401
      if (token.length < 8) {
        message.error('控制台令牌至少 8 个字符(去除首尾空格后)');
        setBusy(false);
        return;
      }
      const r = await runSetup({ console_token: token, old_endpoint_key: v.old_endpoint_key?.trim() || undefined });
      setToken(token); // 后续壳内请求带 Bearer,避免 401
      setResult({ token, masterKey: r.master_key });
      setStep(2);
    } catch (e) {
      message.error(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

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

  return (
    <div
      style={{
        minHeight: '100vh',
        display: 'grid',
        placeItems: 'start center',
        background: 'var(--bg)',
        padding: '8vh 24px 48px',
      }}
    >
      <Card style={{ width: 560, boxShadow: '0 6px 24px rgba(28,36,48,.1)' }} title="首次启动设置 /setup">
        <Steps
          current={step}
          size="small"
          items={[{ title: '了解' }, { title: '配置' }, { title: '保存主密钥' }]}
          style={{ marginBottom: 20 }}
        />

        {step === 0 && (
          <>
            <Alert
              type="info"
              showIcon
              message="单算子本地安全姿态的第一步"
              description="后端会生成一个 32 字节主密钥并写入数据库旁文件(<db>.key,权限 0600)。"
              style={{ marginBottom: 16 }}
            />
            <Paragraph type="secondary" style={{ fontSize: 13, lineHeight: 1.9 }}>
              主密钥用于解密模型端点令牌等机密;控制台访问令牌(你即将设置)只以 sha256 哈希存库,
              之后 /api/v1 要求 <Text code>Authorization: Bearer &lt;令牌&gt;</Text>。
              本页完成后会显示主密钥<b>仅一次</b>——请立即妥善保存(建议记入密码管理器)。
            </Paragraph>
            <Paragraph type="secondary" style={{ fontSize: 13, lineHeight: 1.9 }}>
              若旧版曾用 <Text code>OS_ENDPOINT_KEY</Text> 加密过端点令牌,下一步可填入它以在本次首启
              一并 re-key(否则那些存量令牌运行时解不开);纯新环境可跳过。
            </Paragraph>
            <Button type="primary" block onClick={() => setStep(1)}>
              开始配置 →
            </Button>
          </>
        )}

        {step === 1 && (
          <Form layout="vertical" onFinish={submit} disabled={busy} style={{ marginTop: 4 }}>
            <Form.Item
              name="console_token"
              label="控制台访问令牌 *"
              rules={[
                { required: true, message: '必填' },
                { min: 8, message: '至少 8 个字符' },
              ]}
              extra="存入 localStorage,作为所有 /api/v1 请求的 Bearer。"
            >
              <Input.Password placeholder="例:os-9b2f…(≥8 字符)" autoComplete="new-password" />
            </Form.Item>
            <Form.Item name="old_endpoint_key" label="旧 OS_ENDPOINT_KEY(可选)">
              <Input.Password
                placeholder="64 位 hex;曾在旧版用它加密端点令牌时才填"
                autoComplete="off"
              />
            </Form.Item>
            <Button type="primary" htmlType="submit" block loading={busy}>
              生成主密钥并完成
            </Button>
            <div style={{ textAlign: 'center', marginTop: 10 }}>
              <Button type="link" size="small" disabled={busy} onClick={() => setStep(0)}>
                ← 返回
              </Button>
            </div>
          </Form>
        )}

        {step === 2 && result && (
          <>
            <Alert
              type="warning"
              showIcon
              message="主密钥只显示这一次"
              description="服务端不提供任何通道再次读取。请复制保存后点击进入控制台。"
              style={{ marginBottom: 16 }}
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
            <Button block onClick={copyKey} style={{ marginBottom: 8 }}>
              {copied ? '已复制 ✓' : '复制主密钥'}
            </Button>
            <Paragraph type="secondary" style={{ fontSize: 12.5, marginTop: 8, lineHeight: 1.8 }}>
              主密钥已落盘 <Text code>&lt;db&gt;.key</Text>(0600);之后 <Text code>os</Text> CLI 启动会自动读取注入,
              无需在本页之外再次输入。
            </Paragraph>
            <Button type="primary" block onClick={onDone}>
              我已保存 —— 进入控制台
            </Button>
          </>
        )}
      </Card>
      <div className="mono dim" style={{ fontSize: 11, marginTop: 14 }}>
        POST /api/v1/setup · 主密钥 Web 生成 · console token sha256 哈希落库
      </div>
    </div>
  );
}
