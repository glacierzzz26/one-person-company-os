package service

import (
	"context"
	"time"

	"github.com/glacierzzz26/one-person-company-os/internal/task"
	"github.com/google/uuid"
)

type TaskParams struct {
	CompanyID    string
	CapabilityID *string
	WorkflowID   *string
	AgentID      *string
	Title        string
	Description  string
	ToolName     string
	Risk         string
	MaxAttempts  int64
	TimeoutSec   int64
	Workspace    string
	// Phase 6.2:Engineering 回合字段(可选)。
	ParentTaskID       *string
	WriterEndpointID   *string
	ReviewerEndpointID *string
}

func (s *Service) CreateTask(ctx context.Context, p TaskParams) (task.Task, error) {
	return s.createTask(ctx, p, "human:cli")
}

// CreateTaskAs 同 CreateTask,actor 标来源(human:console = Web 控制台;CLI 用 CreateTask)。
func (s *Service) CreateTaskAs(ctx context.Context, p TaskParams, actor string) (task.Task, error) {
	return s.createTask(ctx, p, actor)
}

// createTask 创建任务并落 Audit。actor 区分来源:human:cli(命令)/ intake(研发 Intake 通道 B)。
func (s *Service) createTask(ctx context.Context, p TaskParams, actor string) (task.Task, error) {
	if p.Risk == "" {
		p.Risk = "low"
	}
	if p.ToolName == "" {
		p.ToolName = "shell"
	}
	if p.MaxAttempts == 0 {
		p.MaxAttempts = 1
	}
	now := time.Now().Unix()
	t := task.Task{
		ID: uuid.NewString(), CompanyID: p.CompanyID,
		CapabilityID: p.CapabilityID, WorkflowID: p.WorkflowID, AgentID: p.AgentID,
		Title: p.Title, Description: p.Description, ToolName: p.ToolName,
		Status: "pending", Priority: 0, Attempt: 0, Risk: p.Risk,
		QStatus: "ready", MaxAttempts: p.MaxAttempts, TimeoutSec: p.TimeoutSec,
		WorkspacePath:      p.Workspace,
		ParentTaskID:       p.ParentTaskID,
		WriterEndpointID:   p.WriterEndpointID,
		ReviewerEndpointID: p.ReviewerEndpointID,
		CreatedAt:          now, UpdatedAt: now,
	}
	created, err := s.store.CreateTask(ctx, t)
	if err != nil {
		return task.Task{}, err
	}
	_, err = s.audit(ctx, "task", created.ID, "create", actor, "")
	return created, err
}

func (s *Service) ListTasks(ctx context.Context, companyID, status, risk string, attemptMin int64) ([]task.Task, error) {
	return s.store.ListTasks(ctx, companyID, status, risk, attemptMin)
}

func (s *Service) GetTask(ctx context.Context, id string) (task.Task, error) {
	return s.store.GetTask(ctx, id)
}
