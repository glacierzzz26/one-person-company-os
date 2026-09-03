// 总览:GET /companies/{id}/overview(15s)。五数值卡 + RD 卡(fused/waiting/ledger)+ 待审批 +
// 最近任务 + 能力域/工作流/最近决策/记忆高亮。rd=null(无 engineering)整体防护。
import { useMemo, useState } from 'react';
import { Alert, App, Button, Card, Flex, Row, Col, Space, Spin, Table, Tag } from 'antd';
import { SyncOutlined, PlusOutlined, RightOutlined } from '@ant-design/icons';
import { useNavigate } from 'react-router-dom';
import { getOverview, intakeSync } from '../api/endpoints';
import type { Approval, Overview as OverviewType, RDTask, Task } from '../api/types';
import { useApp } from '../store/AppContext';
import { useData } from '../hooks/useApi';
import { PageHead, StatTile, RiskText, EmptyState } from '../components/common';
import StatusTag from '../components/StatusTag';
import { TaskCreateModal } from '../components/modals';
import TaskDrawer from '../components/TaskDrawer';
import DecideModal from '../components/DecideModal';
import {
  TASK_STATUS_KEYS,
  TASK_STATUS_META,
  TASK_STATUS_BAR,
  taskStatusLabel,
  taskStatusPreset,
  decisionKindLabel,
} from '../utils/dicts';
import { fmtT, ago } from '../utils/time';

const tilesGrid = { display: 'grid', gridTemplateColumns: 'repeat(auto-fit,minmax(168px,1fr))', gap: 12 } as const;

export default function Overview() {
  const { message } = App.useApp();
  const { companyId, connOk, refreshKey, bump } = useApp();
  const nav = useNavigate();

  const [syncBusy, setSyncBusy] = useState(false);
  const [taskOpen, setTaskOpen] = useState(false);
  const [drawerTask, setDrawerTask] = useState<string | null>(null);
  const [decideApproval, setDecideApproval] = useState<Approval | null>(null);

  const data = useData<OverviewType>(
    () => (companyId ? getOverview(companyId) : Promise.reject(new Error('no company'))),
    { deps: [companyId, refreshKey], intervalMs: 15000, enabled: !!companyId },
  );

  const ov = data.data;

  const doSync = async () => {
    if (!companyId) return;
    setSyncBusy(true);
    try {
      const rs = await intakeSync(companyId);
      const total = rs.reduce((a, r) => a + r.issues_seen, 0);
      const created = rs.reduce((a, r) => a + r.created_tasks.length, 0);
      message.success(`通道 B 同步完成:${rs.length} 仓库 · ${total} issue 看到 · ${created} 任务建单`);
      bump();
    } catch (e) {
      message.error(e instanceof Error ? e.message : String(e));
    } finally {
      setSyncBusy(false);
    }
  };

  const rd = ov?.rd ?? null;

  // Go nil slice → JSON null(capabilities/workflows/pending_approvals 都可能为 null)统一兜底空数组
  const caps = ov?.capabilities ?? [];
  const wfs = ov?.workflows ?? [];
  const pendingApps = ov?.pending_approvals ?? [];
  const recentTasks = ov?.recent_tasks ?? [];

  // 数值卡
  const running = rd?.by_status.running ?? 0;
  const pending = rd?.by_status.pending ?? 0;
  const fusedN = rd?.fused.length ?? 0;
  const completed = rd?.by_status.completed ?? 0;
  const ledgerSeen = rd?.ledger_seen ?? 0;

  // 分布堆积条:按固定顺序,>0 才显示
  const order: { k: (typeof TASK_STATUS_KEYS)[number]; bar: string }[] = TASK_STATUS_KEYS.map((k) => ({
    k,
    bar: TASK_STATUS_BAR[k],
  }));
  const stack = order
    .map(({ k, bar }) => ({ k, bar, n: rd?.by_status[k] ?? 0 }))
    .filter((s) => s.n > 0);
  const total = rd?.total ?? 0;

  const ledgerRows = [
    { k: 'direct_work', t: '直接做', bar: 'sb-ok' },
    { k: 'merge', t: '合并批次', bar: 'sb-accent' },
    { k: 'skip', t: '跳过', bar: 'sb-muted' },
    { k: 'ask', t: '问人', bar: 'sb-warn' },
  ];

  const topRecentTasks = useMemo(() => recentTasks.filter((t) => !t.parent_task_id).slice(0, 6), [recentTasks]);

  if (data.loading && !ov) {
    return (
      <div style={{ display: 'grid', placeItems: 'center', minHeight: '50vh' }}>
        <Spin size="large" />
      </div>
    );
  }
  if (!ov) {
    return (
      <div>
        <PageHead title="总览" sub="GET /api/v1/companies/{id}/overview" />
        <Alert type="error" showIcon message={data.error ?? '无数据'} />
      </div>
    );
  }

  const actionButtons = (
    <>
      <Button icon={<SyncOutlined />} loading={syncBusy} onClick={doSync}>
        通道 B 同步
      </Button>
      <Button type="primary" icon={<PlusOutlined />} onClick={() => setTaskOpen(true)}>
        新建工程请求
      </Button>
    </>
  );

  return (
    <div>
      <PageHead
        title="总览"
        sub={`GET /api/v1/companies/${companyId}/overview · 数据 ${fmtT(Date.now() / 1000)}`}
        actions={actionButtons}
      />

      <div style={tilesGrid}>
        <StatTile k="进行中任务(RD)" v={running + pending} x={`执行 ${running} · 排队 ${pending}`} />
        <StatTile k="待我审批" v={pendingApps.length} x="全局 · 点击行内「决策」" tone={pendingApps.length ? 'warn' : undefined} />
        <StatTile k="研发部熔断" v={fusedN} x="评审连驳 ≥3 次" tone={fusedN ? 'crit' : undefined} />
        <StatTile k="已完成任务(RD)" v={completed} x={`子任务 ${rd?.subtask_count ?? 0}(planner split)`} />
        <StatTile
          k="issue 账本"
          v={ledgerSeen}
          x={`direct ${rd?.ledger.direct_work ?? 0} · merge ${rd?.ledger.merge ?? 0} · skip ${rd?.ledger.skip ?? 0}`}
        />
      </div>

      {!connOk ? (
        <Alert style={{ marginTop: 12 }} type="warning" showIcon message="后端连接异常 —— 数据可能为上次成功缓存" />
      ) : null}

      <Row gutter={[16, 16]} style={{ marginTop: 16 }}>
        {/* 左列 */}
        <Col xs={24} xl={15} style={{ minWidth: 0 }}>
          {!rd ? (
            <Card size="small" title="研发部 · RD" style={{ marginBottom: 16 }}>
              <Alert type="info" showIcon message="未配置 engineering Capability —— 研发部聚合为空(无熔断 / 账本)。" />
            </Card>
          ) : (
            <Card
              size="small"
              title="研发部 · RD Engineering"
              extra={<span className="mono dim" style={{ fontSize: 11 }}>overview → data.rd</span>}
              style={{ marginBottom: 16 }}
            >
              <Flex justify="space-between" align="center" wrap gap={8}>
                <Space>
                  <b>任务分布</b>
                  <span className="dim" style={{ fontSize: 12 }}>
                    total {rd.total} · 子任务 {rd.subtask_count}(planner split)
                  </span>
                </Space>
              </Flex>
              {/* 堆积条 */}
              <div
                style={{
                  display: 'flex',
                  height: 10,
                  borderRadius: 5,
                  overflow: 'hidden',
                  background: 'var(--surface-2)',
                  marginTop: 8,
                }}
              >
                {stack.map((s) => (
                  <div key={s.k} className={s.bar} style={{ width: total ? `${(s.n / total) * 100}%` : 0 }} title={`${TASK_STATUS_META[s.k].t} ${s.n}`} />
                ))}
              </div>
              <Flex wrap gap={12} style={{ marginTop: 9, fontSize: 12, color: 'var(--text-2)' }}>
                {stack.map((s) => (
                  <span key={s.k}>
                    <i
                      className={s.bar}
                      style={{ display: 'inline-block', width: 8, height: 8, borderRadius: 2, marginRight: 5 }}
                    />
                    {TASK_STATUS_META[s.k].t} <b className="mono">{s.n}</b>
                  </span>
                ))}
              </Flex>

              <div className="sect-h">熔断(fused)</div>
              {rd.fused.length ? (
                <div style={{ border: '1px solid var(--line)', borderRadius: 6 }}>
                  {rd.fused.map((f) => (
                    <RDFuseRow key={f.id} f={f} onGo={() => nav('/approvals')} />
                  ))}
                </div>
              ) : (
                <EmptyState text="无熔断 · 研发部回合机运转正常" />
              )}

              {rd.waiting.length ? (
                <>
                  <div className="sect-h">待审批门(waiting)</div>
                  <div style={{ border: '1px solid var(--line)', borderRadius: 6 }}>
                    {rd.waiting.map((f) => (
                      <RDWaitRow key={f.id} f={f} onGo={() => nav('/approvals')} />
                    ))}
                  </div>
                </>
              ) : null}

              <div className="sect-h">GitHub issue 处置账本</div>
              {ledgerRows.map((r) => {
                const n = rd.ledger[r.k] ?? 0;
                const w = rd.ledger_seen ? `${(n / rd.ledger_seen) * 100}%` : '0%';
                return (
                  <Flex key={r.k} gap={10} align="center" style={{ margin: '4px 0', fontSize: 12.5, color: 'var(--text-2)' }}>
                    <span className="pl mono" style={{ width: 110, flex: 'none', display: 'flex', justifyContent: 'space-between' }}>
                      <span>{r.t}</span>
                      <b>{n}</b>
                    </span>
                    <div style={{ flex: 1, maxWidth: 260, height: 6, borderRadius: 3, background: 'var(--surface-2)', overflow: 'hidden' }}>
                      <div className={r.bar} style={{ width: w, height: '100%' }} />
                    </div>
                  </Flex>
                );
              })}
              <div className="dim" style={{ fontSize: 11.5, marginTop: 6 }}>
                POST /api/v1/companies/{companyId}/intake/sync 与 webhook 共写同一账本(UNIQUE 去重)
              </div>
            </Card>
          )}

          <Card
            size="small"
            title="待我审批"
            extra={
              <Button size="small" type="link" onClick={() => nav('/approvals')}>
                审批中心 <RightOutlined />
              </Button>
            }
            style={{ marginBottom: 16 }}
          >
            {pendingApps.length ? (
              <div style={{ display: 'flex', flexDirection: 'column' }}>
                {pendingApps.slice(0, 4).map((a) => (
                  <Flex
                    key={a.approval.id}
                    align="center"
                    gap={12}
                    style={{ padding: '11px 4px', borderBottom: '1px solid var(--line)' }}
                  >
                    <Flex style={{ minWidth: 0 }} flex={1} vertical>
                      <b style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{a.task_title}</b>
                      <div className="meta dim" style={{ fontSize: 12 }}>
                        {a.approval.requested_by} · {ago(a.approval.created_at)}
                      </div>
                    </Flex>
                    <Button size="small" type="primary" onClick={() => nav('/approvals')}>
                      决策
                    </Button>
                  </Flex>
                ))}
              </div>
            ) : (
              <EmptyState text="全部处理完毕" />
            )}
          </Card>

          <Card
            size="small"
            title="最近任务"
            extra={
              <Button size="small" type="link" onClick={() => nav('/tasks')}>
                任务中心 <RightOutlined />
              </Button>
            }
          >
            <Table<Task>
              size="small"
              rowKey="id"
              pagination={false}
              dataSource={topRecentTasks}
              onRow={(t) => ({ onClick: () => setDrawerTask(t.id), className: 'row-click' })}
              columns={[
                {
                  title: '标题',
                  dataIndex: 'title',
                  render: (v: string, t) => (
                    <span>
                      <b>{v}</b>
                      {t.tool_name ? <Tag style={{ marginLeft: 8 }}>{t.tool_name}</Tag> : null}
                    </span>
                  ),
                },
                {
                  title: '状态',
                  width: 96,
                  render: (_, t) => <StatusTag preset={taskStatusPreset(t.status)} label={taskStatusLabel(t.status)} />,
                },
                { title: '风险', width: 76, render: (_, t) => <RiskText risk={t.risk} /> },
                { title: '更新', width: 100, render: (_, t) => <span className="dim" style={{ fontSize: 12 }}>{ago(t.updated_at)}</span> },
              ]}
            />
          </Card>
        </Col>

        {/* 右列 */}
        <Col xs={24} xl={9} style={{ minWidth: 0 }}>
          <Card size="small" title="能力域 × Agent" extra={<span className="mono dim" style={{ fontSize: 11 }}>GET …/capabilities</span>} style={{ marginBottom: 16 }}>
            {caps.length ? (
              caps.map((c) => (
                <Flex key={c.code} align="center" gap={10} style={{ padding: '8px 4px', borderBottom: '1px solid var(--line)' }}>
                  <b style={{ width: 76, flex: 'none', fontSize: 13 }}>{c.name}</b>
                  <span className="mono dim" style={{ fontSize: 10.5, flex: 1 }}>
                    {c.code}
                  </span>
                  <span className="dim" style={{ fontSize: 12 }}>{c.agent_count} agents</span>
                </Flex>
              ))
            ) : (
              <EmptyState text="无能力域(CLI:os capability add)" />
            )}
          </Card>

          <Card
            size="small"
            title="工作流"
            extra={<span className="mono dim" style={{ fontSize: 11 }}>GET …/workflows(定义只读)</span>}
            style={{ marginBottom: 16 }}
          >
            {wfs.length ? (
              wfs.map((wv) => {
                const entries = Object.entries(wv.statuses ?? {}).filter(([, n]) => n > 0);
                return (
                  <Flex key={wv.workflow.id} align="center" gap={8} style={{ padding: '8px 4px', borderBottom: '1px solid var(--line)' }}>
                    <b style={{ flex: 1, minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                      {wv.workflow.name}
                    </b>
                    {entries.map(([st, n]) => (
                      <StatusTag key={st} preset={taskStatusPreset(st)} label={`${taskStatusLabel(st)} ${n}`} />
                    ))}
                  </Flex>
                );
              })
            ) : (
              <EmptyState text="无工作流" />
            )}
            <Alert
              style={{ marginTop: 8 }}
              type="info"
              showIcon
              message={
                <span style={{ fontSize: 12 }}>
                  长时驱动不在此页:终端 <span className="mono">os workflow run &lt;id&gt;</span>;队列由{' '}
                  <span className="mono">os queue work</span> 或 server <span className="mono">--queue-work</span> 消费
                </span>
              }
            />
          </Card>

          <Card size="small" title="最近决策" extra={<span className="mono dim" style={{ fontSize: 11 }}>GET …/decisions</span>} style={{ marginBottom: 16 }}>
            {(ov.recent_decisions ?? []).length ? (
              ov.recent_decisions.slice(0, 3).map((d) => (
                <Flex key={d.id} align="center" gap={10} style={{ padding: '9px 4px', borderBottom: '1px solid var(--line)' }}>
                  <Flex flex={1} vertical style={{ minWidth: 0 }}>
                    <b style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{d.title}</b>
                    <div className="dim" style={{ fontSize: 12 }}>
                      {decisionKindLabel(d.kind)} · {d.decided_by} · {fmtT(d.created_at)}
                    </div>
                  </Flex>
                  <Tag>{d.status}</Tag>
                </Flex>
              ))
            ) : (
              <EmptyState text="暂无决策" />
            )}
          </Card>

          <Card
            size="small"
            title="记忆高亮"
            extra={
              <Button size="small" type="link" onClick={() => nav('/memories')}>
                知识库 <RightOutlined />
              </Button>
            }
          >
            {(ov.memory_highlights ?? []).length ? (
              ov.memory_highlights.slice(0, 3).map((m) => (
                <Flex key={m.id} align="center" gap={10} style={{ padding: '9px 4px', borderBottom: '1px solid var(--line)' }}>
                  <Flex flex={1} vertical style={{ minWidth: 0 }}>
                    <b style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{m.title}</b>
                    <div className="dim" style={{ fontSize: 12 }}>
                      {m.type} · {ago(m.created_at)}
                    </div>
                  </Flex>
                  <Tag className="mono">{m.tags.split(/\s+/)[0] || '—'}</Tag>
                </Flex>
              ))
            ) : (
              <EmptyState text="暂无记忆沉淀" />
            )}
          </Card>
        </Col>
      </Row>

      <TaskCreateModal open={taskOpen} onClose={() => setTaskOpen(false)} onDone={bump} />
      <TaskDrawer taskId={drawerTask} onClose={() => setDrawerTask(null)} onOpenDecide={(a) => setDecideApproval(a)} />
      <DecideModal
        approval={decideApproval}
        onClose={() => setDecideApproval(null)}
        onDone={() => {
          setDecideApproval(null);
          setDrawerTask(null);
          bump();
        }}
      />
    </div>
  );
}

function RDFuseRow({ f, onGo }: { f: RDTask; onGo: () => void }) {
  return (
    <Flex align="center" gap={12} style={{ padding: '11px 16px', borderLeft: '3px solid var(--crit)', borderBottom: '1px solid var(--line)' }}>
      <Flex flex={1} vertical style={{ minWidth: 0 }}>
        <b style={{ color: 'var(--crit)' }}>
          🔥 {f.title} · 回合 {f.round} · 冲突 {f.conflict}
        </b>
        <div className="mono dim" style={{ fontSize: 12 }}>{f.reason}</div>
      </Flex>
      <Button size="small" danger onClick={onGo}>
        去处理
      </Button>
    </Flex>
  );
}

function RDWaitRow({ f, onGo }: { f: RDTask; onGo: () => void }) {
  return (
    <Flex align="center" gap={12} style={{ padding: '11px 16px', borderBottom: '1px solid var(--line)' }}>
      <Flex flex={1} vertical style={{ minWidth: 0 }}>
        <b>{f.title}</b>
        <div className="mono dim" style={{ fontSize: 12 }}>{f.reason}</div>
      </Flex>
      <Button size="small" onClick={onGo}>
        审批
      </Button>
    </Flex>
  );
}
