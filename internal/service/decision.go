package service

import (
	"context"
	"time"

	"github.com/glacierzzz26/one-person-company-os/internal/decision"
	"github.com/google/uuid"
)

// CreateDecision 人工记录一条公司级决策(goal/strategy/policy_change/capital/manual)。
func (s *Service) CreateDecision(ctx context.Context, companyID, kind, status, title, body string) (decision.Decision, error) {
	return s.CreateDecisionAs(ctx, companyID, kind, status, title, body, "human:cli")
}

// CreateDecisionAs 同 CreateDecision,decidedBy 与审计 actor 用传入值(如 human:console)。
func (s *Service) CreateDecisionAs(ctx context.Context, companyID, kind, status, title, body, actor string) (decision.Decision, error) {
	now := time.Now().Unix()
	d := decision.Decision{
		ID: uuid.NewString(), CompanyID: companyID, Title: title, Kind: kind, Status: status,
		Body: body, DecidedBy: actor, Source: "manual", CreatedAt: now, UpdatedAt: now,
	}
	created, err := s.store.CreateDecision(ctx, d)
	if err != nil {
		return decision.Decision{}, err
	}
	_, err = s.audit(ctx, "decision", created.ID, "create", actor, kind)
	return created, err
}

func (s *Service) ListDecisions(ctx context.Context, companyID, kind string) ([]decision.Decision, error) {
	return s.store.ListDecisions(ctx, companyID, kind)
}

func (s *Service) GetDecision(ctx context.Context, id string) (decision.Decision, error) {
	return s.store.GetDecision(ctx, id)
}

// recordApprovalDecision 审批决定落地后自动留痕(kind=approval,source=approval:<id>)。
// 由 DecideApproval(As) 调用,治理链闭环 Policy→Permission→Approval→Audit→Decision。
// decidedBy 记录决策来源(human:cli / human:console);审计 actor 固定 system:approval(系统自动留痕)。
func (s *Service) recordApprovalDecision(ctx context.Context, companyID, title, note, approvalID, decidedBy string) error {
	now := time.Now().Unix()
	created, err := s.store.CreateDecision(ctx, decision.Decision{
		ID: uuid.NewString(), CompanyID: companyID, Title: title,
		Kind: "approval", Status: "made", Body: note,
		DecidedBy: decidedBy, Source: "approval:" + approvalID,
		CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		return err
	}
	_, err = s.audit(ctx, "decision", created.ID, "create", "system:approval", "approval:"+approvalID)
	return err
}
