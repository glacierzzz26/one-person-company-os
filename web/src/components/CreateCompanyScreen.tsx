// 空库首启:创建第一家公司。POST /companies → actor=human:console 审计。
import { useState } from 'react';
import { Alert, Button, Card, Form, Input, message } from 'antd';
import { createCompany } from '../api/endpoints';
import { useApp } from '../store/AppContext';

export default function CreateCompanyScreen() {
  const { reloadCompanies } = useApp();
  const [busy, setBusy] = useState(false);

  const submit = async (v: { name: string; vision?: string }) => {
    setBusy(true);
    try {
      const c = await createCompany({ name: v.name, vision: v.vision ?? '' });
      message.success(`已创建「${c.name}」(${c.id.slice(0, 8)})`);
      reloadCompanies(); // boot 层观察到非空列表 → 进入主界面
    } catch (e) {
      message.error(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div
      style={{
        minHeight: '100vh',
        display: 'grid',
        placeItems: 'center',
        background: 'var(--bg)',
        padding: 24,
      }}
    >
      <Card style={{ width: 440, boxShadow: '0 6px 24px rgba(28,36,48,.1)' }} title="创建你的公司">
        <Alert
          style={{ marginBottom: 16 }}
          type="info"
          showIcon
          message="数据库为空 —— 先登记第一家组织,随后进入运营控制台。"
        />
        <Form layout="vertical" onFinish={submit} disabled={busy}>
          <Form.Item name="name" label="公司名称 *" rules={[{ required: true, message: '必填' }]}>
            <Input placeholder="例:ACME Labs" maxLength={80} />
          </Form.Item>
          <Form.Item name="vision" label="愿景 / 一句话定位">
            <Input.TextArea placeholder="例:把重复劳动变成流水线" rows={2} maxLength={300} showCount />
          </Form.Item>
          <Button type="primary" htmlType="submit" block loading={busy}>
            创建并进入
          </Button>
        </Form>
        <div className="dim" style={{ fontSize: 12, marginTop: 12, lineHeight: 1.8 }}>
          POST /api/v1/companies · 审计 actor <span className="mono">human:console</span>
        </div>
      </Card>
    </div>
  );
}
