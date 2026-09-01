package repository

import (
	"context"
	"database/sql"

	"github.com/glacierzzz26/one-person-company-os/internal/execution"
	"github.com/glacierzzz26/one-person-company-os/internal/storage/query"
)

func (s *Store) CreateExecution(ctx context.Context, e execution.Execution) (execution.Execution, error) {
	row, err := s.q.CreateExecution(ctx, query.CreateExecutionParams{
		ID: e.ID, TaskID: e.TaskID, WorkerID: e.WorkerID, Attempt: e.Attempt,
		Status: e.Status, StartedAt: e.StartedAt, CreatedAt: e.CreatedAt,
	})
	if err != nil {
		return execution.Execution{}, err
	}
	return toExecution(row), nil
}

func (s *Store) GetExecution(ctx context.Context, id string) (execution.Execution, error) {
	row, err := s.q.GetExecution(ctx, id)
	if err != nil {
		return execution.Execution{}, err
	}
	return toExecution(row), nil
}

func (s *Store) ListExecutions(ctx context.Context, taskID, status string) ([]execution.Execution, error) {
	rows, err := s.q.ListExecutions(ctx, query.ListExecutionsParams{
		TaskFilter:   taskID,
		TaskID:       taskID,
		StatusFilter: status,
		Status:       status,
	})
	if err != nil {
		return nil, err
	}
	out := make([]execution.Execution, 0, len(rows))
	for _, r := range rows {
		out = append(out, toExecution(r))
	}
	return out, nil
}

func (s *Store) FinishExecution(ctx context.Context, id, status string, finishedAt *int64, result, execErr string) (execution.Execution, error) {
	row, err := s.q.FinishExecution(ctx, query.FinishExecutionParams{
		Status: status, FinishedAt: int64ToNull(finishedAt), Result: result, Error: execErr, ID: id,
	})
	if err != nil {
		return execution.Execution{}, err
	}
	return toExecution(row), nil
}

func toExecution(r query.Execution) execution.Execution {
	return execution.Execution{
		ID: r.ID, TaskID: r.TaskID, WorkerID: r.WorkerID, Attempt: r.Attempt,
		Status: r.Status, StartedAt: r.StartedAt,
		FinishedAt: nullToInt64Ptr(r.FinishedAt),
		Result:     r.Result, Error: r.Error, CreatedAt: r.CreatedAt,
	}
}

func nullToInt64Ptr(n sql.NullInt64) *int64 {
	if !n.Valid {
		return nil
	}
	v := n.Int64
	return &v
}

func int64ToNull(p *int64) sql.NullInt64 {
	if p == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *p, Valid: true}
}
