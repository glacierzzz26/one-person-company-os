package service

import (
	"context"

	"github.com/glacierzzz26/one-person-company-os/internal/audit"
)

func (s *Service) ListAudits(ctx context.Context, entityType string) ([]audit.Audit, error) {
	return s.store.ListAudits(ctx, entityType)
}
