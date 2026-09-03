// 主布局:Sider(恒深色品牌+分组导航+审批徽标)+ 顶栏(公司选择/连接/主题/刷新)+ 流体内容。
import { useMemo } from 'react';
import { Layout, Menu, Select, Button, Badge, Dropdown, Space, Tooltip } from 'antd';
import {
  ApiOutlined,
  BookOutlined,
  CarryOutOutlined,
  CheckSquareOutlined,
  DashboardOutlined,
  GithubOutlined,
  HistoryOutlined,
  MoonOutlined,
  ReloadOutlined,
  SafetyOutlined,
  SunOutlined,
} from '@ant-design/icons';
import { Outlet, useLocation, useNavigate } from 'react-router-dom';
import { useApp } from '../store/AppContext';
import { useThemeMode } from '../theme/ThemeProvider';
import type { ThemeMode } from '../theme/ThemeProvider';
import { getToken } from '../api/client';

const { Sider, Header, Content, Footer } = Layout;

const NAV = [
  { key: '/', label: '总览', icon: <DashboardOutlined /> },
  {
    key: 'human',
    type: 'group',
    label: '人的决策',
    children: [
      { key: '/approvals', label: '待我审批', icon: <CheckSquareOutlined /> },
      { key: '/decisions', label: '决策档案', icon: <HistoryOutlined /> },
    ],
  } as never,
  {
    key: 'exec',
    type: 'group',
    label: '执行',
    children: [
      { key: '/tasks', label: '任务中心', icon: <CarryOutOutlined /> },
      { key: '/repos', label: '研发仓库 · 通道 B', icon: <GithubOutlined /> },
    ],
  } as never,
  {
    key: 'bank',
    type: 'group',
    label: '沉淀',
    children: [
      { key: '/memories', label: '知识库', icon: <BookOutlined /> },
      { key: '/audit', label: '审计流水', icon: <SafetyOutlined /> },
    ],
  } as never,
  {
    key: 'infra',
    type: 'group',
    label: '基础设施',
    children: [{ key: '/endpoints', label: '模型端点池', icon: <ApiOutlined /> }],
  } as never,
];

export default function AppLayout() {
  const { companies, companyId, selectCompany, pendingApprovals, connOk, connChecked, openAuth } = useApp();
  const { mode, effective, setMode } = useThemeMode();
  const loc = useLocation();
  const nav = useNavigate();

  const active = useMemo(() => {
    if (loc.pathname === '/') return '/';
    // 一级路径匹配(如 /approvals/xxx → /approvals 仍高亮)
    const first = `/${loc.pathname.split('/')[1] || ''}`;
    const valid = ['/approvals', '/decisions', '/tasks', '/repos', '/memories', '/audit', '/endpoints'];
    return valid.includes(first) ? first : '/';
  }, [loc.pathname]);

  const hasToken = !!getToken();
  const connText = !connChecked ? '检测中…' : connOk ? '已连接' : '未连接 :8787';
  const connColor = !connChecked ? 'var(--text-3)' : connOk ? 'var(--ok)' : 'var(--crit)';

  // 审批菜单项注入徽标
  const menuItems = useMemo(() => {
    const copy = NAV.map((it) => {
      if ((it as { type?: string }).type === 'group') return it;
      if ((it as { key: string }).key === '/approvals') {
        return {
          ...it,
          label: (
            <Space>
              待我审批
              <Badge count={pendingApprovals} size="small" offset={[4, 0]} />
            </Space>
          ),
        };
      }
      return it;
    });
    return copy as unknown as Parameters<typeof Menu>[0]['items'];
  }, [pendingApprovals]);

  const themeItems = useMemo(
    () => [
      { key: 'light', label: '浅色' },
      { key: 'dark', label: '深色' },
      { key: 'system', label: '跟随系统' },
    ],
    [],
  );
  const themeLabel = mode === 'system' ? `自动(${effective === 'dark' ? '深' : '浅'})` : mode === 'dark' ? '深色' : '浅色';

  return (
    <Layout style={{ minHeight: '100vh' }}>
      <Sider
        width={224}
        style={{ background: 'var(--side-bg)', position: 'sticky', top: 0, height: '100vh', overflow: 'auto' }}
      >
        <div style={{ padding: '16px 18px 12px', display: 'flex', gap: 10, alignItems: 'center' }}>
          <div
            style={{
              width: 30,
              height: 30,
              borderRadius: 8,
              background: 'var(--accent)',
              color: '#fff',
              display: 'grid',
              placeItems: 'center',
              fontWeight: 700,
            }}
          >
            OS
          </div>
          <div>
            <div style={{ color: '#f0f4fa', fontWeight: 700, fontSize: 14 }}>一人公司 OS</div>
            <div className="mono" style={{ fontSize: 11, color: '#aebaca', opacity: 0.8 }}>
              ops console · 7.3
            </div>
          </div>
        </div>
        <div style={{ padding: '4px 8px 14px' }}>
          <Menu
            theme="dark"
            mode="inline"
            selectedKeys={[active]}
            items={menuItems}
            onClick={(e) => nav(e.key)}
            style={{ background: 'transparent', borderInlineEnd: 0 }}
          />
        </div>
        <div
          className="mono"
          style={{ position: 'sticky', top: '100vh', padding: '10px 16px', fontSize: 11, opacity: 0.75, color: '#aebaca' }}
        >
          GET /api/v1/… · 信封 {`{ok,data}`} · snake_case
        </div>
      </Sider>
      <Layout>
        <Header
          style={{
            position: 'sticky',
            top: 0,
            zIndex: 30,
            height: 56,
            lineHeight: '56px',
            background: 'var(--surface)',
            borderBottom: '1px solid var(--line)',
            padding: '0 24px',
            display: 'flex',
            alignItems: 'center',
            gap: 14,
          }}
        >
          <Select
            value={companyId ?? undefined}
            style={{ minWidth: 220 }}
            onChange={selectCompany}
            options={(companies ?? []).map((c) => ({ value: c.id, label: c.name }))}
            placeholder="选择公司"
          />
          <span className="dim" style={{ fontSize: 12 }}>
            {companies?.find((c) => c.id === companyId)?.vision || ''}
          </span>
          <div className="grow" />
          <Tooltip title={connOk ? 'os server 健康' : '无法连接后端(确认 os server 已起)'}>
            <button
              onClick={openAuth}
              style={{
                display: 'inline-flex',
                alignItems: 'center',
                gap: 6,
                fontFamily: 'JetBrains Mono, ui-monospace, monospace',
                fontSize: 11,
                border: '1px solid var(--line)',
                borderRadius: 999,
                padding: '5px 10px',
                color: 'var(--text-2)',
                background: 'transparent',
                cursor: 'pointer',
              }}
            >
              <i style={{ width: 7, height: 7, borderRadius: '50%', background: connColor, flex: 'none' }} />
              {connText}
              {hasToken ? ' · Bearer 已配' : ''}
            </button>
          </Tooltip>
          <Tooltip title="整页刷新(重拉数据)">
            <Button
              type="text"
              icon={<ReloadOutlined />}
              onClick={() => window.location.reload()}
              aria-label="刷新"
            />
          </Tooltip>
          <Dropdown
            menu={{ items: themeItems, selectable: true, selectedKeys: [mode], onClick: ({ key }) => setMode(key as ThemeMode) }}
          >
            <Button type="text" icon={effective === 'dark' ? <MoonOutlined /> : <SunOutlined />}>
              {themeLabel}
            </Button>
          </Dropdown>
        </Header>
        <Content style={{ padding: '22px 24px 40px', width: '100%' }}>
          <Outlet />
        </Content>
        <Footer style={{ textAlign: 'left', padding: '10px 24px', background: 'var(--surface)', borderTop: '1px solid var(--line)' }}>
          <span className="mono dim" style={{ fontSize: 11, display: 'flex', gap: 20 }}>
            <span>
              <i style={{ color: connOk ? 'var(--ok)' : 'var(--crit)', fontStyle: 'normal' }}>●</i> os server
            </span>
            <span>Phase 7.3 · 单二进制 go:embed · /api/v1 数据通道</span>
          </span>
        </Footer>
      </Layout>
    </Layout>
  );
}
