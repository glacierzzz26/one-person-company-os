package repository

import (
	"context"

	"github.com/glacierzzz26/one-person-company-os/internal/agent"
	"github.com/glacierzzz26/one-person-company-os/internal/storage/query"
)

func (s *Store) CreateAgent(ctx context.Context, a agent.Agent) (agent.Agent, error) {
	row, err := s.q.CreateAgent(ctx, query.CreateAgentParams{
		ID: a.ID, CapabilityID: a.CapabilityID, Name: a.Name, Role: a.Role,
		ModelHint: a.ModelHint, CreatedAt: a.CreatedAt, UpdatedAt: a.UpdatedAt,
	})
	if err != nil {
		return agent.Agent{}, err
	}
	return toAgent(row), nil
}

func (s *Store) GetAgent(ctx context.Context, id string) (agent.Agent, error) {
	row, err := s.q.GetAgent(ctx, id)
	if err != nil {
		return agent.Agent{}, err
	}
	return toAgent(row), nil
}

func (s *Store) ListAgentsByCapability(ctx context.Context, capabilityID string) ([]agent.Agent, error) {
	rows, err := s.q.ListAgentsByCapability(ctx, capabilityID)
	if err != nil {
		return nil, err
	}
	out := make([]agent.Agent, 0, len(rows))
	for _, r := range rows {
		out = append(out, toAgent(r))
	}
	return out, nil
}

// GetAgentByRole 返回公司下该 role 最早创建的 Agent。无匹配 → ErrNoRows。
func (s *Store) GetAgentByRole(ctx context.Context, companyID, role string) (agent.Agent, error) {
	row, err := s.q.GetAgentByRole(ctx, query.GetAgentByRoleParams{
		CompanyID: companyID, Role: role,
	})
	if err != nil {
		return agent.Agent{}, err
	}
	return toAgent(row), nil
}

func toAgent(r query.Agent) agent.Agent {
	return agent.Agent{
		ID: r.ID, CapabilityID: r.CapabilityID, Name: r.Name, Role: r.Role,
		ModelHint: r.ModelHint, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}
