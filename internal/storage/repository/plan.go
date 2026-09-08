package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/glacierzzz26/one-person-company-os/internal/plan"
	"github.com/glacierzzz26/one-person-company-os/internal/storage/query"
)

// Phase 10.3 run 计划账本仓储(契约 phase-plan-contract.md §3.2)。只写读 + 驱动内部翻状态;无外部写口。
// plan 最小外键仅挂 task(task_plan.task_id UNIQUE)→ 项目/流水线删除零新增语义,plan 随 task 保留。

func (s *Store) CreateTaskPlan(ctx context.Context, p plan.RunPlan) (plan.RunPlan, error) {
	row, err := s.q.CreateTaskPlan(ctx, query.CreateTaskPlanParams{
		ID: p.ID, TaskID: p.TaskID, Kind: p.Kind, Materialized: p.Materialized,
		CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
	})
	if err != nil {
		return plan.RunPlan{}, err
	}
	return toRunPlan(row), nil
}

// GetTaskPlanByTask 按 task 取计划;不存在 → found=false(nil err)。非流水线任务/0016 前历史 run 无计划。
func (s *Store) GetTaskPlanByTask(ctx context.Context, taskID string) (plan.RunPlan, bool, error) {
	row, err := s.q.GetTaskPlanByTask(ctx, taskID)
	if errors.Is(err, sql.ErrNoRows) {
		return plan.RunPlan{}, false, nil
	}
	if err != nil {
		return plan.RunPlan{}, false, err
	}
	return toRunPlan(row), true, nil
}

// SetPlanUpdated 刷 plan.updated_at(跟随最近 phase 变更;不为此加字段)。
func (s *Store) SetPlanUpdated(ctx context.Context, planID string, ts int64) error {
	return s.q.SetPlanUpdated(ctx, query.SetPlanUpdatedParams{UpdatedAt: ts, ID: planID})
}

func (s *Store) CreatePlanPhase(ctx context.Context, ph plan.RunPlanPhase) (plan.RunPlanPhase, error) {
	row, err := s.q.CreatePlanPhase(ctx, query.CreatePlanPhaseParams{
		ID: ph.ID, PlanID: ph.PlanID, Seq: ph.Seq, Kind: ph.Kind, Title: ph.Title,
		Allocator: ph.Allocator, Status: ph.Status, Evidence: ph.Evidence, Note: ph.Note,
		StartedAt: int64ToNull(ph.StartedAt), FinishedAt: int64ToNull(ph.FinishedAt),
	})
	if err != nil {
		return plan.RunPlanPhase{}, err
	}
	return toRunPlanPhase(row), nil
}

// ListPlanPhases 按 seq 升序列出计划全部阶段(空计划 = 空切片,grow 未执行/0 阶段)。
func (s *Store) ListPlanPhases(ctx context.Context, planID string) ([]plan.RunPlanPhase, error) {
	rows, err := s.q.ListPlanPhases(ctx, planID)
	if err != nil {
		return nil, err
	}
	out := make([]plan.RunPlanPhase, 0, len(rows))
	for _, r := range rows {
		out = append(out, toRunPlanPhase(r))
	}
	return out, nil
}

// UpdatePlanPhase 只更变列(列指针;nil 列不动)。先读当前行合并再整行回写 —— 单 operator 翻状态,
// 每次阶段边界一次调用,读-改-写成本可忽略;避免 sqlc 动态 WHERE/COALESCE 语义歧义。
func (s *Store) UpdatePlanPhase(ctx context.Context, id string, status, evidence, note *string, startedAt, finishedAt *int64) (plan.RunPlanPhase, error) {
	cur, err := s.q.GetPlanPhase(ctx, id)
	if err != nil {
		return plan.RunPlanPhase{}, err
	}
	st, ev, nt := cur.Status, cur.Evidence, cur.Note
	if status != nil {
		st = *status
	}
	if evidence != nil {
		ev = *evidence
	}
	if note != nil {
		nt = *note
	}
	stAt, finAt := cur.StartedAt, cur.FinishedAt
	if startedAt != nil {
		stAt = int64ToNull(startedAt)
	}
	if finishedAt != nil {
		finAt = int64ToNull(finishedAt)
	}
	row, err := s.q.ReplacePlanPhase(ctx, query.ReplacePlanPhaseParams{
		Status: st, Evidence: ev, Note: nt, StartedAt: stAt, FinishedAt: finAt, ID: id,
	})
	if err != nil {
		return plan.RunPlanPhase{}, err
	}
	return toRunPlanPhase(row), nil
}

func toRunPlan(r query.TaskPlan) plan.RunPlan {
	return plan.RunPlan{
		ID: r.ID, TaskID: r.TaskID, Kind: r.Kind, Materialized: r.Materialized,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

func toRunPlanPhase(r query.TaskPlanPhase) plan.RunPlanPhase {
	return plan.RunPlanPhase{
		ID: r.ID, PlanID: r.PlanID, Seq: r.Seq, Kind: r.Kind, Title: r.Title,
		Allocator: r.Allocator, Status: r.Status, Evidence: r.Evidence, Note: r.Note,
		StartedAt: nullToInt64Ptr(r.StartedAt), FinishedAt: nullToInt64Ptr(r.FinishedAt),
	}
}
