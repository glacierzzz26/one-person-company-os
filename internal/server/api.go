package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/glacierzzz26/one-person-company-os/internal/service"
	"github.com/glacierzzz26/one-person-company-os/internal/task"
	"github.com/go-chi/chi/v5"
)

// Phase 7.1 — /api/v1 JSON API(运营控制台数据通道)。契约见 docs/phase7/design/web-console.md。
// 全部 handler 是 service 方法的 thin wrapper:信封 {ok,data} / 错误 {ok:false,error:{code,message}},
// 404 = sql.ErrNoRows,字段 snake_case = 域结构体 json tag,时间 unix 秒,ID 完整 UUID。

// consoleActor 是 /api/v1 全部写操作的审计 actor(与 CLI 的 human:cli、intake 的 intake:* 区分来源)。
const consoleActor = "human:console"

// ---- 信封 / 通用 ----

func apiOK(w http.ResponseWriter, v any) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "data": v})
}

func apiCreated(w http.ResponseWriter, v any) {
	writeJSON(w, http.StatusCreated, map[string]any{"ok": true, "data": v})
}

func apiErr(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]any{"ok": false, "error": map[string]string{"code": code, "message": msg}})
}

// handleServiceErr 把 service 错误映射到 HTTP:
//   - sql.ErrNoRows → 404 not_found
//   - service.ErrInvalid → 400 bad_request(客户端输入非法:kind/risk/root_path 等)
//   - service.ErrProjectBusy/ErrPipelineBusy/ErrConflict → 409 conflict(重名 / 活跃 run 串行守卫)
//   - 其余 → 500 internal
func handleServiceErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, sql.ErrNoRows):
		apiErr(w, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, service.ErrInvalid):
		apiErr(w, http.StatusBadRequest, "bad_request", err.Error())
	case errors.Is(err, service.ErrProjectBusy),
		errors.Is(err, service.ErrPipelineBusy),
		errors.Is(err, service.ErrConflict):
		apiErr(w, http.StatusConflict, "conflict", err.Error())
	default:
		apiErr(w, http.StatusInternalServerError, "internal", err.Error())
	}
}

// decodeJSON 解析 JSON body;失败回 400 并返回 false。
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		apiErr(w, http.StatusBadRequest, "bad_request", "invalid JSON body: "+err.Error())
		return false
	}
	return true
}

func queryParam(r *http.Request, name string) string { return r.URL.Query().Get(name) }

func pathParam(r *http.Request, name string) string { return chi.URLParam(r, name) }

// strPtr 把请求里空字符串视作缺省(nil);非空才建指针。
func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// ---- 路由注册 ----

func (s *Server) registerAPIRoutes(r chi.Router) {
	// 公司 / 组织
	r.Get("/companies", s.apiListCompanies)
	r.Post("/companies", s.apiCreateCompany)
	r.Get("/companies/{id}", s.apiGetCompany)
	r.Get("/companies/{id}/overview", s.apiOverview)
	r.Get("/companies/{id}/capabilities", s.apiListCapabilities)
	r.Get("/companies/{id}/workflows", s.apiListWorkflows)
	r.Get("/capabilities/{capID}/agents", s.apiListAgents)

	// 决策 / 记忆 / 审计
	r.Get("/companies/{id}/decisions", s.apiListDecisions)
	r.Post("/companies/{id}/decisions", s.apiCreateDecision)
	r.Get("/decisions/{id}", s.apiGetDecision)
	r.Get("/companies/{id}/memories", s.apiListMemories)
	r.Get("/companies/{id}/memories/search", s.apiSearchMemories)
	r.Post("/companies/{id}/memories", s.apiCreateMemory)
	r.Get("/audit", s.apiListAudits)

	// 任务 / 执行
	r.Get("/tasks", s.apiListTasks)
	r.Post("/tasks", s.apiCreateTask)
	r.Get("/tasks/{id}", s.apiGetTask)
	r.Get("/tasks/{id}/executions", s.apiListTaskExecutions)
	r.Get("/tasks/{id}/plan", s.apiGetTaskPlan) // 10.3:run 计划只读端点(无写口;契约 §3.4)
	r.Post("/tasks/{id}/publish-pr", s.apiPublishTaskPR) // 10.5:人工重试发收尾 PR(完成态 + 幂等)

	// 审批
	r.Get("/approvals", s.apiListApprovals)
	r.Get("/approvals/{id}", s.apiGetApproval)
	r.Post("/approvals/{id}/decision", s.apiDecideApproval)

	// 模型端点池 / 通道 B
	r.Get("/companies/{id}/endpoints", s.apiListEndpoints)
	r.Get("/endpoints/{id}", s.apiGetEndpoint)
	r.Post("/endpoints", s.apiAddEndpoint)
	r.Post("/endpoints/{id}/select", s.apiSelectEndpointModel)
	r.Post("/endpoints/{id}/models", s.apiFetchEndpointModels)
	r.Post("/companies/{id}/intake/sync", s.apiIntakeSync)

	// Phase 10.1 — 项目 + 声明式流水线(契约 docs/phase10/design/project-pipeline-foundation.md §3.4)。
	r.Get("/companies/{id}/projects", s.apiListProjects)
	r.Post("/companies/{id}/projects", s.apiCreateProject)
	r.Get("/projects/{id}", s.apiGetProject)
	r.Put("/projects/{id}", s.apiUpdateProject)                                            // 10.5:编辑(名称/描述/GitHub 绑定地址)
	r.Delete("/projects/{id}", s.apiDeleteProject)
	r.Post("/projects/{id}/code-source/refresh", s.apiRefreshProjectCodeSource)            // D7:建后补/换 remote → 重认领代码源
	r.Get("/projects/{id}/secrets", s.handleListProjectSecrets)                            // 10.5:项目级机密(github_token)
	r.Put("/projects/{id}/secrets/{secretID}", s.handleSetProjectSecret)
	r.Delete("/projects/{id}/secrets/{secretID}", s.handleDeleteProjectSecret)
	r.Post("/projects/{id}/intake/sync", s.apiSyncProjectIssues)                           // 10.5:项目级通道 B 同步(绑仓)
	r.Get("/projects/{id}/pipelines", s.apiListPipelines)
	r.Post("/projects/{id}/pipelines", s.apiCreatePipeline)
	r.Get("/projects/{id}/tasks", s.apiListProjectTasks)                 // 最近 runs(复用 ListTasksByProject)
	r.Get("/projects/{projectID}/patrol/{taskID}", s.apiGetPatrolReport) // 10.2:巡检报告正文读端点
	r.Get("/pipelines/{id}", s.apiGetPipeline)
	r.Put("/pipelines/{id}", s.apiUpdatePipelineSchedule) // 10.2:改调度(cron 作者面 Web/HTTP)
	r.Delete("/pipelines/{id}", s.apiDeletePipeline)
	r.Post("/pipelines/{id}/run", s.apiRunPipeline)

	// 设置(Phase 9.2):控制台令牌轮换(Bearer 鉴权后;旧令牌即失效)。
	r.Put("/settings/console-token", s.handleRotateConsoleToken)
	// 设置(Phase 9.3):全局生效行 / 换主密钥 / 公司覆盖 / 公司机密(契约 runtime-knobs-web.md §3.6)。
	r.Get("/settings", s.handleGetGlobalSettings)
	r.Put("/settings", s.handleUpdateGlobalSettings)
	r.Post("/settings/rotate-master-key", s.handleRotateMasterKey)
	r.Get("/companies/{id}/settings", s.handleGetCompanySettings)
	r.Put("/companies/{id}/settings", s.handleUpdateCompanySettings)
	r.Delete("/companies/{id}/settings", s.handleResetCompanySettings)
	r.Get("/companies/{id}/secrets", s.handleListCompanySecrets)
	r.Put("/companies/{id}/secrets/{secretID}", s.handleSetCompanySecret)
	r.Delete("/companies/{id}/secrets/{secretID}", s.handleDeleteCompanySecret)
}

// ---- 公司 / 组织 ----

func (s *Server) apiListCompanies(w http.ResponseWriter, r *http.Request) {
	list, err := s.svc.ListCompanies(r.Context())
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	apiOK(w, list)
}

func (s *Server) apiCreateCompany(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name   string `json:"name"`
		Vision string `json:"vision"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Name == "" {
		apiErr(w, http.StatusBadRequest, "bad_request", "name is required")
		return
	}
	c, err := s.svc.CreateCompanyAs(r.Context(), req.Name, req.Vision, consoleActor)
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	apiCreated(w, c)
}

func (s *Server) apiGetCompany(w http.ResponseWriter, r *http.Request) {
	c, err := s.svc.GetCompany(r.Context(), pathParam(r, "id"))
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	apiOK(w, c)
}

func (s *Server) apiOverview(w http.ResponseWriter, r *http.Request) {
	ov, err := s.svc.Overview(r.Context(), pathParam(r, "id"))
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	apiOK(w, ov)
}

func (s *Server) apiListCapabilities(w http.ResponseWriter, r *http.Request) {
	list, err := s.svc.ListCapabilities(r.Context(), pathParam(r, "id"))
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	apiOK(w, list)
}

func (s *Server) apiListAgents(w http.ResponseWriter, r *http.Request) {
	list, err := s.svc.ListAgents(r.Context(), pathParam(r, "capID"))
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	apiOK(w, list)
}

func (s *Server) apiListWorkflows(w http.ResponseWriter, r *http.Request) {
	list, err := s.svc.ListWorkflows(r.Context(), pathParam(r, "id"))
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	apiOK(w, list)
}

// ---- 任务 / 执行 ----

func (s *Server) apiListTasks(w http.ResponseWriter, r *http.Request) {
	attemptMin, err := parseAttempt(queryParam(r, "attempt"))
	if err != nil {
		apiErr(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	list, err := s.svc.ListTasks(r.Context(), queryParam(r, "company"), queryParam(r, "status"), queryParam(r, "risk"), attemptMin)
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	apiOK(w, list)
}

func (s *Server) apiGetTask(w http.ResponseWriter, r *http.Request) {
	t, err := s.svc.GetTask(r.Context(), pathParam(r, "id"))
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	apiOK(w, t)
}

func (s *Server) apiListTaskExecutions(w http.ResponseWriter, r *http.Request) {
	list, err := s.svc.ListExecutions(r.Context(), pathParam(r, "id"), "")
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	apiOK(w, list)
}

// apiPublishTaskPR POST /tasks/{id}/publish-pr:人工重试发收尾 PR(Phase 10.5,契约 §四 E)。
// 完成态 + 幂等(已置 pull_request_url → 直接返回既有);prTarget / 项目 token / scripted 门不过 → 400
// 明确错误(不发网络)。返回完整 task(带 pull_request_url/number 刷新)。
func (s *Server) apiPublishTaskPR(w http.ResponseWriter, r *http.Request) {
	t, err := s.svc.PublishTaskPR(r.Context(), pathParam(r, "id"), consoleActor)
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	apiOK(w, t)
}

// ---- 10.3 run 计划只读端点 ----

// apiGetTaskPlan 契约 §3.4:run 计划只读端点。未知 task → 404(GetTask);
// 无 plan(非流水线 run / 历史 run)→ data {task_id, plan:null};有 → 阶段列表随 plan 嵌套。
func (s *Server) apiGetTaskPlan(w http.ResponseWriter, r *http.Request) {
	taskID := pathParam(r, "id")
	t, err := s.svc.GetTask(r.Context(), taskID)
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	rp, phases, ok, err := s.svc.GetTaskPlan(r.Context(), taskID)
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	if !ok {
		apiOK(w, planReadResp{TaskID: taskID})
		return
	}
	resp := planReadResp{TaskID: taskID}
	resp.Plan = &planViewResp{
		Kind:         rp.Kind,
		Materialized: rp.Materialized,
		PlanPolicy:   s.pipelinePlanPolicyFor(r.Context(), t), // 10.4:run 的流水线 plan_policy(Web 区分 synthesize 空态)
		CreatedAt:    rp.CreatedAt,
		UpdatedAt:    rp.UpdatedAt,
		Phases:       make([]phaseViewResp, 0, len(phases)),
	}
	for _, ph := range phases {
		resp.Plan.Phases = append(resp.Plan.Phases, phaseViewResp{
			Seq: ph.Seq, Kind: ph.Kind, Title: ph.Title, Allocator: ph.Allocator,
			Status: ph.Status, Evidence: ph.Evidence, Note: ph.Note,
			StartedAt: ph.StartedAt, FinishedAt: ph.FinishedAt,
		})
	}
	apiOK(w, resp)
}

// pipelinePlanPolicyFor 取 run 所属流水线的 plan_policy(展示位)。流水线已删/无反链 → 空串。
func (s *Server) pipelinePlanPolicyFor(ctx context.Context, t task.Task) string {
	if t.PipelineID == nil {
		return ""
	}
	pl, err := s.svc.GetPipeline(ctx, *t.PipelineID)
	if err != nil {
		return ""
	}
	return pl.PlanPolicy
}

// planReadResp 契约 §3.4 响应:task_id 平铺 + plan(可 null = 非流水线 run / 历史 run)。
type planReadResp struct {
	TaskID string        `json:"task_id"`
	Plan   *planViewResp `json:"plan"`
}

type planViewResp struct {
	Kind         string          `json:"kind"` // patrol | engineering
	Materialized string          `json:"materialized"`
	PlanPolicy   string          `json:"plan_policy"` // 10.4:run 所属流水线 adaptive|synthesize;流水线已删 → ""
	CreatedAt    int64           `json:"created_at"`
	UpdatedAt    int64           `json:"updated_at"`
	Phases       []phaseViewResp `json:"phases"`
}

// phaseViewResp 展示字段(seq 起;无内部 id/plan_id)。
type phaseViewResp struct {
	Seq        int64  `json:"seq"`
	Kind       string `json:"kind"`
	Title      string `json:"title"`
	Allocator  string `json:"allocator"`
	Status     string `json:"status"`
	Evidence   string `json:"evidence"`
	Note       string `json:"note"`
	StartedAt  *int64 `json:"started_at"`
	FinishedAt *int64 `json:"finished_at"`
}

type taskCreateReq struct {
	CompanyID          string `json:"company_id"`
	CapabilityID       string `json:"capability_id"`
	WorkflowID         string `json:"workflow_id"`
	AgentID            string `json:"agent_id"`
	Title              string `json:"title"`
	Description        string `json:"description"`
	ToolName           string `json:"tool_name"`
	Risk               string `json:"risk"`
	MaxAttempts        int64  `json:"max_attempts"`
	TimeoutSec         int64  `json:"timeout_sec"`
	Workspace          string `json:"workspace"`
	ParentTaskID       string `json:"parent_task_id"`
	WriterEndpointID   string `json:"writer_endpoint_id"`
	ReviewerEndpointID string `json:"reviewer_endpoint_id"`
	TestEndpointID     string `json:"test_endpoint_id"`
}

func (s *Server) apiCreateTask(w http.ResponseWriter, r *http.Request) {
	var req taskCreateReq
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.CompanyID == "" || req.Title == "" {
		apiErr(w, http.StatusBadRequest, "bad_request", "company_id and title are required")
		return
	}
	t, err := s.svc.CreateTaskAs(r.Context(), service.TaskParams{
		CompanyID: req.CompanyID, CapabilityID: strPtr(req.CapabilityID), WorkflowID: strPtr(req.WorkflowID),
		AgentID: strPtr(req.AgentID), Title: req.Title, Description: req.Description,
		ToolName: req.ToolName, Risk: req.Risk, MaxAttempts: req.MaxAttempts, TimeoutSec: req.TimeoutSec,
		Workspace: req.Workspace, ParentTaskID: strPtr(req.ParentTaskID),
		WriterEndpointID: strPtr(req.WriterEndpointID), ReviewerEndpointID: strPtr(req.ReviewerEndpointID),
		TestEndpointID: strPtr(req.TestEndpointID),
	}, consoleActor)
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	apiCreated(w, t)
}

// ---- 审批 ----

func (s *Server) apiListApprovals(w http.ResponseWriter, r *http.Request) {
	list, err := s.svc.ListApprovals(r.Context(), queryParam(r, "status"))
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	apiOK(w, list)
}

func (s *Server) apiGetApproval(w http.ResponseWriter, r *http.Request) {
	a, err := s.svc.GetApproval(r.Context(), pathParam(r, "id"))
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	apiOK(w, a)
}

func (s *Server) apiDecideApproval(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Decision string `json:"decision"`
		Note     string `json:"note"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	switch req.Decision {
	case "approve", "reject", "changes":
	default:
		apiErr(w, http.StatusBadRequest, "bad_request", "decision must be approve|reject|changes")
		return
	}
	a, err := s.svc.DecideApprovalAs(r.Context(), pathParam(r, "id"), req.Decision, req.Note, consoleActor)
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	apiOK(w, a)
}

// ---- 决策 / 记忆 / 审计 ----

func (s *Server) apiListDecisions(w http.ResponseWriter, r *http.Request) {
	list, err := s.svc.ListDecisions(r.Context(), pathParam(r, "id"), queryParam(r, "kind"))
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	apiOK(w, list)
}

func (s *Server) apiCreateDecision(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Kind   string `json:"kind"`
		Status string `json:"status"`
		Title  string `json:"title"`
		Body   string `json:"body"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if pathParam(r, "id") == "" || req.Kind == "" || req.Title == "" {
		apiErr(w, http.StatusBadRequest, "bad_request", "company_id, kind and title are required")
		return
	}
	if req.Status == "" {
		req.Status = "made"
	}
	d, err := s.svc.CreateDecisionAs(r.Context(), pathParam(r, "id"), req.Kind, req.Status, req.Title, req.Body, consoleActor)
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	apiCreated(w, d)
}

func (s *Server) apiGetDecision(w http.ResponseWriter, r *http.Request) {
	d, err := s.svc.GetDecision(r.Context(), pathParam(r, "id"))
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	apiOK(w, d)
}

func (s *Server) apiListMemories(w http.ResponseWriter, r *http.Request) {
	list, err := s.svc.ListMemories(r.Context(), pathParam(r, "id"), queryParam(r, "type"))
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	apiOK(w, list)
}

func (s *Server) apiSearchMemories(w http.ResponseWriter, r *http.Request) {
	q := queryParam(r, "q")
	if q == "" {
		apiErr(w, http.StatusBadRequest, "bad_request", "q is required")
		return
	}
	list, err := s.svc.SearchMemories(r.Context(), pathParam(r, "id"), q)
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	apiOK(w, list)
}

func (s *Server) apiCreateMemory(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Type    string `json:"type"`
		Title   string `json:"title"`
		Content string `json:"content"`
		Source  string `json:"source"`
		Tags    string `json:"tags"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if pathParam(r, "id") == "" || req.Type == "" || req.Title == "" {
		apiErr(w, http.StatusBadRequest, "bad_request", "company_id, type and title are required")
		return
	}
	m, err := s.svc.CreateMemoryAs(r.Context(), pathParam(r, "id"), req.Type, req.Title, req.Content, req.Source, req.Tags, consoleActor)
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	apiCreated(w, m)
}

func (s *Server) apiListAudits(w http.ResponseWriter, r *http.Request) {
	list, err := s.svc.ListAudits(r.Context(), queryParam(r, "entity"))
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	apiOK(w, list)
}

// ---- 模型端点池 / 通道 B ----

func (s *Server) apiListEndpoints(w http.ResponseWriter, r *http.Request) {
	list, err := s.svc.ListEndpoints(r.Context(), pathParam(r, "id"))
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	apiOK(w, list)
}

func (s *Server) apiGetEndpoint(w http.ResponseWriter, r *http.Request) {
	e, err := s.svc.GetEndpoint(r.Context(), pathParam(r, "id"))
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	apiOK(w, e)
}

func (s *Server) apiAddEndpoint(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CompanyID string `json:"company_id"`
		Name      string `json:"name"`
		BaseURL   string `json:"base_url"`
		Token     string `json:"token"`
		Proto     string `json:"proto"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.CompanyID == "" || req.Name == "" || req.BaseURL == "" {
		apiErr(w, http.StatusBadRequest, "bad_request", "company_id, name and base_url are required")
		return
	}
	e, err := s.svc.AddEndpointAs(r.Context(), req.CompanyID, req.Name, req.BaseURL, req.Token, req.Proto, consoleActor)
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	apiCreated(w, e)
}

func (s *Server) apiSelectEndpointModel(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Model string `json:"model"`
		Role  string `json:"role"`
		Tier  string `json:"tier"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	e, err := s.svc.SelectEndpointModelAs(r.Context(), pathParam(r, "id"), req.Model, req.Role, req.Tier, consoleActor)
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	apiOK(w, e)
}

func (s *Server) apiFetchEndpointModels(w http.ResponseWriter, r *http.Request) {
	models, err := s.svc.FetchEndpointModelsAs(r.Context(), pathParam(r, "id"), consoleActor)
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	apiOK(w, models)
}

func (s *Server) apiIntakeSync(w http.ResponseWriter, r *http.Request) {
	results, err := s.svc.SyncRepos(r.Context(), pathParam(r, "id"))
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	apiOK(w, results)
}

func parseAttempt(s string) (int64, error) {
	if s == "" {
		return 0, nil
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n < 0 {
		return 0, errors.New("attempt must be a non-negative integer")
	}
	return n, nil
}
