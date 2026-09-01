package repository

import (
	"context"

	"github.com/glacierzzz26/one-person-company-os/internal/policy"
	"github.com/glacierzzz26/one-person-company-os/internal/storage/query"
)

func (s *Store) CreatePolicy(ctx context.Context, p policy.Policy) (policy.Policy, error) {
	enabled := int64(0)
	if p.Enabled {
		enabled = 1
	}
	row, err := s.q.CreatePolicy(ctx, query.CreatePolicyParams{
		ID: p.ID, CompanyID: p.CompanyID, Name: p.Name, Kind: p.Kind,
		Statement: p.Statement, Enabled: enabled, CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
	})
	if err != nil {
		return policy.Policy{}, err
	}
	return toPolicy(row), nil
}

func (s *Store) GetPolicy(ctx context.Context, id string) (policy.Policy, error) {
	row, err := s.q.GetPolicy(ctx, id)
	if err != nil {
		return policy.Policy{}, err
	}
	return toPolicy(row), nil
}

func (s *Store) ListPoliciesByCompany(ctx context.Context, companyID string) ([]policy.Policy, error) {
	rows, err := s.q.ListPoliciesByCompany(ctx, companyID)
	if err != nil {
		return nil, err
	}
	out := make([]policy.Policy, 0, len(rows))
	for _, r := range rows {
		out = append(out, toPolicy(r))
	}
	return out, nil
}

func toPolicy(r query.Policy) policy.Policy {
	return policy.Policy{
		ID: r.ID, CompanyID: r.CompanyID, Name: r.Name, Kind: r.Kind,
		Statement: r.Statement, Enabled: r.Enabled != 0,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}
