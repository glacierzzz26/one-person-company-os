package repository

import (
	"context"

	"github.com/glacierzzz26/one-person-company-os/internal/permission"
	"github.com/glacierzzz26/one-person-company-os/internal/storage/query"
)

func (s *Store) CreatePermission(ctx context.Context, p permission.Permission) (permission.Permission, error) {
	row, err := s.q.CreatePermission(ctx, query.CreatePermissionParams{
		ID: p.ID, PolicyID: p.PolicyID, Subject: p.Subject, Action: p.Action,
		Resource: p.Resource, CreatedAt: p.CreatedAt,
	})
	if err != nil {
		return permission.Permission{}, err
	}
	return toPermission(row), nil
}

func (s *Store) ListPermissionsByPolicy(ctx context.Context, policyID string) ([]permission.Permission, error) {
	rows, err := s.q.ListPermissionsByPolicy(ctx, policyID)
	if err != nil {
		return nil, err
	}
	out := make([]permission.Permission, 0, len(rows))
	for _, r := range rows {
		out = append(out, toPermission(r))
	}
	return out, nil
}

// CheckPermission 返回 subject 在 enabled allow policy 下对 (action, resource) 的授权数。
// 0 表示未授权(默认拒绝)。
func (s *Store) CheckPermission(ctx context.Context, subject, action, resource string) (int64, error) {
	return s.q.CheckPermission(ctx, query.CheckPermissionParams{
		Subject: subject, Action: action, Resource: resource,
	})
}

func toPermission(r query.Permission) permission.Permission {
	return permission.Permission{
		ID: r.ID, PolicyID: r.PolicyID, Subject: r.Subject, Action: r.Action,
		Resource: r.Resource, CreatedAt: r.CreatedAt,
	}
}
