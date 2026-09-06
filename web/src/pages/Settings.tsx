// 设置(Phase 9.2):「控制台访问」卡 —— 初始化状态 + 令牌轮换(PUT /settings/console-token)。
// 轮换后旧令牌立即失效;新令牌回写 localStorage(当前会话继续可用)。
import { useState } from 'react';
import { Alert, Button, Card, Form, Input, Tag, Typography, message } from 'antd';
import { CheckCircleFilled } from '@ant-design/icons';
import { setupStatus, rotateConsoleToken } from '../api/endpoints';
import { setToken } from '../api/client';
import { useData } from '../hooks/useApi';
import { PageHead } from '../components/common';

const { Paragraph, Text } = Typography;

export default function Settings() {
  const status = useData(setupStatus, { intervalMs: 0 });
  const [busy, setBusy] = useState(false);

  const initialized = status.data?.initialized ?? false;

  const rotate = async (v: { new_token: string }) => {
    setBusy(true);
    try {
      await rotateConsoleToken({ new_token: v.new_token });
      setToken(v.new_token); // 旧令牌已失效,当前会话无缝切到新令牌
      message.success('控制台令牌已轮换(旧令牌立即失效)');
    } catch (e) {
      message.error(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div style={{ maxWidth: 640 }}>
      <PageHead title="设置" sub="PUT /api/v1/settings/console-token · 控制台访问治理" />

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

        <Form layout="vertical" onFinish={rotate} disabled={busy || !initialized} style={{ marginTop: 4 }}>
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

      <Card title="主密钥(信息)">
        <Paragraph type="secondary" style={{ fontSize: 12.5, lineHeight: 1.9 }}>
          主密钥在首启 <Text code>/setup</Text> 时生成并落盘数据库旁文件 <Text code>&lt;db&gt;.key</Text>(0600);
          <Text code>os</Text> CLI / <Text code>os server</Text> 启动时自动读取注入,用于解密端点令牌等机密。
          服务端不提供任何再次读取通道——遗失即无法解密存量机密。
        </Paragraph>
      </Card>
    </div>
  );
}
