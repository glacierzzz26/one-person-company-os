// 模型端点池:GET /companies/{id}/endpoints。token 指示 = token_enc 非空(永不回显明文/密文)。
// 拉取模型 POST /endpoints/{id}/models → 服务端写 models_cache,行内 parse 成快选 chips;
// 选模型 POST /endpoints/{id}/select {model,role?};新增 POST /endpoints。
import { useState } from 'react';
import { App, Alert, Button, Card, Flex, Space, Table, Tooltip } from 'antd';
import { CloudDownloadOutlined, CheckCircleOutlined, PlusOutlined } from '@ant-design/icons';
import type { Endpoint } from '../api/types';
import { listEndpoints, fetchEndpointModels } from '../api/endpoints';
import { useApp } from '../store/AppContext';
import { useData } from '../hooks/useApi';
import { PageHead, EmptyState } from '../components/common';
import { EndpointCreateModal, EndpointSelectModelModal } from '../components/modals';
import { endpointRoleLabel } from '../utils/dicts';
import { parseModelsCache } from '../utils/parseModelsCache';
import { fmtT, short } from '../utils/time';

export default function Endpoints() {
  const { message } = App.useApp();
  const { companyId, refreshKey, bump } = useApp();
  const [createOpen, setCreateOpen] = useState(false);
  const [selecting, setSelecting] = useState<Endpoint | null>(null);
  const [pullingId, setPullingId] = useState<string | null>(null);

  const data = useData<Endpoint[]>(() => (companyId ? listEndpoints(companyId) : Promise.reject()), {
    deps: [companyId, refreshKey],
    enabled: !!companyId,
  });
  const rows = data.data ?? [];

  const pull = async (e: Endpoint) => {
    setPullingId(e.id);
    try {
      const models = await fetchEndpointModels(e.id);
      message.success(
        models.length
          ? `${e.name}:拉取到 ${models.length} 个模型(已写入 models_cache)`
          : `${e.name}:拉取成功但返回空(端点 /v1/models 无数据)`,
      );
      bump();
    } catch (err) {
      message.error(err instanceof Error ? err.message : String(err));
    } finally {
      setPullingId(null);
    }
  };

  return (
    <div>
      <PageHead
        title="模型端点池"
        sub="GET /api/v1/companies/{id}/endpoints · 池 pool / 规划 planner / 热备 standby"
        actions={
          <Button type="primary" icon={<PlusOutlined />} onClick={() => setCreateOpen(true)}>
            新增端点
          </Button>
        }
      />

      <Alert
        style={{ marginBottom: 12 }}
        type="info"
        showIcon
        message={
          <span style={{ fontSize: 12 }}>
            token 用 AES-GCM <span className="mono">enc:v1:</span> 密文落库,此处只显示「已配置 / 未配置」;base_url 按 vendor 识别 anthropic/openai
            协议。控制台不暴露加密密钥(OS_ENDPOINT_KEY)与明文 token。
          </span>
        }
      />

      <Card size="small" styles={{ body: { padding: 0 } }}>
        <Table<Endpoint>
          size="small"
          rowKey="id"
          dataSource={rows}
          pagination={{ pageSize: 12, showSizeChanger: false }}
          locale={{ emptyText: <EmptyState text="无端点 → 点右上「新增端点」录入首个 LLM 通道" /> }}
          columns={[
            {
              title: '端点',
              render: (_, e) => (
                <div>
                  <b>{e.name}</b>
                  <span className="pill pill-accent" style={{ marginLeft: 8 }}>{endpointRoleLabel(e.role)}</span>
                  {e.status === 'disabled' ? <span className="pill pill-crit" style={{ marginLeft: 4 }}>disabled</span> : null}
                  <div className="mono dim" style={{ fontSize: 11 }}>{short(e.id)} · vendor {e.vendor} · proto {e.proto}</div>
                </div>
              ),
            },
            {
              title: 'Base URL',
              render: (_, e) => (
                <span className="mono dim" style={{ fontSize: 12 }} title={e.base_url}>
                  {e.base_url}
                </span>
              ),
            },
            {
              title: '当前模型',
              width: 150,
              render: (_, e) =>
                e.selected_model ? (
                  <span className="pill pill-ok">
                    <b>{e.selected_model}</b>
                  </span>
                ) : (
                  <span className="dim" style={{ fontSize: 12 }}>未选定</span>
                ),
            },
            {
              title: 'Token',
              width: 130,
              render: (_, e) =>
                e.token_enc ? (
                  <span className="pill pill-ok">
                    <CheckCircleOutlined /> 已配置
                  </span>
                ) : (
                  <span className="pill pill-muted">无 token</span>
                ),
            },
            {
              title: '缓存模型(models_cache)',
              width: 220,
              render: (_, e) => {
                const cached = parseModelsCache(e.models_cache);
                if (!cached.length) return <span className="dim" style={{ fontSize: 12 }}>—</span>;
                const shown = cached.slice(0, 5);
                const more = cached.length - shown.length;
                return (
                  <Space size={[4, 4]} wrap>
                    {shown.map((m) => (
                      <span key={m} className="pill pill-muted mono" style={{ fontSize: 11 }}>
                        {m}
                      </span>
                    ))}
                    {more > 0 ? <span className="dim" style={{ fontSize: 11.5 }}>+{more}</span> : null}
                  </Space>
                );
              },
            },
            {
              title: '操作',
              width: 190,
              render: (_, e) => (
                <Flex gap={6}>
                  <Tooltip title="请求该端点 /v1/models,写回 models_cache">
                    <Button
                      size="small"
                      icon={<CloudDownloadOutlined />}
                      loading={pullingId === e.id}
                      onClick={() => pull(e)}
                    >
                      拉取模型
                    </Button>
                  </Tooltip>
                  <Button size="small" type="primary" onClick={() => setSelecting(e)}>
                    选择模型
                  </Button>
                </Flex>
              ),
            },
            {
              title: '更新',
              width: 90,
              render: (_, e) => <span className="dim" style={{ fontSize: 11.5 }}>{fmtT(e.updated_at)}</span>,
            },
          ]}
        />
      </Card>

      <div className="dim" style={{ fontSize: 11.5, marginTop: 10 }}>
        task.agent 会向 endpoint(按 role 取号:优先 planner,其次 pool,standby 仅 planner 失联时顶)发模型请求;离线环境请自建 fixture 网关。
      </div>

      <EndpointCreateModal open={createOpen} onClose={() => setCreateOpen(false)} onDone={bump} />
      <EndpointSelectModelModal endpoint={selecting} onClose={() => setSelecting(null)} onDone={bump} />
    </div>
  );
}
