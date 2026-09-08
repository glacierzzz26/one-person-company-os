package service

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/glacierzzz26/one-person-company-os/internal/pipeline"
	"github.com/glacierzzz26/one-person-company-os/internal/schedule"
	"github.com/glacierzzz26/one-person-company-os/internal/task"
	"github.com/google/uuid"
)

// 内部 actor(审计 actor 无格式锁,10.1 human:* 之外 system:* 合法;Web 鉴权按 path 不含 actor 判定)。
const (
	// ActorSchedule 是 schedule 调度循环到点触发的审计 actor(契约 3.7;server ScheduleLoop 用它跑单)。
	ActorSchedule = "system:schedule"
	actorChain    = "system:patrol_chain" // 巡检发现处置链:非 ok+high+fix → 链拉同项目 bugfix run
)

// PipelineOpt 建流水线的可选覆写(Phase 10.4 起:plan_policy;签名零改,新增变体走 variadic opts)。
type PipelineOpt func(*pipelineOptions)

type pipelineOptions struct{ planPolicy string }

// PipelinePlanPolicy 声明流水线计划策略(plan_policy,10.4 触发列)。缺省 = 空 → DB 默认 adaptive
// (grow 记账,零漂移)。合法性服务层白名单校验(非法 → ErrInvalid,DB 不 CHECK 只增不改)。
func PipelinePlanPolicy(p string) PipelineOpt {
	return func(o *pipelineOptions) { o.planPolicy = p }
}

func (s *Service) CreatePipeline(ctx context.Context, projectID, name, kind, description, risk, schedule string, opts ...PipelineOpt) (pipeline.Pipeline, error) {
	return s.CreatePipelineAs(ctx, projectID, name, kind, description, risk, schedule, "human:cli", opts...)
}

// CreatePipelineAs 建流水线(绑 project):kind ∈ 白名单(空 → bugfix;非法 → ErrInvalid)、
// risk 归一到 low/medium/high(缺省 medium)、schedule cron 五段校验(空/off = 不调度;
// 非法 → ErrInvalid)、plan_policy ∈ 白名单(空 → adaptive;非法 → ErrInvalid)。重名 → ErrConflict。
// schedule 存原文(10.2 起解析;服务器调度层遇非调度跳过)。plan_policy:insert 零列(DB 默认 adaptive 零
// 漂移);请求 synthesize → 建后 Set(两步落库,仅当 created.PlanPolicy != 期望)。
func (s *Service) CreatePipelineAs(ctx context.Context, projectID, name, kind, description, risk, schedRaw, actor string, opts ...PipelineOpt) (pipeline.Pipeline, error) {
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
	var o pipelineOptions
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}
	policy := strings.TrimSpace(o.planPolicy)
	if policy == "" {
		policy = pipeline.PlanPolicyAdaptive
	}
	if !pipeline.ValidPlanPolicy(policy) {
		return pipeline.Pipeline{}, fmt.Errorf("%w: invalid plan_policy %q (known: %s)", ErrInvalid, policy, strings.Join(pipeline.ValidPlanPolicies, ", "))
	}
	sched, err := normalizeSchedule(schedRaw)
	if err != nil {
		return pipeline.Pipeline{}, err
	}
	now := time.Now().Unix()
	p := pipeline.Pipeline{
		ID: uuid.NewString(), ProjectID: projectID, Name: name, Kind: kind,
		Description: description, Risk: risk, Status: pipeline.StatusActive,
		Schedule: sched, CreatedAt: now, UpdatedAt: now,
	}
	created, err := s.store.CreatePipeline(ctx, p)
	if err != nil {
		if isUniqueViolation(err) {
			return pipeline.Pipeline{}, fmt.Errorf("%w: pipeline %q already exists in project %s", ErrConflict, name, projectID)
		}
		return pipeline.Pipeline{}, err
	}
	// 10.4 零漂移:建时 DB 默认 adaptive;仅当请求策略 ≠ 实际 → 建后 Set(两步落库)。
	if created.PlanPolicy != policy {
		if created, err = s.store.SetPipelinePlanPolicy(ctx, created.ID, policy); err != nil {
			return pipeline.Pipeline{}, err
		}
	}
	_, err = s.audit(ctx, "pipeline", created.ID, "create", actor, kind+" "+name+" (plan_policy="+policy+")")
	return created, err
}

// normalizeSchedule 校验 schedule(5 段数字 cron;契约 3.5 子集)并返回待存原文(trimmed):
// ""/off = 不调度(合法);非法 → ErrInvalid 带指引。存储原文,解析(判下次命中/是否到点)在服务器调度层。
func normalizeSchedule(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if _, scheduled, err := schedule.Parse(s); err != nil {
		return "", fmt.Errorf("%w: invalid schedule %q — use 5-field numeric cron `min hour dom month dow` like \"0 9 * * *\" (daily 09:00) / \"0 9 * * 1-5\" (weekdays 09:00); empty or \"off\" disables", ErrInvalid, s)
	} else if !scheduled {
		return s, nil // 空/off → 存原文,不调度
	}
	return s, nil
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
	// Phase 10.2:先断 task 反链(pipeline_id FK ON),run 历史任务保留(同 project 语义)。
	if _, err := s.store.ClearTaskPipeline(ctx, id); err != nil {
		return err
	}
	if _, err := s.store.DeletePipeline(ctx, id); err != nil {
		return err
	}
	_, err = s.audit(ctx, "pipeline", id, "delete", actor, p.Name)
	return err
}

// UpdatePipelineScheduleAs 改流水线调度(cron 校验同建;空/off = 停调度)。审计 detail 记 old → new。
// 契约 3.4:Web 作者面(改调度 Modal)/HTTP PUT 的唯一写口;不存在的流水线 → sql.ErrNoRows(HTTP 404)。
func (s *Service) UpdatePipelineScheduleAs(ctx context.Context, id, schedRaw, actor string) (pipeline.Pipeline, error) {
	sched, err := normalizeSchedule(schedRaw)
	if err != nil {
		return pipeline.Pipeline{}, err
	}
	old, err := s.store.GetPipeline(ctx, id)
	if err != nil {
		return pipeline.Pipeline{}, err
	}
	updated, err := s.store.UpdatePipelineSchedule(ctx, id, sched)
	if err != nil {
		return pipeline.Pipeline{}, err
	}
	_, err = s.audit(ctx, "pipeline", id, "edit", actor, "schedule "+old.Schedule+" → "+updated.Schedule)
	return updated, err
}

// ListScheduledPipelines 调度循环取数:active 且 schedule 非空(供 server ScheduleLoop 判下次命中;
// 契约 3.7;schedule 由建/改时校验过,off/空不落此集以外,解析判停仍以服务器 parse 为准)。
func (s *Service) ListScheduledPipelines(ctx context.Context) ([]pipeline.ScheduledPipeline, error) {
	return s.store.ListScheduledPipelines(ctx)
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
		PipelineID:  &pl.ID,
		MaxAttempts: 1,
	}, actor)
	if err != nil {
		return task.Task{}, err
	}
	// Phase 10.3:计划 pre-materialize 与建单同步、先于任何 worker 认领 —— ops_patrol upfront 铺全
	// (先审后干),bugfix/develop grow 只建 plan 行。落账失败 → run 不进入执行(不留 plan-less 静默 run):
	// fail 该 run(attempt 0 → max_attempts=1 → FailTask)并把错误回调用方。
	if err := s.ensureRunPlan(ctx, tsk, planKindFor(pl.Kind)); err != nil {
		if _, ferr := s.store.FailTask(ctx, tsk.ID, "plan ledger init: "+err.Error()); ferr != nil {
			log.Printf("fail run task %s after plan ledger init error: %v", short8(tsk.ID), ferr)
		}
		return task.Task{}, fmt.Errorf("init run plan ledger: %w", err)
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
