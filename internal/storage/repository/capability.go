package repository

import (
	"context"

	"github.com/glacierzzz26/one-person-company-os/internal/capability"
	"github.com/glacierzzz26/one-person-company-os/internal/storage/query"
)

func (s *Store) CreateCapability(ctx context.Context, c capability.Capability) (capability.Capability, error) {
	row, err := s.q.CreateCapability(ctx, query.CreateCapabilityParams{
		ID: c.ID, CompanyID: c.CompanyID, Code: c.Code, Name: c.Name,
		Description: c.Description, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
	})
	if err != nil {
		return capability.Capability{}, err
	}
	return toCapability(row), nil
}

func (s *Store) GetCapability(ctx context.Context, id string) (capability.Capability, error) {
	row, err := s.q.GetCapability(ctx, id)
	if err != nil {
		return capability.Capability{}, err
	}
	return toCapability(row), nil
}

// GetCapabilityByCode 按 (company_id, code) 查 Capability。无匹配 → ErrNoRows。
func (s *Store) GetCapabilityByCode(ctx context.Context, companyID, code string) (capability.Capability, error) {
	row, err := s.q.GetCapabilityByCode(ctx, query.GetCapabilityByCodeParams{
		CompanyID: companyID, Code: code,
	})
	if err != nil {
		return capability.Capability{}, err
	}
	return toCapability(row), nil
}

func (s *Store) ListCapabilitiesByCompany(ctx context.Context, companyID string) ([]capability.Capability, error) {
	rows, err := s.q.ListCapabilitiesByCompany(ctx, companyID)
	if err != nil {
		return nil, err
	}
	out := make([]capability.Capability, 0, len(rows))
	for _, r := range rows {
		out = append(out, toCapability(r))
	}
	return out, nil
}

func toCapability(r query.Capability) capability.Capability {
	return capability.Capability{
		ID: r.ID, CompanyID: r.CompanyID, Code: r.Code, Name: r.Name,
		Description: r.Description, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}
