package repository

import (
	"context"

	"github.com/glacierzzz26/one-person-company-os/internal/company"
	"github.com/glacierzzz26/one-person-company-os/internal/storage/query"
)

func (s *Store) CreateCompany(ctx context.Context, c company.Company) (company.Company, error) {
	row, err := s.q.CreateCompany(ctx, query.CreateCompanyParams{
		ID: c.ID, Name: c.Name, Vision: c.Vision, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
	})
	if err != nil {
		return company.Company{}, err
	}
	return toCompany(row), nil
}

func (s *Store) GetCompany(ctx context.Context, id string) (company.Company, error) {
	row, err := s.q.GetCompany(ctx, id)
	if err != nil {
		return company.Company{}, err
	}
	return toCompany(row), nil
}

func (s *Store) ListCompanies(ctx context.Context) ([]company.Company, error) {
	rows, err := s.q.ListCompanies(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]company.Company, 0, len(rows))
	for _, r := range rows {
		out = append(out, toCompany(r))
	}
	return out, nil
}

func toCompany(r query.Company) company.Company {
	return company.Company{
		ID: r.ID, Name: r.Name, Vision: r.Vision,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}
