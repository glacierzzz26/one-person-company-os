// 轻量数据加载 hook:调用 fetcher,支持 interval 轮询与手动 refresh。
// 页面副作用后调用全局 refreshKey 提升(deps 含其值)即整体刷新,不引重状态库。
import { useCallback, useEffect, useRef, useState } from 'react';

export interface UseDataResult<T> {
  data: T | undefined;
  error: string | null;
  loading: boolean;
  refresh: () => void;
}

export function useData<T>(
  fetcher: () => Promise<T>,
  opts: { deps?: unknown[]; intervalMs?: number; enabled?: boolean } = {},
): UseDataResult<T> {
  const { deps = [], intervalMs = 0, enabled = true } = opts;
  const [data, setData] = useState<T | undefined>(undefined);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [tick, setTick] = useState(0);
  const fetcherRef = useRef(fetcher);
  fetcherRef.current = fetcher;

  // deps 为字符串/数字(companyId、filter 等) → JSON 序列化即可稳定比较。
  const depsKey = JSON.stringify(deps);

  useEffect(() => {
    if (!enabled) {
      setLoading(false);
      return;
    }
    let alive = true;
    setLoading(true);
    setError(null);
    const run = () => {
      fetcherRef
        .current()
        .then((d) => {
          if (alive) {
            setData(d);
            setLoading(false);
          }
        })
        .catch((e: unknown) => {
          if (alive) {
            setError(e instanceof Error ? e.message : String(e));
            setLoading(false);
          }
        });
    };
    run();
    if (intervalMs > 0) {
      const iv = setInterval(run, intervalMs);
      return () => {
        alive = false;
        clearInterval(iv);
      };
    }
    return () => {
      alive = false;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [enabled, intervalMs, tick, depsKey]);

  const refresh = useCallback(() => setTick((t) => t + 1), []);
  return { data, error, loading, refresh };
}
