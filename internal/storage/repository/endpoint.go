package repository

import (
	"context"
	"time"

	"github.com/glacierzzz26/one-person-company-os/internal/endpoint"
	"github.com/glacierzzz26/one-person-company-os/internal/storage/query"
)

func (s *Store) CreateEndpoint(ctx context.Context, e endpoint.Endpoint) (endpoint.Endpoint, error) {
	row, err := s.q.CreateEndpoint(ctx, query.CreateEndpointParams{
		ID: e.ID, CompanyID: e.CompanyID, Name: e.Name, BaseUrl: e.BaseURL,
		TokenEnc: e.TokenEnc, Proto: e.Proto, Vendor: e.Vendor, SelectedModel: e.SelectedModel,
		Role: e.Role, Status: e.Status, ModelsCache: e.ModelsCache,
		CreatedAt: e.CreatedAt, UpdatedAt: e.UpdatedAt,
	})
	if err != nil {
		return endpoint.Endpoint{}, err
	}
	return toEndpoint(row), nil
}

func (s *Store) GetEndpoint(ctx context.Context, id string) (endpoint.Endpoint, error) {
	row, err := s.q.GetEndpoint(ctx, id)
	if err != nil {
		return endpoint.Endpoint{}, err
	}
	return toEndpoint(row), nil
}

func (s *Store) ListEndpoints(ctx context.Context, companyID string) ([]endpoint.Endpoint, error) {
	rows, err := s.q.ListEndpoints(ctx, companyID)
	if err != nil {
		return nil, err
	}
	out := make([]endpoint.Endpoint, 0, len(rows))
	for _, r := range rows {
		out = append(out, toEndpoint(r))
	}
	return out, nil
}

func (s *Store) SetEndpointModel(ctx context.Context, id, model, cache string) (endpoint.Endpoint, error) {
	row, err := s.q.UpdateEndpointModel(ctx, query.UpdateEndpointModelParams{
		SelectedModel: model, ModelsCache: cache, UpdatedAt: time.Now().Unix(), ID: id,
	})
	if err != nil {
		return endpoint.Endpoint{}, err
	}
	return toEndpoint(row), nil
}

func (s *Store) SetEndpointRole(ctx context.Context, id, role string) (endpoint.Endpoint, error) {
	row, err := s.q.UpdateEndpointRole(ctx, query.UpdateEndpointRoleParams{
		Role: role, UpdatedAt: time.Now().Unix(), ID: id,
	})
	if err != nil {
		return endpoint.Endpoint{}, err
	}
	return toEndpoint(row), nil
}

func toEndpoint(r query.Endpoint) endpoint.Endpoint {
	return endpoint.Endpoint{
		ID: r.ID, CompanyID: r.CompanyID, Name: r.Name, BaseURL: r.BaseUrl,
		TokenEnc: r.TokenEnc, Proto: r.Proto, Vendor: r.Vendor, SelectedModel: r.SelectedModel,
		Role: r.Role, Status: r.Status, ModelsCache: r.ModelsCache,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}
