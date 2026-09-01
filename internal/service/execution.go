package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/glacierzzz26/one-person-company-os/internal/execution"
	"github.com/glacierzzz26/one-person-company-os/internal/runtime"
	"github.com/glacierzzz26/one-person-company-os/internal/task"
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

func (s *Service) runClaimed(ctx context.Context, workerID string, t task.Task) error {
	rt, ok := runtime.Get("shell")
	if !ok {
		return errors.New("runtime 'shell' not registered")
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

	res, execErr := rt.Execute(runCtx, t)
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

func leaseUntil() int64 {
	return time.Now().Add(defaultLeaseMinutes * time.Minute).Unix()
}

func runContext(parent context.Context, timeoutSec int64) (context.Context, context.CancelFunc) {
	if timeoutSec > 0 {
		return context.WithTimeout(parent, time.Duration(timeoutSec)*time.Second)
	}
	return context.WithCancel(parent)
}
