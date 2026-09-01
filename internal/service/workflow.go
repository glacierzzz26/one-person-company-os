package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
)

// WorkflowNode 是 workflow.definition(JSON)中的有序节点。方向基线:自研轻量
// State Machine,Task 是执行单元;Phase 1 节点顺序执行,不并行。
type WorkflowNode struct {
	Step        string `json:"step"`
	Title       string `json:"title"`
	Description string `json:"description"`
	AgentRole   string `json:"agent_role"`
	Risk        string `json:"risk"`
	Workspace   string `json:"workspace"`
}

// RunWorkflow 解析 workflow definition,按序为每个节点创建 Task(挂 workflow_id)
// 并立即执行。节点 Task 非 completed → 默认中断,不再创建后续节点。
func (s *Service) RunWorkflow(ctx context.Context, workerID, workflowID string) error {
	w, err := s.store.GetWorkflow(ctx, workflowID)
	if err != nil {
		return err
	}
	var nodes []WorkflowNode
	if err := json.Unmarshal([]byte(w.Definition), &nodes); err != nil {
		return fmt.Errorf("parse workflow %s definition: %w", workflowID, err)
	}
	if len(nodes) == 0 {
		return fmt.Errorf("workflow %s has no nodes", workflowID)
	}

	executed := 0
	for _, n := range nodes {
		step := n.Step
		if step == "" {
			step = fmt.Sprintf("%d", executed+1)
		}
		var agentID *string
		if n.AgentRole != "" {
			a, err := s.store.GetAgentByRole(ctx, w.CompanyID, n.AgentRole)
			if err != nil {
				return fmt.Errorf("step %s: resolve agent for role %q: %w", step, n.AgentRole, err)
			}
			agentID = &a.ID
		}
		title := n.Title
		if title == "" {
			title = w.Name + "/" + step
		}
		risk := n.Risk
		if risk == "" {
			risk = "medium"
		}
		ws := n.Workspace
		if ws == "" {
			dir, err := os.MkdirTemp("", "opos-wf-")
			if err != nil {
				return fmt.Errorf("step %s: create workspace: %w", step, err)
			}
			ws = dir
		}

		t, err := s.CreateTask(ctx, TaskParams{
			CompanyID: w.CompanyID, WorkflowID: &w.ID, AgentID: agentID,
			Title: title, Description: n.Description, Risk: risk,
			MaxAttempts: 1, TimeoutSec: 0, Workspace: ws,
		})
		if err != nil {
			return fmt.Errorf("step %s: create task: %w", step, err)
		}
		if err := s.ExecuteTask(ctx, workerID, t.ID); err != nil {
			return fmt.Errorf("step %s (task %s): %w", step, t.ID, err)
		}

		done, err := s.store.GetTask(ctx, t.ID)
		if err != nil {
			return err
		}
		if done.Status != "completed" {
			_, _ = s.audit(ctx, "workflow", w.ID, "run", "runtime:"+workerID,
				fmt.Sprintf("aborted at step %s: task %s %s (%s)", step, t.ID, done.Status, done.LastError))
			return fmt.Errorf("workflow %s aborted at step %s: task %s status=%s err=%s",
				w.Name, step, t.ID, done.Status, done.LastError)
		}
		executed++
	}
	_, err = s.audit(ctx, "workflow", w.ID, "run", "runtime:"+workerID,
		fmt.Sprintf("%d/%d nodes completed", executed, len(nodes)))
	return err
}
