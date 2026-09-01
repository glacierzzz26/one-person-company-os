package service

import (
	"context"
	"time"

	"github.com/glacierzzz26/one-person-company-os/internal/permission"
	"github.com/glacierzzz26/one-person-company-os/internal/policy"
	"github.com/google/uuid"
)

func (s *Service) CreatePolicy(ctx context.Context, companyID, name, kind, statement string) (policy.Policy, error) {
	now := time.Now().Unix()
	p := policy.Policy{
		ID: uuid.NewString(), CompanyID: companyID, Name: name, Kind: kind,
		Statement: statement, Enabled: true, CreatedAt: now, UpdatedAt: now,
	}
	created, err := s.store.CreatePolicy(ctx, p)
	if err != nil {
		return policy.Policy{}, err
	}
	_, err = s.audit(ctx, "policy", created.ID, "create", "human:cli", "")
	return created, err
}

func (s *Service) ListPolicies(ctx context.Context, companyID string) ([]policy.Policy, error) {
	return s.store.ListPoliciesByCompany(ctx, companyID)
}

func (s *Service) CreatePermission(ctx context.Context, policyID, subject, action, resource string) (permission.Permission, error) {
	p := permission.Permission{
		ID: uuid.NewString(), PolicyID: policyID, Subject: subject, Action: action,
		Resource: resource, CreatedAt: time.Now().Unix(),
	}
	created, err := s.store.CreatePermission(ctx, p)
	if err != nil {
		return permission.Permission{}, err
	}
	_, err = s.audit(ctx, "permission", created.ID, "create", "human:cli", "")
	return created, err
}

func (s *Service) ListPermissions(ctx context.Context, policyID string) ([]permission.Permission, error) {
	return s.store.ListPermissionsByPolicy(ctx, policyID)
}
