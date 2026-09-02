package service

import (
	"context"
	"strings"

	"github.com/glacierzzz26/one-person-company-os/internal/approval"
	"github.com/glacierzzz26/one-person-company-os/internal/capability"
	"github.com/glacierzzz26/one-person-company-os/internal/company"
	"github.com/glacierzzz26/one-person-company-os/internal/decision"
	"github.com/glacierzzz26/one-person-company-os/internal/memory"
	"github.com/glacierzzz26/one-person-company-os/internal/task"
	"github.com/glacierzzz26/one-person-company-os/internal/workflow"
)

// Overview 全景运营视图(一人操作台):只读聚合,突出「需要人决策的事项」。json tag = /api/v1 契约(phase7)。
type Overview struct {
	Company          company.Company     `json:"company"`
	Capabilities     []CapabilityView    `json:"capabilities"`      // code × agent 数
	Workflows        []WorkflowView      `json:"workflows"`         // 最近 workflow + 任务状态计数
	PendingApprovals []ApprovalView      `json:"pending_approvals"` // 所有 pending 审批 + 任务标题
	RecentDecisions  []decision.Decision `json:"recent_decisions"`  // 最近 5 条
	RecentTasks      []task.Task         `json:"recent_tasks"`      // 最近 8 条
	MemoryHighlights []memory.Memory     `json:"memory_highlights"` // 最近 5 条 lesson/knowledge
	RD               *RDOverview         `json:"rd"`                // 研发部(engineering Capability 部门视图,6.5);无 engineering → nil
}

type CapabilityView struct {
	Code       string `json:"code"`
	Name       string `json:"name"`
	AgentCount int    `json:"agent_count"`
}

type WorkflowView struct {
	Workflow workflow.Workflow `json:"workflow"`
	Statuses map[string]int64  `json:"statuses"` // status → count
}

type ApprovalView struct {
	Approval  approval.Approval `json:"approval"`
	TaskTitle string            `json:"task_title"`
}

// RDOverview 研发部状态聚合(6.5):engineering Capability 部门视图。
// RD 任务 = capability=engineering 或 tool=engineering(拆解子任务继承 capability,天然落入)。
type RDOverview struct {
	CapabilityID string           `json:"capability_id"`
	Total        int              `json:"total"`         // RD 任务总数
	ByStatus     map[string]int64 `json:"by_status"`     // status → count
	Fused        []RDTask         `json:"fused"`         // 熔断:评审驳回达阈值转人工(waiting_approval)
	Waiting      []RDTask         `json:"waiting"`       // 其余待审批(waiting_approval,ask/高险门)
	SubtaskCount int              `json:"subtask_count"` // parent_task_id 非空子任务数
	Ledger       map[string]int64 `json:"ledger"`        // 通道 B issue_sync 账本 disposition 分布(处置分布/去重计数)
	LedgerSeen   int              `json:"ledger_seen"`   // 账本总条数(=已同步 issue 去重计数)
}

// RDTask 研发任务行视图。
type RDTask struct {
	ID           string  `json:"id"`
	Title        string  `json:"title"`
	Status       string  `json:"status"`
	Risk         string  `json:"risk"`
	Round        int64   `json:"round"`
	Conflict     int64   `json:"conflict"`
	ParentTaskID *string `json:"parent_task_id"`
	Reason       string  `json:"reason"` // waiting_approval 原因(熔断/ask/高险门)
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

	// 研发部聚合(6.5):engineering Capability 部门视图。数据全部复用既有查询,Go 内过滤。
	rd, err := s.buildRD(ctx, companyID, caps, apps)
	if err != nil {
		return nil, err
	}
	ov.RD = rd

	return ov, nil
}

// buildRD 聚合研发部状态。caps 定位 engineering Capability;apps 为全部 pending 审批
// (已在前文取到),按 task_id 反查 waiting_approval 原因。
func (s *Service) buildRD(ctx context.Context, companyID string, caps []capability.Capability, apps []approval.Approval) (*RDOverview, error) {
	engCapID := ""
	for _, c := range caps {
		if c.Code == "engineering" {
			engCapID = c.ID
			break
		}
	}
	if engCapID == "" {
		return nil, nil
	}

	// 无 capability/tool 过滤的现成查询 → company 全量任务后 Go 内过滤(single-operator 量级可接受)。
	all, err := s.store.ListTasks(ctx, companyID, "", "", 0)
	if err != nil {
		return nil, err
	}
	// pending 审批按 task_id 索引(熔断/ask/高险门原因)。
	reasonByTask := map[string]string{}
	for _, a := range apps {
		if a.Status == "pending" {
			reasonByTask[a.TaskID] = a.Reason
		}
	}

	rd := &RDOverview{
		CapabilityID: engCapID,
		ByStatus:     map[string]int64{},
		Fused:        []RDTask{},
		Waiting:      []RDTask{},
	}
	for _, t := range all {
		isRD := t.ToolName == "engineering" || (t.CapabilityID != nil && *t.CapabilityID == engCapID)
		if !isRD {
			continue
		}
		rd.Total++
		rd.ByStatus[t.Status]++
		if t.ParentTaskID != nil {
			rd.SubtaskCount++
		}
		if t.Status != "waiting_approval" {
			continue
		}
		view := RDTask{
			ID: t.ID, Title: t.Title, Status: t.Status, Risk: t.Risk,
			Round: t.RoundNo, Conflict: t.ConflictCount, ParentTaskID: t.ParentTaskID,
			Reason: reasonByTask[t.ID],
		}
		if view.Reason == "" && t.ConflictCount >= engFuseMax {
			view.Reason = "engineering fuse (no pending approval row)"
		}
		// 熔断 = 冲突达阈值,或 waiting 原因带 engineering fuse;否则归入一般待审批。
		if t.ConflictCount >= engFuseMax || strings.HasPrefix(view.Reason, "engineering fuse") {
			rd.Fused = append(rd.Fused, view)
		} else {
			rd.Waiting = append(rd.Waiting, view)
		}
	}

	// 通道 B 账本回放:issue_sync 处置分布(去重计数 = 总条数)。
	ledger, err := s.store.ListIssueSync(ctx, companyID)
	if err != nil {
		return nil, err
	}
	rd.Ledger = map[string]int64{}
	for _, is := range ledger {
		rd.Ledger[is.Disposition]++
	}
	rd.LedgerSeen = len(ledger)

	return rd, nil
}

func firstN[T any](slice []T, n int) []T {
	if len(slice) > n {
		return slice[:n]
	}
	return slice
}
