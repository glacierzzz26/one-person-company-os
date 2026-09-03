// models_cache 是 string:最近一次 /v1/models 原始返回的 JSON。防御性解析为模型 id 数组。
// 真实端点返回形状不定:对象 {data:[{id}…]} / 数组 / OpenAI compatible / Anthropic 变体 → 兜底空数组。

export function parseModelsCache(raw: string | undefined | null): string[] {
  if (!raw) return [];
  try {
    const v = JSON.parse(raw);
    const arr: unknown[] = Array.isArray(v) ? v : v && Array.isArray((v as { data?: unknown }).data) ? (v as { data: unknown[] }).data : [];
    const ids: string[] = [];
    for (const it of arr) {
      if (typeof it === 'string') {
        if (it && !ids.includes(it)) ids.push(it);
      } else if (it && typeof it === 'object') {
        const id = (it as { id?: unknown }).id;
        if (typeof id === 'string' && id && !ids.includes(id)) ids.push(id);
      }
    }
    return ids;
  } catch {
    return [];
  }
}
