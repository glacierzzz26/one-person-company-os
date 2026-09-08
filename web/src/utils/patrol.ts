// 巡检 run 结果(result 文本)解析 —— 与后端 runPatrol 写入格式对齐(Phase 10.2)。
// 完成态 result = "patrol:patrol/<taskID>.md|ok=true|severity=low|action=none|<summary 首行>"。
// ok/severity/action 三个键顺序固定;summary 首行可能含 '|',故遇未知键即视为 summary 起始。

export interface PatrolVerdictInfo {
  ok: boolean;
  severity: string; // low | medium | high
  action: string; // none | fix
  summary: string;
  path: string; // 相对项目根的报告路径 patrol/<taskID>.md
}

export const PATROL_RESULT_TAG = 'patrol:';

/** parsePatrolResult 解析巡检 run 的 result;非 "patrol:" 前缀(非巡检产物)→ null。 */
export function parsePatrolResult(result: string): PatrolVerdictInfo | null {
  if (!result.startsWith(PATROL_RESULT_TAG)) {
    return null;
  }
  const rest = result.slice(PATROL_RESULT_TAG.length);
  const parts = rest.split('|');
  const path = parts[0] ?? '';
  const kv: Record<string, string> = {};
  let summaryStart = parts.length;
  for (let i = 1; i < parts.length; i++) {
    const seg = parts[i];
    const eq = seg.indexOf('=');
    if (eq <= 0) {
      summaryStart = i; // 非 key=value → summary 从此开始
      break;
    }
    const key = seg.slice(0, eq);
    if (key === 'ok' || key === 'severity' || key === 'action') {
      kv[key] = seg.slice(eq + 1);
      continue;
    }
    summaryStart = i; // 未知键(如 summary 值里含 '|')→ 视作 summary
    break;
  }
  return {
    ok: kv.ok === 'true',
    severity: kv.severity || 'low',
    action: kv.action || 'none',
    summary: parts.slice(summaryStart).join('|'),
    path,
  };
}
