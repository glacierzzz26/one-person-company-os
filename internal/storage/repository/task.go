package repository

import (
	"context"
	"database/sql"

	"github.com/glacierzzz26/one-person-company-os/internal/storage/query"
	"github.com/glacierzzz26/one-person-company-os/internal/task"
)

func (s *Store) CreateTask(ctx context.Context, t task.Task) (task.Task, error) {
	row, err := s.q.CreateTask(ctx, query.CreateTaskParams{
		ID: t.ID, CompanyID: t.CompanyID,
		CapabilityID: ptrToNull(t.CapabilityID),
		WorkflowID:   ptrToNull(t.WorkflowID),
		AgentID:      ptrToNull(t.AgentID),
		Title: t.Title, Description: t.Description, Status: t.Status,
		Priority: t.Priority, Attempt: t.Attempt, Risk: t.Risk,
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

func (s *Store) ListTasks(ctx context.Context, companyFilter, statusFilter string) ([]task.Task, error) {
	rows, err := s.q.ListTasks(ctx, query.ListTasksParams{
		CompanyFilter: companyFilter,
		CompanyID:     companyFilter,
		StatusFilter:  statusFilter,
		Status:        statusFilter,
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

func toTask(r query.Task) task.Task {
	return task.Task{
		ID: r.ID, CompanyID: r.CompanyID,
		CapabilityID: nullToPtr(r.CapabilityID),
		WorkflowID:   nullToPtr(r.WorkflowID),
		AgentID:      nullToPtr(r.AgentID),
		Title: r.Title, Description: r.Description, Status: r.Status,
		Priority: r.Priority, Attempt: r.Attempt, Risk: r.Risk,
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
