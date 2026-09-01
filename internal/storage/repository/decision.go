package repository

import (
	"context"

	"github.com/glacierzzz26/one-person-company-os/internal/decision"
	"github.com/glacierzzz26/one-person-company-os/internal/storage/query"
)

func (s *Store) CreateDecision(ctx context.Context, d decision.Decision) (decision.Decision, error) {
	row, err := s.q.CreateDecision(ctx, query.CreateDecisionParams{
		ID: d.ID, CompanyID: d.CompanyID, Title: d.Title, Kind: d.Kind, Status: d.Status,
		Body: d.Body, DecidedBy: d.DecidedBy, Source: d.Source,
		CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt,
	})
	if err != nil {
		return decision.Decision{}, err
	}
	return toDecision(row), nil
}

func (s *Store) GetDecision(ctx context.Context, id string) (decision.Decision, error) {
	row, err := s.q.GetDecision(ctx, id)
	if err != nil {
		return decision.Decision{}, err
	}
	return toDecision(row), nil
}

func (s *Store) ListDecisions(ctx context.Context, companyID, kind string) ([]decision.Decision, error) {
	rows, err := s.q.ListDecisions(ctx, query.ListDecisionsParams{
		CompanyFilter: companyID,
		CompanyID:     companyID,
		KindFilter:    kind,
		Kind:          kind,
	})
	if err != nil {
		return nil, err
	}
	out := make([]decision.Decision, 0, len(rows))
	for _, r := range rows {
		out = append(out, toDecision(r))
	}
	return out, nil
}

func toDecision(r query.Decision) decision.Decision {
	return decision.Decision{
		ID: r.ID, CompanyID: r.CompanyID, Title: r.Title, Kind: r.Kind, Status: r.Status,
		Body: r.Body, DecidedBy: r.DecidedBy, Source: r.Source,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}
