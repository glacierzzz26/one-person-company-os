// 审批三态决策:放行(approve→回队)/ 驳回(reject→failed)/ 要修改(changes→failed)。
// POST /approvals/{id}/decision → 自动落 decision + human:console 审计;成功后 onDone。
import { useEffect, useState } from 'react';
import { App, Button, Input, Modal, Radio, Space, Typography } from 'antd';
import { decideApproval } from '../api/endpoints';
import type { Approval } from '../api/types';
import { RiskText } from './common';
import { useApp } from '../store/AppContext';

export interface DecideResult {
  decision: 'approve' | 'reject' | 'changes';
}

export default function DecideModal({
  approval,
  onClose,
  onDone,
}: {
  approval: Approval | null;
  onClose: () => void;
  onDone: (decision: string) => void; // 页面据此 refresh + bump
}) {
  const { message } = App.useApp();
  const { bump } = useApp();
  const [decision, setDecision] = useState<'approve' | 'reject' | 'changes'>('approve');
  const [note, setNote] = useState('');
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (approval) {
      setDecision('approve');
      setNote('');
      setBusy(false);
    }
  }, [approval]);

  if (!approval) return null;

  const submit = async () => {
    setBusy(true);
    try {
      await decideApproval(approval.id, { decision, note: note.trim() });
      message.success(
        decision === 'approve'
          ? `已放行 → 任务回队续跑`
          : decision === 'reject'
            ? `已驳回 → 任务置 failed`
            : `已打回 → 任务置 failed(重开请再建单)`,
      );
      bump();
      onDone(decision);
    } catch (e) {
      message.error(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  const options: { value: 'approve' | 'reject' | 'changes'; label: string; hint: string }[] = [
    { value: 'approve', label: '放行', hint: '任务回队续跑' },
    { value: 'reject', label: '驳回', hint: '任务置 failed' },
    { value: 'changes', label: '要修改', hint: '任务置 failed · 重开建单' },
  ];

  return (
    <Modal
      title={`审批决策 · ${approval.reason}`}
      open
      onCancel={onClose}
      width={520}
      footer={[
        <Button key="cancel" onClick={onClose}>
          取消
        </Button>,
        <Button key="ok" type="primary" loading={busy} onClick={submit}>
          提交决策
        </Button>,
      ]}
    >
      <Typography.Paragraph type="secondary" style={{ marginTop: 4 }}>
        风险 <RiskText risk={approval.risk} /> · 请求方{' '}
        <span className="mono" style={{ fontSize: 12 }}>
          {approval.requested_by}
        </span>
      </Typography.Paragraph>
      <Radio.Group
        value={decision}
        onChange={(e) => setDecision(e.target.value)}
        style={{ display: 'flex', width: '100%', gap: 8 }}
      >
        <Space direction="vertical" style={{ width: '100%' }}>
          <Space style={{ width: '100%' }} wrap>
            {options.map((o) => (
              <Radio.Button key={o.value} value={o.value} style={{ flex: 1, textAlign: 'center' }}>
                {o.label}
                <span style={{ display: 'block', fontSize: 11, opacity: 0.75 }}>{o.hint}</span>
              </Radio.Button>
            ))}
          </Space>
        </Space>
      </Radio.Group>
      <Input
        style={{ marginTop: 14 }}
        placeholder="备注(随 decision 自动落档,写入 audit detail)"
        value={note}
        onChange={(e) => setNote(e.target.value)}
      />
      <div className="dim mono" style={{ fontSize: 11.5, marginTop: 10 }}>
        POST /api/v1/approvals/{approval.id.slice(0, 8)}/decision · actor=human:console · 与 CLI os approval 同语义
      </div>
    </Modal>
  );
}
