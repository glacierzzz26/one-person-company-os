package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/glacierzzz26/one-person-company-os/internal/task"
)

// Engineering Driver 的 planner 拆解序曲(Phase 6.4,design §10 6.4)。
// 通道 A(workflow engineering 节点)与通道 B(merge 批次 umbrella)整包请求最终都落到
// driver;认领后先过 planner:
//
//	direct → 原子项,直接落入既有回合机(行为同 6.2,无额外建任务)
//	split  → ≤8 条真实子任务(挂 parent_task_id),同步驱动全部完成后聚合父任务
//	ask    → 超 8 / 边界不清 → 人工审批(无 bypass 开关)
//
// 拆解深度有界:仅顶层请求(parent_task_id 为空)可拆,子任务即执行单元不再递归拆解;
// 子任务已存在(租约中断续跑)→ 驱动现存子任务聚合,不重复拆;已有人工批准记录
// (high-risk 预审通过 / planner-ask 被 approve)→ 请求按原样执行,不重复问。
// runEngineering 以 !humanOverride 为前置(熔断续跑不再拆),故此处不再复检。

// planEngineering 返回 handled=true 表示任务已在此次认领内终结(子任务聚合完成或已转审批);
// handled=false 表示 direct,调用方继续回合机。err 为不可恢复计划/驱动错误。
func (s *Service) planEngineering(ctx context.Context, runCtx context.Context, workerID string, t task.Task) (bool, error) {
	// 子任务是已切好的执行单元,不再拆解。
	if t.ParentTaskID != nil {
		return false, nil
	}
	// 已有子任务 → 上次认领已拆解(可能因租约中断未收尾):驱动现存子任务聚合,不重复拆。
	children, err := s.store.ListTasksByParent(ctx, t.ID)
	if err != nil {
		return false, err
	}
	if len(children) > 0 {
		return true, s.driveChildrenToDone(ctx, runCtx, workerID, t, children)
	}
	// 已有人工批准 → 请求按原样执行(direct)。含:high-risk 预审通过后不再自动拆;
	// planner ask 被 approve 后 = 人批准整包按原样交付,无 bypass。
	approved, err := s.store.HasApprovedApproval(ctx, t.ID)
	if err != nil {
		return false, err
	}
	if approved > 0 {
		return false, nil
	}

	plan, err := s.planCall(runCtx, t)
	if err != nil {
		return false, err
	}
	switch plan.action {
	case planActionDirect:
		return false, nil
	case planActionAsk:
		if _, err := s.audit(ctx, "task", t.ID, "eng_plan_ask", taskActor(t), plan.reason); err != nil {
			return false, err
		}
		// Phase 10.3 记账:planner ask → 计划留一条 pending do 行(approve 后续跑时由后续 grow 行补边界)。
		if err := s.ledgerAppendAsk(ctx, t, plan.reason); err != nil {
			return false, err
		}
		return true, s.requestApproval(ctx, t, "planner ask: "+plan.reason)
	case planActionSplit:
		subs := make([]task.Task, 0, len(plan.subtasks))
		for _, st := range plan.subtasks {
			child, cerr := s.createPlannedSubtask(ctx, t, st)
			if cerr != nil {
				return true, fmt.Errorf("create subtask %q: %w", st.Title, cerr)
			}
			subs = append(subs, child)
		}
		if _, err := s.audit(ctx, "task", t.ID, "eng_plan", taskActor(t),
			fmt.Sprintf("planner split into %d subtasks", len(subs))); err != nil {
			return true, err
		}
		// Phase 10.3 记账:父 run 拆解 N 子任务(子任务本身无 plan)。
		if err := s.ledgerAppendSplit(ctx, t, len(subs)); err != nil {
			return true, err
		}
		return true, s.driveChildrenToDone(ctx, runCtx, workerID, t, subs)
	default:
		return false, fmt.Errorf("unexpected planner action %q", plan.action)
	}
}

// createPlannedSubtask 把 planner 的一个子项落地成真实 engineering task,挂父请求 id。
// 继承父任务的工作区/风险/工作流;capability/agent 沿用父任务,父无归属则尽力落到
// engineering coding agent(与 intake 建任务同解析,通道 B 直接落地)。
func (s *Service) createPlannedSubtask(ctx context.Context, t task.Task, st engSubtask) (task.Task, error) {
	desc := strings.TrimSpace(st.Description)
	if desc != "" {
		desc += "\n\n"
	}
	desc += "source: subtask of \"" + strings.TrimSpace(t.Title) + "\" (parent " + short8(t.ID) + ")"
	capID, agentID := t.CapabilityID, t.AgentID
	if capID == nil {
		if cap, cerr := s.store.GetCapabilityByCode(ctx, t.CompanyID, "engineering"); cerr == nil && cap.ID != "" {
			c := cap.ID
			capID = &c
			if a, aerr := s.store.GetAgentByCapabilityAndRole(ctx, cap.ID, "coding"); aerr == nil && a.ID != "" {
				ag := a.ID
				agentID = &ag
			}
		}
	}
	maxAttempts := t.MaxAttempts
	if maxAttempts < 2 {
		maxAttempts = 2 // planner 自动驱动的子任务给 2 次尝试(单次抖动不整包失败)
	}
	return s.createTask(ctx, TaskParams{
		CompanyID:          t.CompanyID,
		CapabilityID:       capID,
		WorkflowID:         t.WorkflowID,
		AgentID:            agentID,
		Title:              st.Title,
		Description:        desc,
		ToolName:           "engineering",
		Risk:               t.Risk,
		MaxAttempts:        maxAttempts,
		TimeoutSec:         t.TimeoutSec,
		Workspace:          t.WorkspacePath,
		ParentTaskID:       &t.ID,
		WriterEndpointID:   t.WriterEndpointID,
		ReviewerEndpointID: t.ReviewerEndpointID,
		TestEndpointID:     t.TestEndpointID, // 8.4 分槽:子任务继承父的 test 槽(父建单已默认解析或显式给)
	}, "planner")
}

// driveChildrenToDone 同步驱动全部子任务到 completed,随后聚合父任务(完成 + 摘要审计)。
// 任一子任务失败 → 父请求以失败收口(带子任务错误),让回收/人工可见。
func (s *Service) driveChildrenToDone(ctx context.Context, runCtx context.Context, workerID string, parent task.Task, children []task.Task) error {
	for _, c := range children {
		if err := s.driveChildTask(runCtx, workerID, c); err != nil {
			return fmt.Errorf("subtask %s %q: %w", short8(c.ID), firstLine(c.Title), err)
		}
	}
	summary := fmt.Sprintf("planner decomposed into %d subtask(s); all completed", len(children))
	if _, err := s.store.CompleteTask(runCtx, parent.ID, summary); err != nil {
		return err
	}
	_, err := s.audit(ctx, "task", parent.ID, "eng_plan_done", taskActor(parent), summary)
	return err
}

// driveChildTask 把单个子任务驱动到 completed(镜像 workflow.driveTaskToCompletion 语义):
// 子任务失败 → 错误(父聚合失败);waiting_approval → 轮询等人工(审批仍可由外部下达);
// pending(requeue)→ 再次 ExecuteTask,短暂让出避免热循环。
func (s *Service) driveChildTask(runCtx context.Context, workerID string, c task.Task) error {
	for {
		cur, err := s.store.GetTask(runCtx, c.ID)
		if err != nil {
			return err
		}
		switch cur.Status {
		case "completed":
			return nil
		case "failed":
			return fmt.Errorf("%s", cur.LastError)
		case "waiting_approval":
			if err := sleepCtx(runCtx, approvalPollInterval); err != nil {
				return err
			}
		case "pending":
			if err := s.ExecuteTask(runCtx, workerID, c.ID); err != nil {
				return err
			}
			if err := sleepCtx(runCtx, 200*time.Millisecond); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unexpected status %s", cur.Status)
		}
	}
}
