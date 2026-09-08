// run 计划账本展示块(Phase 10.3,契约 §3.5 + 10.4 §3.6):GET /tasks/{id}/plan → 只读阶段时间线。
// - plan=null(非流水线 run / 历史 run 无计划)→ 空态文案;
// - 有 plan → kind 徽标 + materialized 注记(10.4 起按 plan_policy 区分:
//   adaptive grow =「执行中生成」记账; synthesize grow 未认领 =「认领后合成」空态;
//   synthesize upfront =「合成计划 · 先审后干」)+ 阶段行列表。
// 消费方:TaskDrawer(任务详情抽屉「计划」区)、DecideModal(审批页先审后干)、
// ProjectDetail runs 行「计划」动作(经 TaskDrawer 复用)。
import { Alert, Flex, Spin } from 'antd';
import { getTaskPlan } from '../api/endpoints';
import type { PlanPhase, TaskPlan, TaskPlanResponse } from '../api/types';
import { useData } from '../hooks/useApi';
import {
  ALLOCATOR_CN,
  PHASE_KIND_CN,
  PHASE_STATUS_CN,
  PLAN_KIND_CN,
  PLAN_POLICY_CN,
  phaseStatusPreset,
} from '../utils/dicts';
import { fmtT } from '../utils/time';
import StatusTag from './StatusTag';

const PHASE_KIND_TONE: Record<string, string> = { do: 'pill-accent', accept: 'pill-muted', dispose: 'pill-warn' };

/** 计划主体(已取到 plan):kind 徽标 + 形态注记(10.4 synthesize 区分)+ 阶段行列表。 */
export function PlanTimeline({ plan }: { plan: TaskPlan }) {
  const p = plan;
  const phases: PlanPhase[] = p.phases ?? [];
  const synth = p.plan_policy === 'synthesize';
  const grow = p.materialized === 'grow';
  // grow 的注记:adaptive = 执行中生成(逐回合 append);synthesize = 认领后合成(首次认领 frontier 铺全)。
  const growLabel = synth ? '认领后合成' : '执行中生成';
  const note = grow
    ? synth
      ? '首次认领时由 frontier 合成整段计划(先审后干);合成失败将按自适应执行'
      : '计划随执行回合自动生成(非整包预演)'
    : synth
      ? '合成计划已落账 · 批准后逐阶段执行'
      : '建单即铺全,可先审后干';
  return (
    <div>
      <Flex wrap gap={6} align="center" style={{ marginBottom: 6 }}>
        <span className={`pill ${p.kind === 'patrol' ? 'pill-warn' : 'pill-accent'}`}>
          {PLAN_KIND_CN[p.kind] ?? p.kind}
        </span>
        {synth && p.kind === 'engineering' ? (
          <span className="pill pill-accent">{PLAN_POLICY_CN.synthesize}计划</span>
        ) : null}
        <span className={`pill ${grow ? 'pill-muted' : 'pill-accent'}`}>{grow ? growLabel : '预先铺全'}</span>
        <span className="dim" style={{ fontSize: 11.5 }}>
          {note}
        </span>
      </Flex>
      {phases.length === 0 ? (
        <div className="dim" style={{ fontSize: 12.5, padding: '4px 0' }}>
          {grow && synth
            ? '尚未认领执行:首次认领时 frontier 对意图合成计划(合成失败降级为自适应执行)'
            : grow
              ? '尚未生成任何阶段(认领执行后逐回合追加)'
              : '计划已铺全,暂无阶段明细'}
        </div>
      ) : (
        <div style={{ marginTop: 2 }}>
          {phases.map((ph) => (
            <div key={ph.seq} style={{ display: 'flex', gap: 10, padding: '6px 0', borderTop: '1px solid var(--line)' }}>
              <div className="mono dim" style={{ width: 22, textAlign: 'right', fontSize: 12, lineHeight: 1.7, flexShrink: 0 }}>
                {ph.seq}
              </div>
              <div style={{ width: 52, flexShrink: 0, paddingTop: 1 }}>
                <span className={`pill ${PHASE_KIND_TONE[ph.kind] ?? 'pill-muted'}`}>{PHASE_KIND_CN[ph.kind] ?? ph.kind}</span>
              </div>
              <div style={{ flex: 1, minWidth: 0 }}>
                <Flex gap={6} align="center" wrap>
                  <span style={{ fontWeight: 600, fontSize: 12.5 }}>{ph.title}</span>
                  <span className="dim" style={{ fontSize: 11.5 }}>{ALLOCATOR_CN[ph.allocator] ?? ph.allocator}</span>
                  <StatusTag preset={phaseStatusPreset(ph.status)} label={PHASE_STATUS_CN[ph.status] ?? ph.status} />
                </Flex>
                {ph.evidence || ph.note ? (
                  <div className="muted" style={{ fontSize: 11.5, marginTop: 2 }}>
                    {ph.evidence ? <span className="mono">{ph.evidence}</span> : null}
                    {ph.evidence && ph.note ? ' — ' : null}
                    {ph.note || null}
                  </div>
                ) : null}
                {ph.started_at || ph.finished_at ? (
                  <div className="mono dim" style={{ fontSize: 11, marginTop: 1 }}>
                    {ph.started_at ? fmtT(ph.started_at) : '—'} → {ph.finished_at ? fmtT(ph.finished_at) : '…'}
                  </div>
                ) : null}
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

/** 按 taskId 拉计划后渲染(错误 → 内联红字;任务不存在 → 空态)。
 * mode="decide"(审批 Modal):upfront 计划 → 决策上方渲染完整阶段列表 +「批准 = 按计划执行」提示;
 * grow → 提示执行时生成(adaptive)或认领后合成(synthesize),不渲染阶段。 */
export default function PlanBlock({ taskId, mode }: { taskId: string; mode?: 'timeline' | 'decide' }) {
  const plan = useData<TaskPlanResponse>(() => getTaskPlan(taskId), { deps: [taskId], enabled: !!taskId });

  if (plan.loading && !plan.data) {
    return (
      <div style={{ textAlign: 'center', padding: '14px 0' }}>
        <Spin size="small" />
      </div>
    );
  }
  if (plan.error) {
    return <div className="dim" style={{ fontSize: 12.5, color: 'var(--crit)' }}>计划读取失败:{plan.error}</div>;
  }
  const p = plan.data?.plan ?? null;
  if (mode === 'decide') {
    if (!p) {
      return (
        <Alert
          type="info"
          showIcon
          message="计划将在执行中生成"
          description="该 run 无预铺计划:工程类流水线按回合逐步记账(自适应)或首次认领时合成(synthesize),批准后照常执行。"
          style={{ marginBottom: 10 }}
        />
      );
    }
    const synth = p.plan_policy === 'synthesize';
    if (p.materialized === 'grow') {
      return (
        <Alert
          type={synth ? 'warning' : 'info'}
          showIcon
          message={synth ? '计划尚未合成(认领后合成)' : '计划将在执行中生成(自适应)'}
          description={
            synth
              ? '该 synthesize run 尚未被 worker 认领:首次认领时 frontier 对意图合成整段计划并过审批;批准后由 OS 按合成计划先审后干执行,合成失败将按自适应降级。'
              : `${PLAN_KIND_CN[p.kind] ?? p.kind} · 阶段随执行回合逐步记账,批准后照常执行。`
          }
          style={{ marginBottom: 10 }}
        />
      );
    }
    // upfront:先审后干 —— 决策按钮上方渲染完整计划(patrol 巡检 / engineering 合成计划同款)。
    const isPatrol = p.kind === 'patrol';
    return (
      <div style={{ marginBottom: 10 }}>
        <Alert
          type="warning"
          showIcon
          message="批准 = 按上述计划执行(先审后干)"
          description={
            isPatrol
              ? '该巡检 run 计划已预先铺全,放行后 OS 依计划逐阶段执行。'
              : synth
                ? '该 synthesize run 计划已由 frontier 铺全(合成计划 · 先审后干),放行后 OS 依计划逐阶段执行。'
                : '该 run 计划已铺全,放行后 OS 依计划逐阶段执行。'
          }
          style={{ marginBottom: 8 }}
        />
        <div style={{ border: '1px solid var(--line)', borderRadius: 6, padding: '4px 10px' }}>
          <PlanTimeline plan={p} />
        </div>
      </div>
    );
  }
  if (!p) {
    return <div className="dim" style={{ fontSize: 12.5 }}>非流水线 run / 历史 run 无计划</div>;
  }
  return <PlanTimeline plan={p} />;
}
