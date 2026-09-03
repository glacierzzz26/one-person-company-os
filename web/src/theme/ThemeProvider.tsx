// 三态主题 light / dark / system;antd ConfigProvider algorithm + token 映射原型令牌;
// 同时把生效态写到 <html data-theme="light|dark">,驱动 styles.css 的 CSS 变量(图表/风险色)。
import { createContext, useContext, useEffect, useMemo, useState } from 'react';
import type { ReactNode } from 'react';
import { ConfigProvider, theme as antdTheme, App as AntApp } from 'antd';
import zhCN from 'antd/locale/zh_CN';
import dayjs from 'dayjs';
import 'dayjs/locale/zh-cn';

dayjs.locale('zh-cn');

export type ThemeMode = 'light' | 'dark' | 'system';
const KEY = 'os73-theme';

export interface ThemeCtx {
  mode: ThemeMode;
  setMode: (m: ThemeMode) => void;
  effective: 'light' | 'dark';
}

const Ctx = createContext<ThemeCtx>({ mode: 'system', setMode: () => {}, effective: 'light' });
export const useThemeMode = () => useContext(Ctx);

function readMode(): ThemeMode {
  try {
    const v = localStorage.getItem(KEY);
    if (v === 'light' || v === 'dark' || v === 'system') return v;
  } catch {
    /* noop */
  }
  return 'system';
}

const prefersDark = () => window.matchMedia?.('(prefers-color-scheme: dark)').matches ?? false;

export default function ThemeProvider({ children }: { children: ReactNode }) {
  const [mode, setModeState] = useState<ThemeMode>(readMode);
  const [sysDark, setSysDark] = useState(prefersDark);

  useEffect(() => {
    const mq = window.matchMedia('(prefers-color-scheme: dark)');
    const onChange = (e: MediaQueryListEvent) => setSysDark(e.matches);
    mq.addEventListener('change', onChange);
    return () => mq.removeEventListener('change', onChange);
  }, []);

  const effective: 'light' | 'dark' = mode === 'system' ? (sysDark ? 'dark' : 'light') : mode;

  // 显式把生效态写到 <html data-theme>,CSS 变量随之切换(不依赖 media 竞态)。
  useEffect(() => {
    document.documentElement.dataset.theme = effective;
  }, [effective]);

  const setMode = (m: ThemeMode) => {
    setModeState(m);
    try {
      if (m === 'system') localStorage.removeItem(KEY);
      else localStorage.setItem(KEY, m);
    } catch {
      /* noop */
    }
  };

  const themeTokens = useMemo(
    () => ({
      algorithm: effective === 'dark' ? antdTheme.darkAlgorithm : antdTheme.defaultAlgorithm,
      token: {
        colorPrimary: '#2f54eb',
        colorSuccess: '#1a9e65',
        colorWarning: '#c9861a',
        colorError: '#d43b52',
        colorInfo: '#2f54eb',
        borderRadius: 8,
        fontFamily:
          '"Noto Sans SC","PingFang SC","Microsoft YaHei",system-ui,-apple-system,sans-serif',
      },
      components: {
        Layout: {
          headerBg: 'transparent',
          siderBg: '#141a26',
        },
        Menu: {
          darkItemBg: '#141a26',
          darkItemColor: '#aebaca',
          darkItemSelectedBg: '#2f54eb',
          darkItemSelectedColor: '#ffffff',
          darkSubMenuItemBg: '#101725',
        },
      },
    }),
    [effective],
  );

  return (
    <Ctx.Provider value={{ mode, setMode, effective }}>
      <ConfigProvider locale={zhCN} theme={themeTokens}>
        <AntApp>{children}</AntApp>
      </ConfigProvider>
    </Ctx.Provider>
  );
}
