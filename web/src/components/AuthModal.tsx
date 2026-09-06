// token 输入弹窗:401 或点击连接胶囊时打开;提交即存 localStorage 并整体重载(boot 重拉)。
import { useState } from 'react';
import { Alert, Button, Input, Modal, Typography } from 'antd';
import { getToken, setToken } from '../api/client';
import { useApp } from '../store/AppContext';

export default function AuthModal() {
  const { authOpen, closeAuth } = useApp();
  const [val, setVal] = useState('');
  const [err, setErr] = useState('');
  const hasToken = !!getToken();

  const ok = () => {
    const t = val.trim();
    if (!t) {
      setErr('token 不能为空(留空请取消)');
      return;
    }
    setToken(t);
    closeAuth();
    // 整页重载:token 生效后重走 boot / 各页首次拉取,避免局部陈旧状态
    window.location.reload();
  };

  const clear = () => {
    setToken('');
    closeAuth();
    window.location.reload();
  };

  return (
    <Modal
      title="控制台令牌 · /api/v1 访问令牌"
      open={authOpen}
      onCancel={closeAuth}
      footer={[
        <Button key="cancel" onClick={closeAuth}>
          取消
        </Button>,
        hasToken ? (
          <Button key="clear" danger onClick={clear}>
            清除令牌
          </Button>
        ) : null,
        <Button key="ok" type="primary" onClick={ok}>
          保存并重载
        </Button>,
      ]}
    >
      <Typography.Paragraph type="secondary" style={{ fontSize: 12.5 }}>
        已通过 <span className="mono">/setup</span> 首启后,/api/v1 要求
        <span className="mono"> Authorization: Bearer &lt;token&gt;</span>(DB 只存哈希)。
        此 token 仅存于本浏览器 localStorage,提交后整页重载生效;轮换请到「设置」页。
      </Typography.Paragraph>
      <Input.Password
        autoFocus
        placeholder="Bearer token…"
        value={val}
        onChange={(e) => {
          setVal(e.target.value);
          setErr('');
        }}
        onPressEnter={ok}
      />
      {err ? <Alert style={{ marginTop: 8 }} type="error" showIcon message={err} /> : null}
      <div style={{ fontSize: 12, color: 'var(--text-3)', marginTop: 10 }}>
        {hasToken ? '当前已配置令牌 —— 可通过上方按钮清除。' : '未配置 —— 开放后端可留空并关闭。'}
      </div>
    </Modal>
  );
}
