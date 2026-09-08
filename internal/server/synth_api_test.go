package server

// Phase 10.4 HTTP 契约用例(run-synthesis.md §五,Vsrv-1;server 层 = 真 svc + 真 http,
// live 默认引擎 env seam 关 → run 只建单不执行,聚焦读端点形状 + plan_policy 回显):
//   - 无令牌 → 401(受控读全挡)
//   - POST 建 synthesize 流水线 → 201 echo PlanPolicy=synthesize;GET 列表同款回显
//   - plan_policy 非法 → 400 bad_request;缺省(不传)→ adaptive 零漂移
//   - synthesize run → GET /tasks/{id}/plan → plan.kind=engineering + plan_policy=synthesize
//     + materialized=grow + phases 空(未认领诚实空态;合成只发生在 worker 首次认领,server 不执行)

import (
	"context"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/glacierzzz26/one-person-company-os/internal/pipeline"
	"github.com/glacierzzz26/one-person-company-os/internal/project"
	"github.com/glacierzzz26/one-person-company-os/internal/task"
)

// synthPlanReadMirror 10.4 计划读镜像(带 additive plan_policy,不依赖 plan_api_test.go 的类型)。
type synthPlanReadMirror struct {
	TaskID string           `json:"task_id"`
	Plan   *synthPlanMirror `json:"plan"`
}

type synthPlanMirror struct {
	Kind         string `json:"kind"`
	Materialized string `json:"materialized"`
	PlanPolicy   string `json:"plan_policy"`
}

type synthPipelineMirror struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	PlanPolicy string `json:"plan_policy"`
}

type synthRunEnvelope struct {
	TaskID     string    `json:"task_id"`
	PipelineID string    `json:"pipeline_id"`
	ProjectID  *string   `json:"project_id"`
	Task       task.Task `json:"task"`
}

func TestVsrv1SynthPipelinePlanPolicyHTTP(t *testing.T) {
	srv, st := newSetupServer(t)
	h := srv.Handler()
	if rec, env := doAPI(t, h, http.MethodPost, "/api/v1/setup", `{"console_token":"sekret-token"}`); rec.Code != http.StatusOK || !env.OK {
		t.Fatalf("setup: code=%d env=%+v", rec.Code, env)
	}
	token := "sekret-token"
	comp := seedCompany(t, st, "ACME", "")
	seedEngineEndpoints(t, st, comp.ID) // live 建单 8.4 默认落槽;run 不触网不执行

	// 无令牌 → 401(受控读全挡)。
	if rec, _ := doAPI(t, h, http.MethodGet, "/api/v1/companies/"+comp.ID+"/projects", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("no-token list: code=%d, want 401", rec.Code)
	}

	// 建项目(nonexistent root → OS mkdir+git init)。
	root := filepath.Join(t.TempDir(), "synth-repo")
	rec, env := doBearer(t, h, http.MethodPost, "/api/v1/companies/"+comp.ID+"/projects", token,
		`{"name":"synth-app","root_path":"`+root+`","description":"synthesized plan demo"}`)
	if rec.Code != http.StatusCreated || !env.OK {
		t.Fatalf("create project: code=%d env=%+v body=%s", rec.Code, env, rec.Body.String())
	}
	var prj project.Project
	decodeData(t, env, &prj)
	if prj.ID == "" || prj.RootPath != root {
		t.Fatalf("project = %+v", prj)
	}

	// ---- 建 synthesize 流水线 → 201 回显 plan_policy=synthesize ----
	rec, env = doBearer(t, h, http.MethodPost, "/api/v1/projects/"+prj.ID+"/pipelines", token,
		`{"name":"login-fix-synth","kind":"bugfix","description":"fix flaky login","risk":"medium","plan_policy":"synthesize"}`)
	if rec.Code != http.StatusCreated || !env.OK {
		t.Fatalf("create synth pipeline: code=%d env=%+v body=%s", rec.Code, env, rec.Body.String())
	}
	var plSynth pipeline.Pipeline
	decodeData(t, env, &plSynth)
	if plSynth.PlanPolicy != pipeline.PlanPolicySynthesize {
		t.Fatalf("create echo plan_policy=%q, want synthesize", plSynth.PlanPolicy)
	}

	// GET 列表回显同款 plan_policy。
	rec, env = doBearer(t, h, http.MethodGet, "/api/v1/projects/"+prj.ID+"/pipelines", token, "")
	if rec.Code != http.StatusOK || !env.OK {
		t.Fatalf("list pipelines: code=%d env=%+v", rec.Code, env)
	}
	var pls []synthPipelineMirror
	decodeData(t, env, &pls)
	if len(pls) != 1 || pls[0].ID != plSynth.ID || pls[0].PlanPolicy != "synthesize" {
		t.Fatalf("pipelines = %+v, want 1 synthesize row", pls)
	}

	// plan_policy 非法 → 400 bad_request(白名单服务层拒,不落库)。
	rec, env = doBearer(t, h, http.MethodPost, "/api/v1/projects/"+prj.ID+"/pipelines", token,
		`{"name":"bad-policy","kind":"bugfix","plan_policy":"whack"}`)
	if rec.Code != http.StatusBadRequest || env.Error == nil || env.Error.Code != "bad_request" {
		t.Fatalf("invalid plan_policy: code=%d env=%+v, want 400 bad_request", rec.Code, env)
	}

	// ---- synthesize run → 建单信封 → GET plan:grow + plan_policy=synthesize(诚实空态)----
	rec, env = doBearer(t, h, http.MethodPost, "/api/v1/pipelines/"+plSynth.ID+"/run", token, `{}`)
	if rec.Code != http.StatusOK || !env.OK {
		t.Fatalf("run synth pipeline: code=%d env=%+v body=%s", rec.Code, env, rec.Body.String())
	}
	var runE synthRunEnvelope
	decodeData(t, env, &runE)
	if runE.TaskID == "" || runE.PipelineID != plSynth.ID || runE.ProjectID == nil || *runE.ProjectID != prj.ID {
		t.Fatalf("run envelope = %+v", runE)
	}
	if runE.Task.ToolName != "engineering" || runE.Task.WorkspacePath != root {
		t.Fatalf("run task = %+v, want engineering in project root", runE.Task)
	}

	// GET plan:kind=engineering + plan_policy=synthesize;未认领 → materialized=grow(合成仅在 worker 首次认领)。
	rec, env = doBearer(t, h, http.MethodGet, "/api/v1/tasks/"+runE.TaskID+"/plan", token, "")
	if rec.Code != http.StatusOK || !env.OK {
		t.Fatalf("GET synth plan: code=%d env=%+v body=%s", rec.Code, env, rec.Body.String())
	}
	var pd synthPlanReadMirror
	decodeData(t, env, &pd)
	if pd.Plan == nil {
		t.Fatalf("synth run plan nil (must exist via pipeline ref)")
	}
	if pd.Plan.Kind != "engineering" || pd.Plan.PlanPolicy != "synthesize" || pd.Plan.Materialized != "grow" {
		t.Fatalf("synth plan = %+v, want kind=engineering plan_policy=synthesize materialized=grow(未认领诚实空态)", *pd.Plan)
	}

	// ---- 无 policy 流水线(缺省)→ adaptive 零漂移 ----
	// 同项目串行守卫:先收尾 synth run 任务(server 不执行,直铺完成态),否则 409。
	if _, err := st.CompleteTask(context.Background(), runE.TaskID, "done (fixture)"); err != nil {
		t.Fatalf("complete synth run: %v", err)
	}
	rec, env = doBearer(t, h, http.MethodPost, "/api/v1/projects/"+prj.ID+"/pipelines", token,
		`{"name":"login-fix-adaptive","kind":"bugfix","description":"plain fix","risk":"medium"}`)
	if rec.Code != http.StatusCreated || !env.OK {
		t.Fatalf("create adaptive pipeline: code=%d env=%+v", rec.Code, env)
	}
	var plAdapt pipeline.Pipeline
	decodeData(t, env, &plAdapt)
	if plAdapt.PlanPolicy != pipeline.PlanPolicyAdaptive {
		t.Fatalf("default plan_policy=%q, want adaptive(零漂移)", plAdapt.PlanPolicy)
	}
	rec, env = doBearer(t, h, http.MethodPost, "/api/v1/pipelines/"+plAdapt.ID+"/run", token, `{}`)
	if rec.Code != http.StatusOK || !env.OK {
		t.Fatalf("run adaptive pipeline: code=%d env=%+v", rec.Code, env)
	}
	var runA synthRunEnvelope
	decodeData(t, env, &runA)
	rec, env = doBearer(t, h, http.MethodGet, "/api/v1/tasks/"+runA.TaskID+"/plan", token, "")
	if rec.Code != http.StatusOK || !env.OK {
		t.Fatalf("GET adaptive plan: code=%d env=%+v", rec.Code, env)
	}
	var pdA synthPlanReadMirror
	decodeData(t, env, &pdA)
	if pdA.Plan == nil || pdA.Plan.PlanPolicy != "adaptive" {
		t.Fatalf("adaptive plan = %+v, want plan_policy=adaptive", pdA.Plan)
	}
}
