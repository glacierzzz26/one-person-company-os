import React from 'react';
import ReactDOM from 'react-dom/client';
import App from './App';
import './styles.css';

// Phase 7.3 boot:styles.css 载入(CSS 变量 + pill/sb 语义色,随 data-theme 明暗切换);
// 主题/路由/公司由 ThemeProvider + AppProvider 层叠,App 内处理空库建公司。
ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
);
