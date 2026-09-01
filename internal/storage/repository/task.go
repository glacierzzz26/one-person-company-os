package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/glacierzzz26/one-person-company-os/internal/storage/query"
	"github.com/glacierzzz26/one-person-company-os/internal/task"
)

func (s *Store) CreateTask(ctx context.Context, t task.Task) (task.Task, error) {
	row, err := s.q.CreateTask(ctx, query.CreateTaskParams{
		ID: t.ID, CompanyID: t.CompanyID,
		CapabilityID: ptrToNull(t.CapabilityID),
		WorkflowID:   ptrToNull(t.WorkflowID),
		AgentID:      ptrToNull(t.AgentID),
		Title:        t.Title, Description: t.Description, ToolName: t.ToolName, Status: t.Status,
		Priority: t.Priority, Attempt: t.Attempt, Risk: t.Risk,
		Qstatus: t.QStatus, LeaseWorkerID: t.LeaseWorkerID, LeaseUntil: t.LeaseUntil,
		MaxAttempts: t.MaxAttempts, TimeoutSec: t.TimeoutSec,
		LastError: t.LastError, Result: t.Result, WorkspacePath: t.WorkspacePath,
		CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt,
	})
	if err != nil {
		return task.Task{}, err
	}
	return toTask(row), nil
}

func (s *Store) GetTask(ctx context.Context, id string) (task.Task, error) {
	row, err := s.q.GetTask(ctx, id)
	if err != nil {
		return task.Task{}, err
	}
	return toTask(row), nil
}

func (s *Store) ListTasks(ctx context.Context, companyFilter, statusFilter, riskFilter string, attemptMin int64) ([]task.Task, error) {
	rows, err := s.q.ListTasks(ctx, query.ListTasksParams{
		CompanyFilter: companyFilter,
		CompanyID:     companyFilter,
		StatusFilter:  statusFilter,
		Status:        statusFilter,
		RiskFilter:    riskFilter,
		Risk:          riskFilter,
		AttemptFilter: attemptMin,
	})
	if err != nil {
		return nil, err
	}
	out := make([]task.Task, 0, len(rows))
	for _, r := range rows {
		out = append(out, toTask(r))
	}
	return out, nil
}

// ClaimTask 原子领取指定 Task(qstatus: ready → leased)。未领取到返回 ok=false。
func (s *Store) ClaimTask(ctx context.Context, taskID, workerID string, leaseUntil int64) (task.Task, bool, error) {
	row, err := s.q.ClaimTask(ctx, query.ClaimTaskParams{
		LeaseWorkerID: workerID, LeaseUntil: leaseUntil, UpdatedAt: now(), ID: taskID,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return task.Task{}, false, nil
	}
	if err != nil {
		return task.Task{}, false, err
	}
	return toTask(row), true, nil
}

// LeaseNextTask 原子领取下一个 READY Task(按 priority desc, created_at asc)。无任务时 ok=false。
func (s *Store) LeaseNextTask(ctx context.Context, workerID string, leaseUntil int64) (task.Task, bool, error) {
	row, err := s.q.LeaseNextTask(ctx, query.LeaseNextTaskParams{
		LeaseWorkerID: workerID, LeaseUntil: leaseUntil, UpdatedAt: now(),
	})
	if errors.Is(err, sql.ErrNoRows) {
		return task.Task{}, false, nil
	}
	if err != nil {
		return task.Task{}, false, err
	}
	return toTask(row), true, nil
}

func (s *Store) MarkTaskRunning(ctx context.Context, taskID string) (task.Task, error) {
	row, err := s.q.MarkTaskRunning(ctx, query.MarkTaskRunningParams{UpdatedAt: now(), ID: taskID})
	if err != nil {
		return task.Task{}, err
	}
	return toTask(row), nil
}

func (s *Store) CompleteTask(ctx context.Context, taskID, result string) (task.Task, error) {
	row, err := s.q.CompleteTask(ctx, query.CompleteTaskParams{Result: result, UpdatedAt: now(), ID: taskID})
	if err != nil {
		return task.Task{}, err
	}
	return toTask(row), nil
}

func (s *Store) FailTask(ctx context.Context, taskID, lastError string) (task.Task, error) {
	row, err := s.q.FailTask(ctx, query.FailTaskParams{LastError: lastError, UpdatedAt: now(), ID: taskID})
	if err != nil {
		return task.Task{}, err
	}
	return toTask(row), nil
}

// RequeueTask 重试:attempt+1 回 READY。返回新 task。
func (s *Store) RequeueTask(ctx context.Context, taskID, lastError string) (task.Task, error) {
	row, err := s.q.RequeueTask(ctx, query.RequeueTaskParams{LastError: lastError, UpdatedAt: now(), ID: taskID})
	if err != nil {
		return task.Task{}, err
	}
	return toTask(row), nil
}

// RecoverLeasedTasks 回收过期租约(孤儿 LEASED → READY)。
func (s *Store) RecoverLeasedTasks(ctx context.Context) ([]task.Task, error) {
	rows, err := s.q.RecoverLeasedTasks(ctx, query.RecoverLeasedTasksParams{UpdatedAt: now(), LeaseUntil: now()})
	if err != nil {
		return nil, err
	}
	out := make([]task.Task, 0, len(rows))
	for _, r := range rows {
		out = append(out, toTask(r))
	}
	return out, nil
}

// RequestApprovalTask 把已领取的 Task 置为 waiting_approval(qstatus 同步,释放租约)。
func (s *Store) RequestApprovalTask(ctx context.Context, taskID string) (task.Task, error) {
	row, err := s.q.RequestApprovalTask(ctx, query.RequestApprovalTaskParams{ID: taskID, UpdatedAt: now()})
	if err != nil {
		return task.Task{}, err
	}
	return toTask(row), nil
}

// ApproveTask 审批通过:Task 重新入队(ready/pending),由 worker 领取执行。
func (s *Store) ApproveTask(ctx context.Context, taskID string) (task.Task, error) {
	row, err := s.q.ApproveTask(ctx, query.ApproveTaskParams{ID: taskID, UpdatedAt: now()})
	if err != nil {
		return task.Task{}, err
	}
	return toTask(row), nil
}

// ListRecentTasks 最近 N 个 Task(创建时间倒序),用于全景视图。
func (s *Store) ListRecentTasks(ctx context.Context, companyID string, limit int64) ([]task.Task, error) {
	rows, err := s.q.ListRecentTasks(ctx, query.ListRecentTasksParams{CompanyID: companyID, Limit: limit})
	if err != nil {
		return nil, err
	}
	out := make([]task.Task, 0, len(rows))
	for _, r := range rows {
		out = append(out, toTask(r))
	}
	return out, nil
}

// TaskStatusCountsByWorkflow 各 Workflow 的任务状态计数,用于全景视图。
type TaskStatusCount struct {
	WorkflowID string
	Status     string
	Cnt        int64
}

func (s *Store) TaskStatusCountsByWorkflow(ctx context.Context, companyID string) ([]TaskStatusCount, error) {
	rows, err := s.q.TaskStatusCountsByWorkflow(ctx, companyID)
	if err != nil {
		return nil, err
	}
	out := make([]TaskStatusCount, 0, len(rows))
	for _, r := range rows {
		out = append(out, TaskStatusCount{
			WorkflowID: r.WorkflowID.String, Status: r.Status, Cnt: r.Cnt,
		})
	}
	return out, nil
}

func toTask(r query.Task) task.Task {
	return task.Task{
		ID: r.ID, CompanyID: r.CompanyID,
		CapabilityID: nullToPtr(r.CapabilityID),
		WorkflowID:   nullToPtr(r.WorkflowID),
		AgentID:      nullToPtr(r.AgentID),
		Title:        r.Title, Description: r.Description, ToolName: r.ToolName, Status: r.Status,
		Priority: r.Priority, Attempt: r.Attempt, Risk: r.Risk,
		QStatus: r.Qstatus, LeaseWorkerID: r.LeaseWorkerID, LeaseUntil: r.LeaseUntil,
		MaxAttempts: r.MaxAttempts, TimeoutSec: r.TimeoutSec,
		LastError: r.LastError, Result: r.Result, WorkspacePath: r.WorkspacePath,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

func nullToPtr(s sql.NullString) *string {
	if !s.Valid {
		return nil
	}
	v := s.String
	return &v
}

func ptrToNull(p *string) sql.NullString {
	if p == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *p, Valid: true}
}
