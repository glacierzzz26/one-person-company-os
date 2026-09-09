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
		ParentTaskID:       ptrToNull(t.ParentTaskID),
		RoundNo:            t.RoundNo,
		ConflictCount:      t.ConflictCount,
		WriterEndpointID:   ptrToNull(t.WriterEndpointID),
		ReviewerEndpointID: ptrToNull(t.ReviewerEndpointID),
		TestEndpointID:     ptrToNull(t.TestEndpointID),
		ProjectID:          ptrToNull(t.ProjectID),
		PipelineID:         ptrToNull(t.PipelineID),
		CreatedAt:          t.CreatedAt, UpdatedAt: t.UpdatedAt,
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

// ListTasksByParent 返回某父任务(planner 拆解父请求)下的全部子任务(创建序)。无子任务 → 空切片。
func (s *Store) ListTasksByParent(ctx context.Context, parentTaskID string) ([]task.Task, error) {
	rows, err := s.q.ListTasksByParent(ctx, sql.NullString{String: parentTaskID, Valid: true})
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
		ParentTaskID:       nullToPtr(r.ParentTaskID),
		RoundNo:            r.RoundNo,
		ConflictCount:      r.ConflictCount,
		WriterEndpointID:   nullToPtr(r.WriterEndpointID),
		ReviewerEndpointID: nullToPtr(r.ReviewerEndpointID),
		TestEndpointID:     nullToPtr(r.TestEndpointID),
		ProjectID:          nullToPtr(r.ProjectID),
		PipelineID:         nullToPtr(r.PipelineID),
		PullRequestURL:     r.PullRequestUrl.String, PullRequestNumber: nullIntToPtr(r.PullRequestNumber),
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

// nullIntToPtr sql.NullInt64 → *int64(nil 值 → nil)。task 可空 INTEGER 列(10.5 PR 号)用。
func nullIntToPtr(n sql.NullInt64) *int64 {
	if !n.Valid {
		return nil
	}
	v := n.Int64
	return &v
}

// SetTaskPullRequest 记任务收尾发的 PR(pull_request_url / pull_request_number;幂等账本)。
// Phase 10.5 publishRunPR 成功后落;URL 已置 = 已发,重试不重复建。
func (s *Store) SetTaskPullRequest(ctx context.Context, taskID, url string, number int64) error {
	var num sql.NullInt64
	if number > 0 {
		num = sql.NullInt64{Int64: number, Valid: true}
	}
	_, err := s.q.SetTaskPullRequest(ctx, query.SetTaskPullRequestParams{
		PullRequestUrl:    sql.NullString{String: url, Valid: url != ""},
		PullRequestNumber: num,
		UpdatedAt:         now(),
		ID:                taskID,
	})
	return err
}

// SetTaskRound 更新 Task 回合状态(round_no / conflict_count),Engineering Driver 每次
// 推进回合后调用;返回更新后的 Task。
func (s *Store) SetTaskRound(ctx context.Context, taskID string, roundNo, conflictCount int64) (task.Task, error) {
	row, err := s.q.UpdateTaskRound(ctx, query.UpdateTaskRoundParams{
		RoundNo: roundNo, ConflictCount: conflictCount, UpdatedAt: now(), ID: taskID,
	})
	if err != nil {
		return task.Task{}, err
	}
	return toTask(row), nil
}

// ListTasksByProject 返回挂某项目的任务(创建倒序,limit 截断)。Phase 10.1:项目详情「最近 runs」。
func (s *Store) ListTasksByProject(ctx context.Context, projectID string, limit int64) ([]task.Task, error) {
	rows, err := s.q.ListTasksByProject(ctx, query.ListTasksByProjectParams{
		ProjectID: ptrToNull(&projectID), Limit: limit,
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

// CountActiveTasksByProject 返回某项目下活跃任务数(pending/running/waiting_approval)。
// Phase 10.1:同项目 run 串行守卫(>0 → 409)。
func (s *Store) CountActiveTasksByProject(ctx context.Context, projectID string) (int64, error) {
	cnt, err := s.q.CountActiveTasksByProject(ctx, ptrToNull(&projectID))
	if err != nil {
		return 0, err
	}
	return cnt, nil
}

// ClearTaskProject 解除某项目下全部任务的 project/pipeline 引用(双清 → NULL)。
// Phase 10.1/10.2:删项目先断引用(FK ON),保留任务历史,不级联删任务。
// 项目任务的 pipeline 必属本项目(pipeline run 挂同项目 task),故同条 UPDATE 双清。
func (s *Store) ClearTaskProject(ctx context.Context, projectID string) (int64, error) {
	return s.q.ClearTaskProject(ctx, ptrToNull(&projectID))
}

// ClearTaskPipeline 解除某流水线下全部任务的 pipeline 引用(pipeline_id → NULL)。
// Phase 10.2:删流水线先断引用(FK ON),run 历史任务保留(同 project 语义)。
func (s *Store) ClearTaskPipeline(ctx context.Context, pipelineID string) (int64, error) {
	return s.q.ClearTaskPipeline(ctx, ptrToNull(&pipelineID))
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
