package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/glacierzzz26/one-person-company-os/internal/agent"
	"github.com/glacierzzz26/one-person-company-os/internal/task"
	"github.com/glacierzzz26/one-person-company-os/internal/workflow"
)

// WorkflowNode 是 workflow.definition(JSON)中的有序节点。方向基线:自研轻量
// State Machine,Task 是执行单元;节点顺序执行,不并行。
// skip=true 的节点为输入声明(如 Engineering 的 issue),不创建/执行 Task。
// agent_role 为空的节点是审批门:进入等待审批,人工 approve 后放行(直接完成)。
// capability 为节点所属 Capability code(如 research/product/engineering):
// 非空 → Agent 按 (capability, role) 解析、Task 落 capability_id(跨 Capability Workflow);
// 空 → 回退公司级 role 解析(向后兼容 2.2)。
type WorkflowNode struct {
	Step        string `json:"step"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Tool        string `json:"tool"`
	Capability  string `json:"capability,omitempty"`
	AgentRole   string `json:"agent_role"`
	Risk        string `json:"risk"`
	Workspace   string `json:"workspace"`
	Skip        bool   `json:"skip,omitempty"`
}

const approvalPollInterval = 1 * time.Second

// RunWorkflow 解析 workflow definition,按序为每个节点创建 Task(挂 workflow_id)
// 并执行至完成。节点 Task 失败 → 默认中断;节点进入 waiting_approval → 暂停等待
// 人工决定(approve 后续跑,reject/changes 中断)。skip 节点不执行,只留审计。
// progress 回调用于 CLI 输出节点进度;返回摘要如 "7/7 nodes completed"。
func (s *Service) RunWorkflow(ctx context.Context, workerID, workflowID string, progress func(string)) (string, error) {
	w, err := s.store.GetWorkflow(ctx, workflowID)
	if err != nil {
		return "", err
	}
	var nodes []WorkflowNode
	if err := json.Unmarshal([]byte(w.Definition), &nodes); err != nil {
		return "", fmt.Errorf("parse workflow %s definition: %w", workflowID, err)
	}
	if len(nodes) == 0 {
		return "", fmt.Errorf("workflow %s has no nodes", workflowID)
	}

	executed := 0
	total := 0
	for _, n := range nodes {
		if n.Skip {
			if _, err := s.audit(ctx, "workflow", w.ID, "skip", "runtime:"+workerID,
				fmt.Sprintf("step %s: %s (input, not executed)", n.Step, n.Title)); err != nil {
				return "", err
			}
			continue
		}
		total++
		step := n.Step
		if step == "" {
			step = fmt.Sprintf("%d", total)
		}
		if progress != nil {
			progress(fmt.Sprintf("→ %s (%s): %s", step, n.Tool, n.Title))
		}
		if err := s.runWorkflowNode(ctx, workerID, w, n, step, progress); err != nil {
			return "", err
		}
		executed++
	}
	summary := fmt.Sprintf("%d/%d nodes completed", executed, total)
	_, err = s.audit(ctx, "workflow", w.ID, "run", "runtime:"+workerID, summary)
	return summary, err
}

// runWorkflowNode 为一个节点创建 Task,并驱动到 completed(或失败中断 workflow)。
func (s *Service) runWorkflowNode(ctx context.Context, workerID string, w workflow.Workflow, n WorkflowNode, step string, progress func(string)) error {
	var agentID, capabilityID *string
	if n.AgentRole != "" {
		a, capID, err := s.resolveAgentForNode(ctx, w.CompanyID, n)
		if err != nil {
			return fmt.Errorf("step %s: %w", step, err)
		}
		agentID = &a.ID
		capabilityID = &capID
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
		CompanyID: w.CompanyID, WorkflowID: &w.ID, AgentID: agentID, CapabilityID: capabilityID,
		Title: title, Description: n.Description, ToolName: n.Tool, Risk: risk,
		MaxAttempts: 1, TimeoutSec: 0, Workspace: ws,
	})
	if err != nil {
		return fmt.Errorf("step %s: create task: %w", step, err)
	}

	// 审批门(agent_role 为空):审批通过后无实际执行内容,直接置 completed 放行。
	return s.driveTaskToCompletion(ctx, workerID, w, t, step, n.AgentRole == "", progress)
}

// resolveAgentForNode 按节点解析执行 Agent 及其所属 Capability。
// 节点声明 capability → 按 (company, code) 找 Capability,再按 (capability_id, role) 找 Agent;
// 未声明 → 回退 2.2 公司级 role 解析,返回该 Agent 的 capability_id。
func (s *Service) resolveAgentForNode(ctx context.Context, companyID string, n WorkflowNode) (agent.Agent, string, error) {
	if n.Capability != "" {
		c, err := s.store.GetCapabilityByCode(ctx, companyID, n.Capability)
		if err != nil {
			return agent.Agent{}, "", fmt.Errorf("resolve capability %q: %w", n.Capability, err)
		}
		a, err := s.store.GetAgentByCapabilityAndRole(ctx, c.ID, n.AgentRole)
		if err != nil {
			return agent.Agent{}, "", fmt.Errorf("resolve agent for role %q in capability %q: %w", n.AgentRole, n.Capability, err)
		}
		return a, c.ID, nil
	}
	a, err := s.store.GetAgentByRole(ctx, companyID, n.AgentRole)
	if err != nil {
		return agent.Agent{}, "", fmt.Errorf("resolve agent for role %q: %w", n.AgentRole, err)
	}
	return a, a.CapabilityID, nil
}

// driveTaskToCompletion 驱动节点 Task 直至 completed,期间处理审批门:
// 节点进入 waiting_approval → 暂停轮询等待人工决定;approve 后 gate 直接完成、
// 执行节点重跑一次(审批记录使其放行);reject/changes → failed → 中断 workflow。
func (s *Service) driveTaskToCompletion(ctx context.Context, workerID string, w workflow.Workflow, t task.Task, step string, gate bool, progress func(string)) error {
	firstRun := true
	paused := false
	for {
		cur, err := s.store.GetTask(ctx, t.ID)
		if err != nil {
			return err
		}
		switch cur.Status {
		case "completed":
			return nil
		case "failed":
			return fmt.Errorf("workflow %s aborted at step %s: task %s status=%s err=%s",
				w.Name, step, t.ID, cur.Status, cur.LastError)
		case "waiting_approval":
			if !paused {
				if _, err := s.audit(ctx, "workflow", w.ID, "pause", "runtime:"+workerID,
					fmt.Sprintf("step %s: waiting approval on task %s", step, t.ID)); err != nil {
					return err
				}
				if progress != nil {
					progress(fmt.Sprintf("⏸ %s: waiting approval on task %s (os approval list / approve <id>)", step, t.ID))
				}
				paused = true
			}
			if err := sleepCtx(ctx, approvalPollInterval); err != nil {
				return err
			}
		case "pending":
			if gate && !firstRun {
				// 已通过审批的门节点:人工批准即完成。
				if _, err := s.store.CompleteTask(ctx, t.ID, "approved by human"); err != nil {
					return err
				}
				return nil
			}
			if err := s.ExecuteTask(ctx, workerID, t.ID); err != nil {
				return fmt.Errorf("step %s (task %s): %w", step, t.ID, err)
			}
			firstRun = false
		default:
			return fmt.Errorf("step %s: task %s unexpected status %s", step, t.ID, cur.Status)
		}
	}
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
