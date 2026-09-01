package service

import (
	"context"

	"github.com/glacierzzz26/one-person-company-os/internal/approval"
	"github.com/glacierzzz26/one-person-company-os/internal/company"
	"github.com/glacierzzz26/one-person-company-os/internal/decision"
	"github.com/glacierzzz26/one-person-company-os/internal/memory"
	"github.com/glacierzzz26/one-person-company-os/internal/task"
	"github.com/glacierzzz26/one-person-company-os/internal/workflow"
)

// Overview 全景运营视图(一人操作台):只读聚合,突出「需要人决策的事项」。
type Overview struct {
	Company          company.Company
	Capabilities     []CapabilityView    // code × agent 数
	Workflows        []WorkflowView      // 最近 workflow + 任务状态计数
	PendingApprovals []ApprovalView      // 所有 pending 审批 + 任务标题
	RecentDecisions  []decision.Decision // 最近 5 条
	RecentTasks      []task.Task         // 最近 8 条
	MemoryHighlights []memory.Memory     // 最近 5 条 lesson/knowledge
}

type CapabilityView struct {
	Code       string
	Name       string
	AgentCount int
}

type WorkflowView struct {
	Workflow workflow.Workflow
	Statuses map[string]int64 // status → count
}

type ApprovalView struct {
	Approval  approval.Approval
	TaskTitle string
}

func (s *Service) Overview(ctx context.Context, companyID string) (*Overview, error) {
	comp, err := s.store.GetCompany(ctx, companyID)
	if err != nil {
		return nil, err
	}
	ov := &Overview{Company: comp}

	caps, err := s.store.ListCapabilitiesByCompany(ctx, companyID)
	if err != nil {
		return nil, err
	}
	for _, c := range caps {
		agents, err := s.store.ListAgentsByCapability(ctx, c.ID)
		if err != nil {
			return nil, err
		}
		ov.Capabilities = append(ov.Capabilities, CapabilityView{Code: c.Code, Name: c.Name, AgentCount: len(agents)})
	}

	wfs, err := s.store.ListWorkflowsByCompany(ctx, companyID)
	if err != nil {
		return nil, err
	}
	counts, err := s.store.TaskStatusCountsByWorkflow(ctx, companyID)
	if err != nil {
		return nil, err
	}
	byWorkflow := map[string]map[string]int64{}
	for _, c := range counts {
		if byWorkflow[c.WorkflowID] == nil {
			byWorkflow[c.WorkflowID] = map[string]int64{}
		}
		byWorkflow[c.WorkflowID][c.Status] = c.Cnt
	}
	for _, w := range wfs {
		ov.Workflows = append(ov.Workflows, WorkflowView{Workflow: w, Statuses: byWorkflow[w.ID]})
	}

	apps, err := s.store.ListApprovals(ctx, "pending")
	if err != nil {
		return nil, err
	}
	for _, a := range apps {
		title := ""
		if t, err := s.store.GetTask(ctx, a.TaskID); err == nil {
			title = t.Title
		}
		ov.PendingApprovals = append(ov.PendingApprovals, ApprovalView{Approval: a, TaskTitle: title})
	}

	decisions, err := s.store.ListDecisions(ctx, companyID, "")
	if err != nil {
		return nil, err
	}
	ov.RecentDecisions = firstN(decisions, 5)

	recentTasks, err := s.store.ListRecentTasks(ctx, companyID, 8)
	if err != nil {
		return nil, err
	}
	ov.RecentTasks = recentTasks

	memories, err := s.store.ListMemories(ctx, companyID, "")
	if err != nil {
		return nil, err
	}
	ov.MemoryHighlights = firstN(memories, 5)

	return ov, nil
}

func firstN[T any](slice []T, n int) []T {
	if len(slice) > n {
		return slice[:n]
	}
	return slice
}
