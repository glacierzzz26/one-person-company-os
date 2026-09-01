package service

import (
	"context"
	"time"

	"github.com/glacierzzz26/one-person-company-os/internal/agent"
	"github.com/glacierzzz26/one-person-company-os/internal/capability"
	"github.com/glacierzzz26/one-person-company-os/internal/workflow"
	"github.com/google/uuid"
)

func (s *Service) CreateCapability(ctx context.Context, companyID, code, name, description string) (capability.Capability, error) {
	now := time.Now().Unix()
	c := capability.Capability{
		ID: uuid.NewString(), CompanyID: companyID, Code: code, Name: name,
		Description: description, CreatedAt: now, UpdatedAt: now,
	}
	created, err := s.store.CreateCapability(ctx, c)
	if err != nil {
		return capability.Capability{}, err
	}
	_, err = s.audit(ctx, "capability", created.ID, "create", "human:cli", "")
	return created, err
}

func (s *Service) ListCapabilities(ctx context.Context, companyID string) ([]capability.Capability, error) {
	return s.store.ListCapabilitiesByCompany(ctx, companyID)
}

func (s *Service) CreateAgent(ctx context.Context, capabilityID, name, role, modelHint string) (agent.Agent, error) {
	now := time.Now().Unix()
	a := agent.Agent{
		ID: uuid.NewString(), CapabilityID: capabilityID, Name: name, Role: role,
		ModelHint: modelHint, CreatedAt: now, UpdatedAt: now,
	}
	created, err := s.store.CreateAgent(ctx, a)
	if err != nil {
		return agent.Agent{}, err
	}
	_, err = s.audit(ctx, "agent", created.ID, "create", "human:cli", "")
	return created, err
}

func (s *Service) ListAgents(ctx context.Context, capabilityID string) ([]agent.Agent, error) {
	return s.store.ListAgentsByCapability(ctx, capabilityID)
}

func (s *Service) CreateWorkflow(ctx context.Context, companyID, name, description, definition string) (workflow.Workflow, error) {
	now := time.Now().Unix()
	w := workflow.Workflow{
		ID: uuid.NewString(), CompanyID: companyID, Name: name,
		Description: description, Definition: definition, CreatedAt: now, UpdatedAt: now,
	}
	created, err := s.store.CreateWorkflow(ctx, w)
	if err != nil {
		return workflow.Workflow{}, err
	}
	_, err = s.audit(ctx, "workflow", created.ID, "create", "human:cli", "")
	return created, err
}

func (s *Service) ListWorkflows(ctx context.Context, companyID string) ([]workflow.Workflow, error) {
	return s.store.ListWorkflowsByCompany(ctx, companyID)
}
