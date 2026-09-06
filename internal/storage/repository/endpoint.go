package repository

import (
	"context"
	"time"

	"github.com/glacierzzz26/one-person-company-os/internal/endpoint"
	"github.com/glacierzzz26/one-person-company-os/internal/storage/query"
)

func (s *Store) CreateEndpoint(ctx context.Context, e endpoint.Endpoint) (endpoint.Endpoint, error) {
	// tier 未给(AddEndpoint 无 tier 参数等全调用点)→ 落列默认 standard:INSERT 现显式列 tier,
	// 空串会绕过 DB DEFAULT,故在 mapper 层把空档归一为 standard(8.4 §六「存量/未标档端点落中档」)。
	tier := e.Tier
	if tier == "" {
		tier = "standard"
	}
	row, err := s.q.CreateEndpoint(ctx, query.CreateEndpointParams{
		ID: e.ID, CompanyID: e.CompanyID, Name: e.Name, BaseUrl: e.BaseURL,
		TokenEnc: e.TokenEnc, Proto: e.Proto, Vendor: e.Vendor, SelectedModel: e.SelectedModel,
		Role: e.Role, Tier: tier, Status: e.Status, ModelsCache: e.ModelsCache,
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

func (s *Store) SetEndpointTier(ctx context.Context, id, tier string) (endpoint.Endpoint, error) {
	row, err := s.q.UpdateEndpointTier(ctx, query.UpdateEndpointTierParams{
		Tier: tier, UpdatedAt: time.Now().Unix(), ID: id,
	})
	if err != nil {
		return endpoint.Endpoint{}, err
	}
	return toEndpoint(row), nil
}

// SetEndpointToken 写回端点 token 密文(re-key / 主密钥换钥路径;raw cipher 直写,明文不出 repo)。
func (s *Store) SetEndpointToken(ctx context.Context, id, cipher string) (endpoint.Endpoint, error) {
	row, err := s.q.UpdateEndpointToken(ctx, query.UpdateEndpointTokenParams{
		TokenEnc: cipher, UpdatedAt: time.Now().Unix(), ID: id,
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
		Role: r.Role, Tier: r.Tier, Status: r.Status, ModelsCache: r.ModelsCache,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}
