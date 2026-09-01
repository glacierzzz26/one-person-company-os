package repository

import (
	"context"

	"github.com/glacierzzz26/one-person-company-os/internal/storage/query"
	"github.com/glacierzzz26/one-person-company-os/internal/workflow"
)

func (s *Store) CreateWorkflow(ctx context.Context, w workflow.Workflow) (workflow.Workflow, error) {
	row, err := s.q.CreateWorkflow(ctx, query.CreateWorkflowParams{
		ID: w.ID, CompanyID: w.CompanyID, Name: w.Name,
		Description: w.Description, Definition: w.Definition,
		CreatedAt: w.CreatedAt, UpdatedAt: w.UpdatedAt,
	})
	if err != nil {
		return workflow.Workflow{}, err
	}
	return toWorkflow(row), nil
}

func (s *Store) GetWorkflow(ctx context.Context, id string) (workflow.Workflow, error) {
	row, err := s.q.GetWorkflow(ctx, id)
	if err != nil {
		return workflow.Workflow{}, err
	}
	return toWorkflow(row), nil
}

func (s *Store) ListWorkflowsByCompany(ctx context.Context, companyID string) ([]workflow.Workflow, error) {
	rows, err := s.q.ListWorkflowsByCompany(ctx, companyID)
	if err != nil {
		return nil, err
	}
	out := make([]workflow.Workflow, 0, len(rows))
	for _, r := range rows {
		out = append(out, toWorkflow(r))
	}
	return out, nil
}

func toWorkflow(r query.Workflow) workflow.Workflow {
	return workflow.Workflow{
		ID: r.ID, CompanyID: r.CompanyID, Name: r.Name,
		Description: r.Description, Definition: r.Definition,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}
