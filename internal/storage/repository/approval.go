package repository

import (
	"context"

	"github.com/glacierzzz26/one-person-company-os/internal/approval"
	"github.com/glacierzzz26/one-person-company-os/internal/storage/query"
)

func (s *Store) CreateApproval(ctx context.Context, a approval.Approval) (approval.Approval, error) {
	row, err := s.q.CreateApproval(ctx, query.CreateApprovalParams{
		ID: a.ID, TaskID: a.TaskID, Risk: a.Risk, Reason: a.Reason,
		RequestedBy: a.RequestedBy, CreatedAt: a.CreatedAt,
	})
	if err != nil {
		return approval.Approval{}, err
	}
	return toApproval(row), nil
}

func (s *Store) GetApproval(ctx context.Context, id string) (approval.Approval, error) {
	row, err := s.q.GetApproval(ctx, id)
	if err != nil {
		return approval.Approval{}, err
	}
	return toApproval(row), nil
}

// HasApprovedApproval 返回 Task 是否已有被人工批准的审批记录(放行标记)。
func (s *Store) HasApprovedApproval(ctx context.Context, taskID string) (int64, error) {
	return s.q.HasApprovedApproval(ctx, taskID)
}

func (s *Store) ListApprovals(ctx context.Context, status string) ([]approval.Approval, error) {
	rows, err := s.q.ListApprovals(ctx, query.ListApprovalsParams{
		StatusFilter: status,
		Status:       status,
	})
	if err != nil {
		return nil, err
	}
	out := make([]approval.Approval, 0, len(rows))
	for _, r := range rows {
		out = append(out, toApproval(r))
	}
	return out, nil
}

func (s *Store) UpdateApproval(ctx context.Context, id, status, decidedBy, note string, decidedAt *int64) (approval.Approval, error) {
	row, err := s.q.UpdateApproval(ctx, query.UpdateApprovalParams{
		Status: status, DecidedBy: decidedBy, DecisionNote: note,
		DecidedAt: int64ToNull(decidedAt), ID: id,
	})
	if err != nil {
		return approval.Approval{}, err
	}
	return toApproval(row), nil
}

func toApproval(r query.Approval) approval.Approval {
	return approval.Approval{
		ID: r.ID, TaskID: r.TaskID, Risk: r.Risk, Reason: r.Reason,
		Status: r.Status, RequestedBy: r.RequestedBy, DecidedBy: r.DecidedBy,
		DecisionNote: r.DecisionNote, CreatedAt: r.CreatedAt,
		DecidedAt: nullToInt64Ptr(r.DecidedAt),
	}
}
