package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/glacierzzz26/one-person-company-os/internal/pipeline"
	"github.com/glacierzzz26/one-person-company-os/internal/task"
	"github.com/google/uuid"
)

func (s *Service) CreatePipeline(ctx context.Context, projectID, name, kind, description, risk string) (pipeline.Pipeline, error) {
	return s.CreatePipelineAs(ctx, projectID, name, kind, description, risk, "human:cli")
}

// CreatePipelineAs 建流水线(绑 project):kind ∈ 白名单(空 → bugfix;非法 → ErrInvalid)、
// risk 归一到 low/medium/high(缺省 medium)。重名 → ErrConflict。
func (s *Service) CreatePipelineAs(ctx context.Context, projectID, name, kind, description, risk, actor string) (pipeline.Pipeline, error) {
	if projectID == "" || name == "" {
		return pipeline.Pipeline{}, fmt.Errorf("%w: --project and --name are required", ErrInvalid)
	}
	// 校验 project 存在(不存在 → sql.ErrNoRows,HTTP 层 404)。
	if _, err := s.store.GetProject(ctx, projectID); err != nil {
		return pipeline.Pipeline{}, err
	}
	if kind == "" {
		kind = pipeline.KindBugfix
	}
	if !pipeline.ValidKind(kind) {
		return pipeline.Pipeline{}, fmt.Errorf("%w: unknown pipeline kind %q (known: %s)", ErrInvalid, kind, strings.Join(pipeline.ValidKinds, ", "))
	}
	if risk == "" {
		risk = pipeline.RiskMedium
	}
	if !pipeline.ValidRisk(risk) {
		return pipeline.Pipeline{}, fmt.Errorf("%w: invalid risk %q (low|medium|high)", ErrInvalid, risk)
	}
	now := time.Now().Unix()
	p := pipeline.Pipeline{
		ID: uuid.NewString(), ProjectID: projectID, Name: name, Kind: kind,
		Description: description, Risk: risk, Status: pipeline.StatusActive,
		Schedule: "", CreatedAt: now, UpdatedAt: now,
	}
	created, err := s.store.CreatePipeline(ctx, p)
	if err != nil {
		if isUniqueViolation(err) {
			return pipeline.Pipeline{}, fmt.Errorf("%w: pipeline %q already exists in project %s", ErrConflict, name, projectID)
		}
		return pipeline.Pipeline{}, err
	}
	_, err = s.audit(ctx, "pipeline", created.ID, "create", actor, kind+" "+name)
	return created, err
}

func (s *Service) GetPipeline(ctx context.Context, id string) (pipeline.Pipeline, error) {
	return s.store.GetPipeline(ctx, id)
}

func (s *Service) ListPipelines(ctx context.Context, projectID string) ([]pipeline.Pipeline, error) {
	return s.store.ListPipelinesByProject(ctx, projectID)
}

// DeletePipelineAs 删流水线元数据(受控 DELETE):其 run 历史任务保留(project 不删、磁盘不动)。
// DeletePipeline 删流水线(CLI actor human:cli;见 DeletePipelineAs)。
func (s *Service) DeletePipeline(ctx context.Context, id string) error {
	return s.DeletePipelineAs(ctx, id, "human:cli")
}

func (s *Service) DeletePipelineAs(ctx context.Context, id, actor string) error {
	p, err := s.store.GetPipeline(ctx, id)
	if err != nil {
		return err
	}
	if _, err := s.store.DeletePipeline(ctx, id); err != nil {
		return err
	}
	_, err = s.audit(ctx, "pipeline", id, "delete", actor, p.Name)
	return err
}

// RunPipelineAs 触发一次流水线 run = 在项目 root 目录建一条 engineering task
// (方向决策②:复用 engineering driver,不另起执行体)。返回创建出的任务。
//
// 流程(契约 §三.3):GetPipeline + GetProject → status 非 active → ErrInvalid;
// 串行守卫 CountActiveTasksByProject(projectID)>0 → ErrPipelineBusy(409);
// 意图文本 = trim(request) 非空用 request,否则 pipeline.description;
// CreateTaskAs(8.4 落槽/硬失败在 createTask 内自然发生) → audit("pipeline", id, "run")。
// run = 异步建单(qstatus ready),由 worker(server QueueLoop 自驱或 os queue work)消费。
func (s *Service) RunPipelineAs(ctx context.Context, pipelineID, request, actor string) (task.Task, error) {
	pl, err := s.store.GetPipeline(ctx, pipelineID)
	if err != nil {
		return task.Task{}, err
	}
	if pl.Status != pipeline.StatusActive {
		return task.Task{}, fmt.Errorf("%w: pipeline %q is %s — only active pipelines can run", ErrInvalid, pl.Name, pl.Status)
	}
	prj, err := s.store.GetProject(ctx, pl.ProjectID)
	if err != nil {
		return task.Task{}, err
	}
	// 同目录串行守卫:同 project 至多一条活跃 run(极端并发由 8.3 git baseline 起点语义兜底)。
	cnt, err := s.store.CountActiveTasksByProject(ctx, pl.ProjectID)
	if err != nil {
		return task.Task{}, err
	}
	if cnt > 0 {
		return task.Task{}, fmt.Errorf("%w: project %q already has an active run — same-directory runs are serialized (wait or finish it first)", ErrPipelineBusy, prj.Name)
	}
	intent := strings.TrimSpace(request)
	if intent == "" {
		intent = pl.Description
	}
	if strings.TrimSpace(intent) == "" {
		return task.Task{}, fmt.Errorf("%w: no run request given and pipeline %q has an empty description", ErrInvalid, pl.Name)
	}
	tsk, err := s.CreateTaskAs(ctx, TaskParams{
		CompanyID: prj.CompanyID, ToolName: "engineering",
		Title:       pl.Name,
		Description: intent,
		Risk:        pl.Risk,
		Workspace:   prj.RootPath,
		ProjectID:   &prj.ID,
		MaxAttempts: 1,
	}, actor)
	if err != nil {
		return task.Task{}, err
	}
	_, err = s.audit(ctx, "pipeline", pl.ID, "run", actor, "task "+tsk.ID)
	return tsk, err
}

// RunPipeline 触发一次流水线 run(CLI actor human:cli;见 RunPipelineAs)。
func (s *Service) RunPipeline(ctx context.Context, pipelineID, request string) (task.Task, error) {
	return s.RunPipelineAs(ctx, pipelineID, request, "human:cli")
}

// ListTasksByProject 项目详情「最近 runs」(复用仓库层;limit 缺省调用方定)。
func (s *Service) ListTasksByProject(ctx context.Context, projectID string, limit int64) ([]task.Task, error) {
	return s.store.ListTasksByProject(ctx, projectID, limit)
}
