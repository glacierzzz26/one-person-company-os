// Boot:ThemeProvider(三态)→ AppProvider(公司/审批徽标/连接)→ Shell(空库建公司 / Spin / 主布局)。
// 路由 / /approvals /decisions /tasks /repos /memories /audit /endpoints(与原型 8 视图一致)。
import { useEffect } from 'react';
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

function FullSpin() {
  return (
    <Flex align="center" justify="center" style={{ minHeight: '100vh' }}>
      <Spin size="large" tip="加载公司…">
        <div style={{ width: 180, height: 60 }} />
      </Spin>
    </Flex>
  );
}

function Shell() {
  const { companies, companyId, selectCompany } = useApp();
  if (companies === null) return <FullSpin />;
  if (companies.length === 0) return <CreateCompanyScreen />;

  const active = companyId && companies.some((c) => c.id === companyId) ? companyId : companies[0].id;
  useEffect(() => {
    if (active !== companyId) selectCompany(active);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [active, companyId]);

  return <AppLayout />;
}

export default function App() {
  return (
    <ThemeProvider>
      <BrowserRouter>
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
              <Route path="*" element={<Navigate to="/" replace />} />
            </Route>
          </Routes>
        </AppProvider>
      </BrowserRouter>
    </ThemeProvider>
  );
}
