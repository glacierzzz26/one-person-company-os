package service

import (
	"context"
	"fmt"
	"time"

	"github.com/glacierzzz26/one-person-company-os/internal/approval"
	"github.com/glacierzzz26/one-person-company-os/internal/task"
	"github.com/google/uuid"
)

// needsApproval 判定 Task 是否需人工审批:已有人工批准记录 → 放行不再触发;
// 否则 risk=high,或公司存在 enabled approval policy。方向基线 6.3:按风险策略触发。
func (s *Service) needsApproval(ctx context.Context, t task.Task) (bool, string, error) {
	approved, err := s.store.HasApprovedApproval(ctx, t.ID)
	if err != nil {
		return false, "", err
	}
	if approved > 0 {
		return false, "", nil
	}
	if t.Risk == "high" {
		return true, "risk=high", nil
	}
	pols, err := s.store.ListPoliciesByCompany(ctx, t.CompanyID)
	if err != nil {
		return false, "", err
	}
	for _, p := range pols {
		if p.Kind == "approval" && p.Enabled {
			return true, p.Statement, nil
		}
	}
	return false, "", nil
}

// requestApproval 创建 Approval 记录并把 Task 置为 waiting_approval(释放租约,不再入队)。
func (s *Service) requestApproval(ctx context.Context, t task.Task, reason string) error {
	requestedBy := "system"
	if t.AgentID != nil {
		requestedBy = *t.AgentID
	}
	nowUnix := time.Now().Unix()
	id := uuid.NewString()
	if _, err := s.store.CreateApproval(ctx, approval.Approval{
		ID: id, TaskID: t.ID, Risk: t.Risk, Reason: reason,
		RequestedBy: requestedBy, CreatedAt: nowUnix,
	}); err != nil {
		return err
	}
	if _, err := s.store.RequestApprovalTask(ctx, t.ID); err != nil {
		return err
	}
	if _, err := s.audit(ctx, "approval", t.ID, "request", taskActor(t), reason); err != nil {
		return err
	}
	// 事件点即时通知(6.5):熔断/planner ask/高险审批门都汇到此处。best-effort,失败只记日志。
	s.notifyApproval(ctx, t, id, reason)
	return nil
}

func (s *Service) ListApprovals(ctx context.Context, status string) ([]approval.Approval, error) {
	return s.store.ListApprovals(ctx, status)
}

func (s *Service) GetApproval(ctx context.Context, id string) (approval.Approval, error) {
	return s.store.GetApproval(ctx, id)
}

// DecideApproval 由人工(actor=human:cli)对 pending 审批作出决定。
// approve → Task 重新入队;reject / changes → Task failed(含备注)。
func (s *Service) DecideApproval(ctx context.Context, id, decision, note string) (approval.Approval, error) {
	return s.DecideApprovalAs(ctx, id, decision, note, "human:cli")
}

// DecideApprovalAs 同 DecideApproval,actor 标来源(human:cli 命令行 / human:console Web 控制台)。
func (s *Service) DecideApprovalAs(ctx context.Context, id, decision, note, actor string) (approval.Approval, error) {
	a, err := s.store.GetApproval(ctx, id)
	if err != nil {
		return approval.Approval{}, err
	}
	if a.Status != "pending" {
		return approval.Approval{}, fmt.Errorf("approval %s already %s", id, a.Status)
	}
	switch decision {
	case "approve":
		if _, err := s.store.ApproveTask(ctx, a.TaskID); err != nil {
			return approval.Approval{}, err
		}
	case "reject":
		if _, err := s.store.FailTask(ctx, a.TaskID, "rejected by human"); err != nil {
			return approval.Approval{}, err
		}
	case "changes":
		if _, err := s.store.FailTask(ctx, a.TaskID, "changes requested: "+note); err != nil {
			return approval.Approval{}, err
		}
	default:
		return approval.Approval{}, fmt.Errorf("unknown decision %q", decision)
	}
	status := decision
	if status == "approve" {
		status = "approved"
	}
	if status == "reject" {
		status = "rejected"
	}
	decidedAt := time.Now().Unix()
	updated, err := s.store.UpdateApproval(ctx, id, status, actor, note, &decidedAt)
	if err != nil {
		return approval.Approval{}, err
	}
	_, err = s.audit(ctx, "approval", id, decision, actor, note)
	if err != nil {
		return updated, err
	}
	// 自动记录 Decision(治理链闭环):取 Task 拿 company_id 与标题,失败即报错暴露不一致。
	t, err := s.store.GetTask(ctx, a.TaskID)
	if err != nil {
		return updated, err
	}
	if err := s.recordApprovalDecision(ctx, t.CompanyID, "审批 "+status+": "+t.Title, note, id, actor); err != nil {
		return updated, err
	}
	return updated, nil
}
