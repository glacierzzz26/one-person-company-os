// 时间工具:后端 unix 秒(int64)→ 展示文案。空/0/未决 → '—'。
const pad = (n: number) => String(n).padStart(2, '0');

export const short = (id: string | null | undefined): string =>
  !id ? '' : id.length > 8 ? id.slice(0, 8) : id;

export function fmtT(ts?: number | null): string {
  if (!ts) return '—';
  const d = new Date(ts * 1000);
  return `${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

export function fmtFull(ts?: number | null): string {
  if (!ts) return '—';
  const d = new Date(ts * 1000);
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

export function ago(ts?: number | null): string {
  if (!ts) return '—';
  const s = Math.max(0, Date.now() / 1000 - ts);
  if (s < 90) return '刚刚';
  const MIN = 60,
    HR = 3600,
    DAY = 86400;
  if (s < HR) return `${Math.round(s / MIN)} 分钟前`;
  if (s < DAY) return `${Math.round(s / HR)} 小时前`;
  return `${Math.round(s / DAY)} 天前`;
}
