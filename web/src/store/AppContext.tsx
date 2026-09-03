// 全局应用上下文:公司选择、待审批徽标、连接健康、token/AuthModal、全局刷新键。
// 401 由 api client 触发 setUnauthorizedHandler → 这里打开 AuthModal(模块级解耦)。
import { createContext, useContext, useEffect, useMemo, useRef, useState } from 'react';
import type { ReactNode } from 'react';
import { listApprovals, listCompanies } from '../api/endpoints';
import type { Company } from '../api/types';
import { setUnauthorizedHandler } from '../api/client';

const COMPANY_KEY = 'os.company';

interface AppState {
  companies: Company[] | null; // null = 未加载(boot)
  reloadCompanies: () => void;
  companyId: string | null;
  selectCompany: (id: string) => void;
  pendingApprovals: number;
  connOk: boolean;
  connChecked: boolean;
  authOpen: boolean;
  openAuth: () => void;
  closeAuth: () => void;
  refreshKey: number; // 写操作后 bump → 依赖它的页面整体重拉
  bump: () => void;
}

const Ctx = createContext<AppState | null>(null);
export const useApp = (): AppState => {
  const v = useContext(Ctx);
  if (!v) throw new Error('useApp must be used within AppProvider');
  return v;
};

const HEALTH_MS = 20000;
const APPROVAL_MS = 15000;

export default function AppProvider({ children }: { children: ReactNode }) {
  const [companies, setCompanies] = useState<Company[] | null>(null);
  const [companyId, setCompanyId] = useState<string | null>(() => {
    try {
      return localStorage.getItem(COMPANY_KEY);
    } catch {
      return null;
    }
  });
  const [pendingApprovals, setPendingApprovals] = useState(0);
  const [connOk, setConnOk] = useState(false);
  const [connChecked, setConnChecked] = useState(false);
  const [authOpen, setAuthOpen] = useState(false);
  const [refreshKey, setRefreshKey] = useState(0);
  const openAuthRef = useRef<() => void>(() => {});
  openAuthRef.current = () => setAuthOpen(true);
  const authOpenRef = useRef(false);
  authOpenRef.current = authOpen;

  const reloadCompanies = () => {
    listCompanies()
      .then(setCompanies)
      .catch((e: unknown) => {
        setCompanies([]);
        console.error('list companies:', e);
      });
  };

  // boot 加载一次
  useEffect(() => {
    reloadCompanies();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // 401 → 弹 token 输入(api client 调用;防重复弹窗)
  useEffect(() => {
    setUnauthorizedHandler(() => {
      if (!authOpenRef.current) openAuthRef.current();
    });
    return () => setUnauthorizedHandler(null);
  }, []);

  // 健康检查(healthz 非信封,直接 fetch)
  useEffect(() => {
    let alive = true;
    const check = async () => {
      try {
        const res = await fetch('/healthz', { method: 'GET' });
        if (!alive) return;
        if (res.ok) {
          setConnOk(true);
        } else {
          setConnOk(false);
        }
      } catch {
        if (alive) setConnOk(false);
      } finally {
        if (alive) setConnChecked(true);
      }
    };
    check();
    const iv = setInterval(check, HEALTH_MS);
    return () => {
      alive = false;
      clearInterval(iv);
    };
  }, []);

  // 待审批徽标(审批全局,不随公司;15s 轮询,401/网络错静默)
  useEffect(() => {
    let alive = true;
    const poll = async () => {
      try {
        const list = await listApprovals('pending');
        if (alive) setPendingApprovals(list.length);
      } catch {
        /* 未授权/未连接:徽标保持上次,静默 */
      }
    };
    poll();
    const iv = setInterval(poll, APPROVAL_MS);
    return () => {
      alive = false;
      clearInterval(iv);
    };
  }, []);

  // 公司切换持久化
  const selectCompany = (id: string) => {
    setCompanyId(id);
    try {
      localStorage.setItem(COMPANY_KEY, id);
    } catch {
      /* noop */
    }
  };

  const value = useMemo<AppState>(
    () => ({
      companies,
      reloadCompanies,
      companyId,
      selectCompany,
      pendingApprovals,
      connOk,
      connChecked,
      authOpen,
      openAuth: () => setAuthOpen(true),
      closeAuth: () => setAuthOpen(false),
      refreshKey,
      bump: () => setRefreshKey((k) => k + 1),
    }),
    [companies, companyId, pendingApprovals, connOk, connChecked, authOpen, refreshKey],
  );

  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}
