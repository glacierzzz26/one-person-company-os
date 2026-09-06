// Boot:ThemeProvider(三态)→ BootGate(/setup 首启门)→ AppProvider(公司/审批徽标/连接)→
// Shell(空库建公司 / Spin / 主布局)。路由 / /approvals /decisions /tasks /repos /memories /audit
// /endpoints /settings(与原型 8+1 视图一致;settings 为 9.2「控制台访问」卡)。
import { useEffect, useState } from 'react';
import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom';
import { Flex, Spin } from 'antd';
import ThemeProvider from './theme/ThemeProvider';
import AppProvider, { useApp } from './store/AppContext';
import AuthModal from './components/AuthModal';
import CreateCompanyScreen from './components/CreateCompanyScreen';
import AppLayout from './components/AppLayout';
import Overview from './pages/Overview';
import Approvals from './pages/Approvals';
import Decisions from './pages/Decisions';
import Tasks from './pages/Tasks';
import Repos from './pages/Repos';
import Memories from './pages/Memories';
import Audit from './pages/Audit';
import Endpoints from './pages/Endpoints';
import Settings from './pages/Settings';
import SetupWizard from './pages/SetupWizard';
import { setupStatus } from './api/endpoints';

function FullSpin({ tip = '加载中…' }: { tip?: string }) {
  return (
    <Flex align="center" justify="center" style={{ minHeight: '100vh' }}>
      <Spin size="large" tip={tip}>
        <div style={{ width: 200, height: 60 }} />
      </Spin>
    </Flex>
  );
}

function Shell() {
  const { companies, companyId, selectCompany } = useApp();
  if (companies === null) return <FullSpin tip="加载公司…" />;
  if (companies.length === 0) return <CreateCompanyScreen />;

  const active = companyId && companies.some((c) => c.id === companyId) ? companyId : companies[0].id;
  useEffect(() => {
    if (active !== companyId) selectCompany(active);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [active, companyId]);

  return <AppLayout />;
}

// 首启门(Phase 9.2):/setup/status 显示未初始化(console_token_hash='')→ 全屏向导;已初始化 → 正常壳。
// 后端连不上 → 放行进壳(连接胶囊会显示「未连接」;向导需后端才可提交)。
function BootGate() {
  const [phase, setPhase] = useState<'checking' | 'setup' | 'ready'>('checking');
  useEffect(() => {
    let alive = true;
    setupStatus()
      .then((s) => {
        if (alive) setPhase(s.initialized ? 'ready' : 'setup');
      })
      .catch(() => {
        if (alive) setPhase('ready');
      });
    return () => {
      alive = false;
    };
  }, []);

  if (phase === 'checking') return <FullSpin tip="检测系统状态…" />;
  if (phase === 'setup') return <SetupWizard onDone={() => setPhase('ready')} />;
  return (
    <AppProvider>
      <AuthModal />
      <Routes>
        <Route element={<Shell />}>
          <Route index element={<Overview />} path="/" />
          <Route path="/approvals" element={<Approvals />} />
          <Route path="/decisions" element={<Decisions />} />
          <Route path="/tasks" element={<Tasks />} />
          <Route path="/repos" element={<Repos />} />
          <Route path="/memories" element={<Memories />} />
          <Route path="/audit" element={<Audit />} />
          <Route path="/endpoints" element={<Endpoints />} />
          <Route path="/settings" element={<Settings />} />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Route>
      </Routes>
    </AppProvider>
  );
}

export default function App() {
  return (
    <ThemeProvider>
      <BrowserRouter>
        <BootGate />
      </BrowserRouter>
    </ThemeProvider>
  );
}
