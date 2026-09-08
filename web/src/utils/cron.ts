// 流水线 schedule(cron 五段)前端校验 —— 与后端 internal/schedule 子集同语义(Phase 10.2)。
// `分 时 日 月 周` 五段数字;空串 / "off" = 不调度(合法)。每段:`*` | `n` | `a-b` | `a-b/n` | `*/n`
// | 逗号并集。值域:分0-59 时0-23 日1-31 月1-12 周0-6(0=周日;7 亦按周日)。名字(MON/JAN)、`?`、L/W/#、秒/年 → 非法。
// 前端只做作者面即时校验(免提交才 400);权威仍以服务端校验为准(create/update 各自再判一次)。

export interface CronCheck {
  ok: boolean;
  value: string; // 归一化后待存原文(trimmed;空/off → "" 语义等价不调度)
  message?: string; // ok=false 时的指引
}

// 每段合法值域(minute hour dom month dow)。
const CRON_FIELD_RANGES: ReadonlyArray<readonly [number, number]> = [
  [0, 59],
  [0, 23],
  [1, 31],
  [1, 12],
  [0, 7], // dow raw 0-7(7 = 周日别名;落位时 %7)
];
const CRON_FIELD_NAMES = ['minute', 'hour', 'day-of-month', 'month', 'day-of-week'];

/** validateCron 校验流水线 schedule;空/"off"(大小写)→ ok(不调度);非法 5 段/越界/名字 → ok:false + 指引。 */
export function validateCron(raw: string): CronCheck {
  const s = raw.trim();
  if (s === '' || s.toLowerCase() === 'off') {
    return { ok: true, value: s };
  }
  const parts = s.split(/\s+/);
  if (parts.length !== 5) {
    return {
      ok: false,
      value: s,
      message: `cron 须 5 段(分 时 日 月 周),当前 ${parts.length} 段`,
    };
  }
  for (let i = 0; i < 5; i++) {
    const err = parseCronField(CRON_FIELD_NAMES[i], parts[i], CRON_FIELD_RANGES[i]);
    if (err) {
      return { ok: false, value: s, message: err };
    }
  }
  return { ok: true, value: s };
}

// parseCronField 单段校验;返回错误文本或 "" = 合法。
function parseCronField(name: string, raw: string, range: readonly [number, number]): string {
  const [lo, hi] = range;
  const items = raw.split(',');
  if (items.some((it) => it.trim() === '')) {
    return `${name}: 并集里有空项("${raw}")`;
  }
  for (const item of items) {
    const it = item.trim();
    const err = parseCronUnit(name, it, lo, hi);
    if (err) {
      return err;
    }
  }
  return '';
}

function parseCronUnit(name: string, p: string, lo: number, hi: number): string {
  // 步进后缀 "/n"(值合法即可,实际步进计算以服务端为准)。
  let base = p;
  const slash = p.indexOf('/');
  if (slash >= 0) {
    const sv = p.slice(slash + 1);
    base = p.slice(0, slash);
    if (sv === '' || !/^[0-9]+$/.test(sv) || Number(sv) < 1) {
      return `${name}: 步进 "${sv}" 须为正整数`;
    }
  }
  if (base === '*') {
    return ''; // * 与 */n 均合法
  }
  // 单值或 a-b 范围。
  const dash = base.indexOf('-');
  let a: number;
  let b: number;
  if (dash >= 0) {
    const aStr = base.slice(0, dash);
    const bStr = base.slice(dash + 1);
    if (!/^[0-9]+$/.test(aStr) || !/^[0-9]+$/.test(bStr)) {
      return `${name}: 范围须为数字 "${base}"`;
    }
    a = Number(aStr);
    b = Number(bStr);
    if (a < lo || b > hi || a > b) {
      return `${name}: 范围 ${a}-${b} 越界(合法 ${lo}..${hi})`;
    }
    return '';
  }
  if (!/^[0-9]+$/.test(base)) {
    return `${name}: "${base}" 无法解析 —— 仅数字 5 段 cron(如 "0 9 * * *" 每日9点);不支持名字(MON/JAN)、?、L/W/#、@`;
  }
  const v = Number(base);
  if (v < lo || v > hi) {
    return `${name}: ${v} 越界(合法 ${lo}..${hi})`;
  }
  return '';
}
