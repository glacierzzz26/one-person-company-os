package repository

import (
	"context"

	"github.com/glacierzzz26/one-person-company-os/internal/audit"
	"github.com/glacierzzz26/one-person-company-os/internal/storage/query"
)

func (s *Store) CreateAudit(ctx context.Context, a audit.Audit) (audit.Audit, error) {
	row, err := s.q.CreateAudit(ctx, query.CreateAuditParams{
		ID: a.ID, EntityType: a.EntityType, EntityID: a.EntityID,
		Action: a.Action, Actor: a.Actor, Detail: a.Detail, CreatedAt: a.CreatedAt,
	})
	if err != nil {
		return audit.Audit{}, err
	}
	return toAudit(row), nil
}

func (s *Store) ListAudits(ctx context.Context, entityType string) ([]audit.Audit, error) {
	rows, err := s.q.ListAudits(ctx, query.ListAuditsParams{
		EntityTypeFilter: entityType,
		EntityType:       entityType,
	})
	if err != nil {
		return nil, err
	}
	out := make([]audit.Audit, 0, len(rows))
	for _, r := range rows {
		out = append(out, toAudit(r))
	}
	return out, nil
}

func toAudit(r query.Audit) audit.Audit {
	return audit.Audit{
		ID: r.ID, EntityType: r.EntityType, EntityID: r.EntityID,
		Action: r.Action, Actor: r.Actor, Detail: r.Detail, CreatedAt: r.CreatedAt,
	}
}
