// 轻量展示原语:页头 / 数值卡 / 风险文本 / KV 描述 / 空态。
import type { ReactNode } from 'react';
import { Flex, Typography } from 'antd';
import { riskColor, riskLabel } from '../utils/dicts';
import type { Risk } from '../api/types';

/** 页头:标题 + mono 副标题(接口血缘)+ 右侧动作 */
export function PageHead({ title, sub, actions }: { title: string; sub?: string; actions?: ReactNode }) {
  return (
    <Flex align="flex-end" gap={12} style={{ margin: '2px 0 16px', flexWrap: 'wrap' }}>
      <Typography.Title level={4} style={{ margin: 0 }}>
        {title}
      </Typography.Title>
      {sub ? (
        <span className="mono dim" style={{ fontSize: 11.5, paddingBottom: 2 }}>
          {sub}
        </span>
      ) : null}
      <div className="grow" />
      {actions ? (
        <Flex gap={8} style={{ paddingBottom: 2 }}>
          {actions}
        </Flex>
      ) : null}
    </Flex>
  );
}

/** 数值卡(仿原型 .tile):k 小标签 + 大数值 + x 脚注;tone 高亮描边 */
export function StatTile({
  k,
  v,
  x,
  tone,
}: {
  k: string;
  v: ReactNode;
  x?: ReactNode;
  tone?: 'ok' | 'warn' | 'crit';
}) {
  const border = tone ? { borderColor: tone === 'crit' ? 'var(--crit)' : 'var(--warn)' } : undefined;
  const strip = tone === 'crit' ? '3px solid var(--crit)' : tone === 'warn' ? '3px solid var(--warn)' : undefined;
  return (
    <div
      style={{
        background: 'var(--surface)',
        border: '1px solid var(--line)',
        borderRadius: 8,
        padding: '12px 15px',
        boxShadow: '0 1px 2px rgba(28,36,48,.05)',
        position: 'relative',
        overflow: 'hidden',
        ...border,
      }}
    >
      {strip ? (
        <div style={{ position: 'absolute', left: 0, top: 0, bottom: 0, width: 3, background: strip }} />
      ) : null}
      <div style={{ fontSize: 12, color: 'var(--text-2)' }}>{k}</div>
      <div className="mono" style={{ fontSize: 26, lineHeight: 1.2, fontWeight: 700, marginTop: 2 }}>
        {v}
      </div>
      {x ? (
        <div style={{ fontSize: 11.5, color: 'var(--text-3)', marginTop: 2 }}>{x}</div>
      ) : null}
    </div>
  );
}

/** 风险文本:低灰 / 中琥珀 / 高红加粗 */
export function RiskText({ risk }: { risk: string | Risk }) {
  return (
    <span style={{ color: riskColor(risk), fontWeight: risk === 'high' ? 700 : 400 }}>
      {riskLabel(risk)}险
    </span>
  );
}

/** KV 描述网格(dt mono 灰 / dd 可断行) */
export function KV({ rows }: { rows: [string, ReactNode][] }) {
  return (
    <dl className="dl-kv">
      {rows.map(([k, v]) => (
        <div key={k} style={{ display: 'contents' }}>
          <dt>{k}</dt>
          <dd>{v ?? '—'}</dd>
        </div>
      ))}
    </dl>
  );
}

/** 空态(表格/列表) */
export function EmptyState({ text }: { text?: string }) {
  return (
    <div style={{ padding: '26px 0', textAlign: 'center', color: 'var(--text-3)', fontSize: 13 }}>
      {text ?? '暂无数据'}
    </div>
  );
}
