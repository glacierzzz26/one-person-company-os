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
	Risk         string
}

func (s *Service) CreateTask(ctx context.Context, p TaskParams) (task.Task, error) {
	if p.Risk == "" {
		p.Risk = "low"
	}
	now := time.Now().Unix()
	t := task.Task{
		ID: uuid.NewString(), CompanyID: p.CompanyID,
		CapabilityID: p.CapabilityID, WorkflowID: p.WorkflowID, AgentID: p.AgentID,
		Title: p.Title, Description: p.Description,
		Status: "pending", Priority: 0, Attempt: 0, Risk: p.Risk,
		CreatedAt: now, UpdatedAt: now,
	}
	created, err := s.store.CreateTask(ctx, t)
	if err != nil {
		return task.Task{}, err
	}
	_, err = s.audit(ctx, "task", created.ID, "create", "human:cli", "")
	return created, err
}

func (s *Service) ListTasks(ctx context.Context, companyID, status string) ([]task.Task, error) {
	return s.store.ListTasks(ctx, companyID, status)
}

func (s *Service) GetTask(ctx context.Context, id string) (task.Task, error) {
	return s.store.GetTask(ctx, id)
}
