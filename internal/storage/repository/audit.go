package repository

import (
	"context"

	"github.com/glacierzzz26/one-person-company-os/internal/audit"
	"github.com/glacierzzz26/one-person-company-os/internal/storage/query"
)

func (s *Store) CreateAudit(ctx context.Context, a audit.Audit) (audit.Audit, error) {
	row, err := s.q.CreateAudit(ctx, query.CreateAuditParams{
		ID: a.ID, EntityType: a.EntityType, EntityID: a.EntityID,
		Action: a.Action, Actor: a.Actor, Detail: a.Detail, CreatedAt: a.CreatedAt,
	})
	if err != nil {
		return audit.Audit{}, err
	}
	return toAudit(row), nil
}

func (s *Store) ListAudits(ctx context.Context, entityType string) ([]audit.Audit, error) {
	rows, err := s.q.ListAudits(ctx, query.ListAuditsParams{
		EntityTypeFilter: entityType,
		EntityType:       entityType,
	})
	if err != nil {
		return nil, err
	}
	out := make([]audit.Audit, 0, len(rows))
	for _, r := range rows {
		out = append(out, toAudit(r))
	}
	return out, nil
}

// HasAuditAction 判定实体是否已存在某 action 的审计记录(Phase 8.2 委派前置用:
// 区分「任务从未委派(认领起点须 clean)」与「工作树残留 = 本任务先前委派产物」)。
// 手写参数化查询(不经 sqlc),表结构照 sqlc listAudits。
func (s *Store) HasAuditAction(ctx context.Context, entityType, entityID, action string) (bool, error) {
	const sqlq = `SELECT COUNT(1) FROM audit WHERE entity_type = ? AND entity_id = ? AND "action" = ?`
	var n int
	if err := s.db.QueryRowContext(ctx, sqlq, entityType, entityID, action).Scan(&n); err != nil {
		return false, err
	}
	return n > 0, nil
}

func toAudit(r query.Audit) audit.Audit {
	return audit.Audit{
		ID: r.ID, EntityType: r.EntityType, EntityID: r.EntityID,
		Action: r.Action, Actor: r.Actor, Detail: r.Detail, CreatedAt: r.CreatedAt,
	}
}
