package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/glacierzzz26/one-person-company-os/internal/execution"
	"github.com/glacierzzz26/one-person-company-os/internal/task"
	"github.com/glacierzzz26/one-person-company-os/internal/tool"
	"github.com/google/uuid"
)

const defaultLeaseMinutes = 30

// LeaseAndExecute 领取并执行下一个 READY Task。无任务时 ok=false。返回执行的 task id。
func (s *Service) LeaseAndExecute(ctx context.Context, workerID string) (string, bool, error) {
	t, ok, err := s.store.LeaseNextTask(ctx, workerID, leaseUntil())
	if err != nil {
		return "", false, err
	}
	if !ok {
		return "", false, nil
	}
	if err := s.runClaimed(ctx, workerID, t); err != nil {
		return t.ID, true, err
	}
	return t.ID, true, nil
}

// ExecuteTask 领取并执行指定 Task(qstatus 必须为 ready)。
func (s *Service) ExecuteTask(ctx context.Context, workerID, taskID string) error {
	t, err := s.store.GetTask(ctx, taskID)
	if err != nil {
		return err
	}
	if t.Status == "completed" || t.Status == "failed" {
		return fmt.Errorf("task %s already %s", taskID, t.Status)
	}
	claimed, ok, err := s.store.ClaimTask(ctx, taskID, workerID, leaseUntil())
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("task %s is not ready (qstatus=%s)", taskID, claimed.QStatus)
	}
	return s.runClaimed(ctx, workerID, claimed)
}

// RecoverLeasedTasks 回收孤儿 LEASED Task(租约过期)。返回回收数量。
func (s *Service) RecoverLeasedTasks(ctx context.Context) (int, error) {
	recovered, err := s.store.RecoverLeasedTasks(ctx)
	if err != nil {
		return 0, err
	}
	return len(recovered), nil
}

func (s *Service) ListExecutions(ctx context.Context, taskID, status string) ([]execution.Execution, error) {
	return s.store.ListExecutions(ctx, taskID, status)
}

func (s *Service) GetExecution(ctx context.Context, id string) (execution.Execution, error) {
	return s.store.GetExecution(ctx, id)
}

// runClaimed 执行已领取的 Task:先判定是否需要人工审批(risk=high 或 approval policy),
// 需要则置 waiting_approval 不执行;否则做 Tool 权限校验(默认拒绝),未授权则
// Audit deny 并失败;通过后创建 Execution,在 Tool 沙箱内执行 task.Description。
// engineering 家族 Task 先分流给 Engineering Driver(回合循环),0-5 语义不变。
func (s *Service) runClaimed(ctx context.Context, workerID string, t task.Task) error {
	if isEngineeringTask(t) {
		return s.runEngineering(ctx, workerID, t)
	}
	if need, reason, err := s.needsApproval(ctx, t); err != nil {
		return err
	} else if need {
		return s.requestApproval(ctx, t, reason)
	}

	tl, ok := tool.Get(t.ToolName)
	if !ok {
		return fmt.Errorf("task %s: tool %q not registered", t.ID, t.ToolName)
	}

	allowed, err := s.authorizeTool(ctx, t, tl)
	if err != nil {
		return err
	}
	if !allowed {
		msg := fmt.Sprintf("unauthorized: role has no permission for %s/%s", tl.Permission().Action, tl.Permission().Resource)
		if _, err := s.audit(ctx, "tool", tl.Name(), "deny", taskActor(t), msg+": "+t.Title); err != nil {
			return err
		}
		_, err := s.store.FailTask(ctx, t.ID, msg)
		return err
	}

	execID := uuid.NewString()
	nowUnix := time.Now().Unix()
	if _, err := s.store.CreateExecution(ctx, execution.Execution{
		ID: execID, TaskID: t.ID, WorkerID: workerID, Attempt: t.Attempt,
		Status: "running", StartedAt: nowUnix, CreatedAt: nowUnix,
	}); err != nil {
		return err
	}
	if _, err := s.store.MarkTaskRunning(ctx, t.ID); err != nil {
		return err
	}
	if _, err := s.audit(ctx, "execution", execID, "start", "runtime:"+workerID, ""); err != nil {
		return err
	}

	runCtx, cancel := runContext(ctx, t.TimeoutSec)
	defer cancel()

	res, execErr := tl.Execute(runCtx, t)
	finishAt := time.Now().Unix()

	if execErr != nil {
		msg := execErr.Error()
		if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
			msg = "timeout"
		}
		if _, err := s.store.FinishExecution(ctx, execID, "failed", &finishAt, "", msg); err != nil {
			return err
		}
		if _, err := s.audit(ctx, "execution", execID, "fail", "runtime:"+workerID, msg); err != nil {
			return err
		}
		if t.Attempt < t.MaxAttempts-1 {
			_, err := s.store.RequeueTask(ctx, t.ID, msg)
			return err
		}
		_, err := s.store.FailTask(ctx, t.ID, msg)
		return err
	}

	if _, err := s.store.CompleteTask(ctx, t.ID, res.Output); err != nil {
		return err
	}
	if _, err := s.store.FinishExecution(ctx, execID, "completed", &finishAt, res.Output, ""); err != nil {
		return err
	}
	if _, err := s.audit(ctx, "execution", execID, "complete", "runtime:"+workerID, ""); err != nil {
		return err
	}
	return nil
}

// authorizeTool 校验 Task 关联 Agent 的 Role 是否被授予对 tool 的权限。
// 无 Agent 或无匹配 allow policy → 拒绝(默认拒绝)。
func (s *Service) authorizeTool(ctx context.Context, t task.Task, tl tool.Tool) (bool, error) {
	if t.AgentID == nil {
		return false, nil
	}
	a, err := s.store.GetAgent(ctx, *t.AgentID)
	if err != nil {
		return false, err
	}
	perm := tl.Permission()
	n, err := s.store.CheckPermission(ctx, a.Role, perm.Action, perm.Resource)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func taskActor(t task.Task) string {
	if t.AgentID == nil {
		return "agent:none"
	}
	return "agent:" + *t.AgentID
}

func leaseUntil() int64 {
	return time.Now().Add(defaultLeaseMinutes * time.Minute).Unix()
}

func runContext(parent context.Context, timeoutSec int64) (context.Context, context.CancelFunc) {
	if timeoutSec > 0 {
		return context.WithTimeout(parent, time.Duration(timeoutSec)*time.Second)
	}
	return context.WithCancel(parent)
}
