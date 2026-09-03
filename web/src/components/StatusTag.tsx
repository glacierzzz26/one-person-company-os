// 状态胶囊:预设色类(pill-ok/crit/warn/accent/muted)映射 dicts 语义。带圆点(仿原型)。
import type { Preset } from '../utils/dicts';

const CLASS: Record<Preset, string> = {
  default: 'pill-muted',
  processing: 'pill-accent',
  success: 'pill-ok',
  warning: 'pill-warn',
  error: 'pill-crit',
};

export default function StatusTag({ preset, label }: { preset: Preset; label: string }) {
  return (
    <span className={`pill ${CLASS[preset] ?? 'pill-muted'}`}>
      <i />
      {label}
    </span>
  );
}
