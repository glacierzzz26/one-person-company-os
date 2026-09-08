package service

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/glacierzzz26/one-person-company-os/internal/plan"
	"github.com/glacierzzz26/one-person-company-os/internal/task"
	"github.com/google/uuid"
)

// Phase 10.3 — run 计划账本服务助手(契约 docs/phase10/design/phase-plan-contract.md §3.3)。
// 纯记账/翻状态副作用:不新起状态机、不加 pipeline 字段、不改控制流。
//   - ensureRunPlan:RunPipelineAs 建单成功后同步落账(幂等)——
//     ops_patrol → materialized=upfront 并按模板一次铺 5 条 pending 行(先审后干);
//     bugfix/develop(engineering)→ materialized=grow(只建 plan 行,执行中 append)。
//   - runPatrol 过渡 = runLedger(seq → 行)start/finish;机械预检失败 → accept fail 不经判读。
//   - runEngineering/planner 的 grow 记账 = appendPlanPhase(按 title 去重,跨 requeue 幂等)。
//   - 非流水线 run / 0016 前历史 run 无 plan → 全部助手 no-op(计划只服务 RunPipelineAs 建单的 run)。

// ensureRunPlan 幂等建 run 计划(先于任何 worker 认领;与建单同步)。已存在 → no-op(重放/重复认领)。
func (s *Service) ensureRunPlan(ctx context.Context, t task.Task, kind string) error {
	if _, ok, err := s.store.GetTaskPlanByTask(ctx, t.ID); err != nil {
		return err
	} else if ok {
		return nil
	}
	now := time.Now().Unix()
	rp := plan.RunPlan{ID: uuid.NewString(), TaskID: t.ID, Kind: kind, CreatedAt: now, UpdatedAt: now}
	if kind == plan.PlanKindPatrol {
		rp.Materialized = plan.MaterializedUpfront
	} else {
		rp.Materialized = plan.MaterializedGrow
	}
	if _, err := s.store.CreateTaskPlan(ctx, rp); err != nil {
		return err
	}
	if kind != plan.PlanKindPatrol {
		return nil // grow:只建 plan 行,阶段执行中 append
	}
	reportRel := filepath.Join(PatrolDirName, t.ID+".md")
	for i, tpl := range patrolPlanTemplate(t, reportRel) {
		tpl.ID = uuid.NewString()
		tpl.PlanID = rp.ID
		tpl.Seq = int64(i + 1) // 模板不预写 seq;预铺时按序 1..5(UNIQUE(plan_id, seq))
		if _, err := s.store.CreatePlanPhase(ctx, tpl); err != nil {
			return err
		}
	}
	return nil
}

// patrolPlanTemplate 巡检 run 的 5 条预铺阶段(seq1..5;契约 §3.3 逐阶段对应)。
// status 全部 pending(先审后干);seq = i+1 由调用方铺写。
func patrolPlanTemplate(t task.Task, reportRel string) []plan.RunPlanPhase {
	return []plan.RunPlanPhase{
		{Kind: plan.PhaseKindDo, Title: "巡检:委派产出报告 " + reportRel, Allocator: plan.AllocatorDelegate, Status: plan.PhaseStatusPending},
		{Kind: plan.PhaseKindDo, Title: "提交巡检报告(OS git commit)", Allocator: plan.AllocatorOS, Status: plan.PhaseStatusPending},
		{Kind: plan.PhaseKindAccept, Title: "OS 机械预检:报告存在 + 工作树无越界残留", Allocator: plan.AllocatorOS, Status: plan.PhaseStatusPending},
		{Kind: plan.PhaseKindAccept, Title: "巡检判读(frontier/scripted verdict)", Allocator: plan.AllocatorJudge, Status: plan.PhaseStatusPending},
		{Kind: plan.PhaseKindDispose, Title: "发现处置(fix&high 链拉 / 留人)", Allocator: plan.AllocatorManual, Status: plan.PhaseStatusPending},
	}
}

// planKindFor 把 pipeline.kind 映到计划形态(与驱动分流同源;bugfix/develop 同 engineering)。
func planKindFor(kind string) string {
	if kind == "ops_patrol" {
		return plan.PlanKindPatrol
	}
	return plan.PlanKindEngineering
}

// GetTaskPlan 取 run 计划 + 阶段列表(只读;供 HTTP GET /tasks/{id}/plan)。
// 非流水线 run / 历史 run 无 plan → found=false。
func (s *Service) GetTaskPlan(ctx context.Context, taskID string) (plan.RunPlan, []plan.RunPlanPhase, bool, error) {
	rp, ok, err := s.store.GetTaskPlanByTask(ctx, taskID)
	if err != nil || !ok {
		return plan.RunPlan{}, nil, ok, err
	}
	phases, err := s.store.ListPlanPhases(ctx, rp.ID)
	if err != nil {
		return plan.RunPlan{}, nil, false, err
	}
	return rp, phases, true, nil
}

// ---- grow 记账(engineering:执行中 append;按 title 去重 → 跨 requeue/re-entry 幂等) ----

// appendPlanPhase 把一行阶段追加进 task 的 grow 计划:同 title 行已存在 → no-op(幂等)。
// 无 plan(非流水线 run)→ no-op。终态行(非 pending)补 started/finished=now(记录已完成的真实边界);
// pending 行(planner ask 等未决)留空时间戳。失败透传(插桩失败 → fail 该 task,不吞驱动错误)。
func (s *Service) appendPlanPhase(ctx context.Context, t task.Task, ph plan.RunPlanPhase) error {
	rp, ok, err := s.store.GetTaskPlanByTask(ctx, t.ID)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	phases, err := s.store.ListPlanPhases(ctx, rp.ID)
	if err != nil {
		return err
	}
	for _, p := range phases {
		if p.Title == ph.Title {
			return nil // 已记过该边界(requeue/re-entry/同 round 返工)→ 不重复 append
		}
	}
	now := time.Now().Unix()
	ph.ID = uuid.NewString()
	ph.PlanID = rp.ID
	ph.Seq = int64(len(phases) + 1)
	if ph.Status != plan.PhaseStatusPending {
		if ph.StartedAt == nil {
			ph.StartedAt = &now
		}
		if ph.FinishedAt == nil {
			ph.FinishedAt = &now
		}
	}
	if _, err := s.store.CreatePlanPhase(ctx, ph); err != nil {
		return err
	}
	return s.store.SetPlanUpdated(ctx, rp.ID, now)
}

// ledgerAppendRound 把一个 round 的写→测→审三行追加(round 完成/裁决时调用一次):
// do「写 round R」delegate / accept「测试 round R」judge(ok;免费返工次数进 note)/
// accept「评审 round R」judge(reviewStatus ok|fail;reason 进 note)。
func (s *Service) ledgerAppendRound(ctx context.Context, t task.Task, round int64, diff, testSummary string, freeReworks int64, reviewStatus, reviewNote string) error {
	diffShort := truncate(firstLine(diff), 120)
	testNote := ""
	if freeReworks > 0 {
		testNote = fmt.Sprintf("免费返工 %d 次", freeReworks)
	}
	if err := s.appendPlanPhase(ctx, t, plan.RunPlanPhase{
		Kind: plan.PhaseKindDo, Title: fmt.Sprintf("写 round %d", round),
		Allocator: plan.AllocatorDelegate, Status: plan.PhaseStatusOK, Evidence: diffShort,
	}); err != nil {
		return err
	}
	if err := s.appendPlanPhase(ctx, t, plan.RunPlanPhase{
		Kind: plan.PhaseKindAccept, Title: fmt.Sprintf("测试 round %d", round),
		Allocator: plan.AllocatorJudge, Status: plan.PhaseStatusOK,
		Evidence: truncate(firstLine(testSummary), 120), Note: testNote,
	}); err != nil {
		return err
	}
	if err := s.appendPlanPhase(ctx, t, plan.RunPlanPhase{
		Kind: plan.PhaseKindAccept, Title: fmt.Sprintf("评审 round %d", round),
		Allocator: plan.AllocatorJudge, Status: reviewStatus,
		Evidence: truncate(firstLine(reviewNote), 120), Note: reviewNote,
	}); err != nil {
		return err
	}
	return nil
}

// ledgerAppendSplit 记 planner split(父 run 拆解 N 子任务;子任务本身无 plan)。
func (s *Service) ledgerAppendSplit(ctx context.Context, t task.Task, n int) error {
	return s.appendPlanPhase(ctx, t, plan.RunPlanPhase{
		Kind: plan.PhaseKindDo, Title: "planner 拆解序曲",
		Allocator: plan.AllocatorPlanner, Status: plan.PhaseStatusOK,
		Evidence: fmt.Sprintf("split → %d subtasks", n),
	})
}

// ledgerAppendAsk 记 planner ask(超边界 → 人工审批;行保持 pending,批准后续跑)。
func (s *Service) ledgerAppendAsk(ctx context.Context, t task.Task, reason string) error {
	return s.appendPlanPhase(ctx, t, plan.RunPlanPhase{
		Kind: plan.PhaseKindDo, Title: "planner 拆解序曲",
		Allocator: plan.AllocatorPlanner, Status: plan.PhaseStatusPending,
		Evidence: "", Note: "ask → 人工审批: " + truncate(firstLine(reason), 120),
	})
}

// ---- runPatrol 过渡(upfront 预铺行翻状态;无计划 run → 全部 no-op) ----

// runLedger 一次巡检认领内加载的计划视图(seq → 行);start/finish 同时刷本地 + 落库。
type runLedger struct {
	plan   plan.RunPlan
	phases map[int64]plan.RunPlanPhase
}

// loadRunLedger 取巡检 run 的预铺计划;无计划 → nil(不阻塞既有语义)。
func (s *Service) loadRunLedger(ctx context.Context, taskID string) (*runLedger, error) {
	rp, ok, err := s.store.GetTaskPlanByTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}
	rows, err := s.store.ListPlanPhases(ctx, rp.ID)
	if err != nil {
		return nil, err
	}
	m := make(map[int64]plan.RunPlanPhase, len(rows))
	for _, r := range rows {
		m[r.Seq] = r
	}
	return &runLedger{plan: rp, phases: m}, nil
}

// lgStart 仅 pending → running(记 started_at);已在 running/终态(requeue 续跑)→ no-op。
func (s *Service) lgStart(ctx context.Context, lg *runLedger, seq int64) error {
	if lg == nil {
		return nil
	}
	ph, ok := lg.phases[seq]
	if !ok || ph.Status != plan.PhaseStatusPending {
		return nil
	}
	now := time.Now().Unix()
	st := now
	upd, err := s.store.UpdatePlanPhase(ctx, ph.ID, strPtr(plan.PhaseStatusRunning), nil, nil, &st, nil)
	if err != nil {
		return err
	}
	lg.phases[seq] = upd
	return s.store.SetPlanUpdated(ctx, lg.plan.ID, now)
}

// lgFinish 置终态(ok|fail|skipped;填 evidence/note + finished_at;started_at 缺省补 now)。
// 目标态与现有相同(requeue 续跑重复完成)→ no-op。
func (s *Service) lgFinish(ctx context.Context, lg *runLedger, seq int64, status, evidence, note string) error {
	if lg == nil {
		return nil
	}
	ph, ok := lg.phases[seq]
	if !ok {
		return nil
	}
	if ph.Status == status && ph.Evidence == evidence && ph.Note == note {
		return nil
	}
	now := time.Now().Unix()
	var st, fin *int64
	if ph.StartedAt == nil {
		st = &now
	}
	fin = &now
	upd, err := s.store.UpdatePlanPhase(ctx, ph.ID, &status, &evidence, &note, st, fin)
	if err != nil {
		return err
	}
	lg.phases[seq] = upd
	return s.store.SetPlanUpdated(ctx, lg.plan.ID, now)
}

func strPtr(s string) *string { return &s }
