package service

import (
	"context"
	"strings"
	"time"

	"github.com/glacierzzz26/one-person-company-os/internal/task"
	"github.com/google/uuid"
)

type TaskParams struct {
	CompanyID    string
	CapabilityID *string
	WorkflowID   *string
	AgentID      *string
	Title        string
	Description  string
	ToolName     string
	Risk         string
	MaxAttempts  int64
	TimeoutSec   int64
	Workspace    string
	// Phase 6.2:Engineering 回合字段(可选)。8.4:空槽在 engineering(live)建单按默认档解析(见 tier.go)。
	ParentTaskID       *string
	WriterEndpointID   *string // 写者端点;默认 cheap(proto=anthropic,可作 claude 委派后端;空 = claude 自带)
	ReviewerEndpointID *string // 把关端点;默认 frontier(proto=openai)
	TestEndpointID     *string // test 判读端点(8.4 分槽);默认 standard(proto=openai)
}

func (s *Service) CreateTask(ctx context.Context, p TaskParams) (task.Task, error) {
	return s.createTask(ctx, p, "human:cli")
}

// CreateTaskAs 同 CreateTask,actor 标来源(human:console = Web 控制台;CLI 用 CreateTask)。
func (s *Service) CreateTaskAs(ctx context.Context, p TaskParams, actor string) (task.Task, error) {
	return s.createTask(ctx, p, actor)
}

// createTask 创建任务并落 Audit。actor 区分来源:human:cli(命令)/ intake(研发 Intake 通道 B)。
func (s *Service) createTask(ctx context.Context, p TaskParams, actor string) (task.Task, error) {
	if p.Risk == "" {
		p.Risk = "low"
	}
	if p.ToolName == "" {
		p.ToolName = "shell"
	}
	if p.MaxAttempts == 0 {
		p.MaxAttempts = 1
	}
	// 8.4:engineering(live)建单按默认档解析空端点槽(reviewer=frontier/test=standard/writer=cheap)。
	// 显式给的不解析;judging 槽解析不到 → 硬失败;writer 槽空 = 合法(不触发)。scripted 一律跳过(离线红线)。
	// 模式判定 9.3 起走 DB 生效(engineScripted,env 仅测试 seam)。
	defaultDetail := ""
	if p.ToolName == "engineering" && !s.engineScripted(ctx, p.CompanyID) {
		w, r, tt, detail, derr := s.applyEndpointDefaults(ctx, p.CompanyID, p.WriterEndpointID, p.ReviewerEndpointID, p.TestEndpointID)
		if derr != nil {
			return task.Task{}, derr
		}
		p.WriterEndpointID, p.ReviewerEndpointID, p.TestEndpointID = w, r, tt
		if len(detail) > 0 {
			defaultDetail = strings.Join(detail, "; ")
		}
	}
	now := time.Now().Unix()
	t := task.Task{
		ID: uuid.NewString(), CompanyID: p.CompanyID,
		CapabilityID: p.CapabilityID, WorkflowID: p.WorkflowID, AgentID: p.AgentID,
		Title: p.Title, Description: p.Description, ToolName: p.ToolName,
		Status: "pending", Priority: 0, Attempt: 0, Risk: p.Risk,
		QStatus: "ready", MaxAttempts: p.MaxAttempts, TimeoutSec: p.TimeoutSec,
		WorkspacePath:      p.Workspace,
		ParentTaskID:       p.ParentTaskID,
		WriterEndpointID:   p.WriterEndpointID,
		ReviewerEndpointID: p.ReviewerEndpointID,
		TestEndpointID:     p.TestEndpointID,
		CreatedAt:          now, UpdatedAt: now,
	}
	created, err := s.store.CreateTask(ctx, t)
	if err != nil {
		return task.Task{}, err
	}
	// detail 只在确有默认落档时非空(§八 验收 2「日志/审计可见所选模型」);显式给齐则保持空(与既有一致)。
	_, err = s.audit(ctx, "task", created.ID, "create", actor, defaultDetail)
	return created, err
}

func (s *Service) ListTasks(ctx context.Context, companyID, status, risk string, attemptMin int64) ([]task.Task, error) {
	return s.store.ListTasks(ctx, companyID, status, risk, attemptMin)
}

func (s *Service) GetTask(ctx context.Context, id string) (task.Task, error) {
	return s.store.GetTask(ctx, id)
}
