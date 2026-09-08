package server

// Phase 10.1 HTTP 契约用例(project-pipeline-foundation.md §五,Web/console actor 侧;CLI actor 见 service 层 P13):
//   - 建项目 201(git 现成/空/nonexistent);非空非 git → 400 bad_request;重名 → 409 conflict
//   - project/pipeline 读写信封 + 快照字段;run → {task_id, project_id, pipeline_id, task}
//   - run 挂 project 目录 + engineering;同 project 活跃 run 再跑 → 409 conflict;完成态不阻塞
//   - delete pipeline(历史 run 保留);delete project 受控(活跃 → 409;无活跃 → 删元数据,磁盘不动)
//   - 审计 actor = human:console 可见
//
// server 包无 env seam(9.4:产品恒关)→ 模式走 DB 默认 live:run 建单须配齐判读档端点(8.4),
// 行存在即可(建单不触网;执行不在本层触发)。

import (
	"context"
	"encoding/hex"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/glacierzzz26/one-person-company-os/internal/endpoint"
	"github.com/glacierzzz26/one-person-company-os/internal/pipeline"
	"github.com/glacierzzz26/one-person-company-os/internal/project"
	"github.com/glacierzzz26/one-person-company-os/internal/storage/repository"
	"github.com/glacierzzz26/one-person-company-os/internal/task"
	"github.com/google/uuid"
)

// seedEngineEndpoints 给公司配齐 8.4 判读默认档端点(live 建单默认解析用,不触网)。
func seedEngineEndpoints(t *testing.T, st *repository.Store, compID string) {
	t.Helper()
	key, _ := hex.DecodeString("00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff")
	tok, err := endpoint.SealTokenWith(key, "sk-project-test")
	if err != nil {
		t.Fatalf("seal token: %v", err)
	}
	now := time.Now().Unix()
	mk := func(name, tier string) {
		if _, err := st.CreateEndpoint(context.Background(), endpoint.Endpoint{
			ID: uuid.NewString(), CompanyID: compID, Name: name,
			BaseURL: "http://127.0.0.1:1", TokenEnc: tok, Proto: "openai", Vendor: "gateway",
			Tier: tier, SelectedModel: "judge-model", Role: "pool", Status: "active",
			CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatalf("seed judge endpoint %s: %v", tier, err)
		}
	}
	mk("judge-frontier", "frontier")
	mk("judge-standard", "standard")
}

func TestAPIProjectsPipelines(t *testing.T) {
	srv, st := newTestServer(t)
	comp := seedCompany(t, st, "ACME", "")

	root := filepath.Join(t.TempDir(), "web-repo")

	// 建项目:nonexistent root → 201 + OS mkdir/git init(目录落盘);stored root = abs。
	rec, env := doAPI(t, srv.Handler(), http.MethodPost, "/api/v1/companies/"+comp.ID+"/projects",
		`{"name":"acme-web","root_path":"`+root+`","description":"one person web"}`)
	if rec.Code != http.StatusCreated || !env.OK {
		t.Fatalf("create project: code=%d ok=%v err=%+v body=%s", rec.Code, env.OK, env.Error, rec.Body.String())
	}
	var prj project.Project
	decodeData(t, env, &prj)
	if prj.ID == "" || prj.Name != "acme-web" {
		t.Fatalf("project = %+v", prj)
	}
	if _, err := os.Stat(prj.RootPath); err != nil {
		t.Fatalf("root should have been created on disk: %v", err)
	}

	// 列表含该项目;GET 单项目一致。
	rec, env = doAPI(t, srv.Handler(), http.MethodGet, "/api/v1/companies/"+comp.ID+"/projects", "")
	if rec.Code != http.StatusOK || !env.OK {
		t.Fatalf("list projects: code=%d env=%+v", rec.Code, env)
	}
	var prjs []project.Project
	decodeData(t, env, &prjs)
	if len(prjs) != 1 || prjs[0].RootPath != prj.RootPath {
		t.Fatalf("projects = %+v, want 1 row with root %s", prjs, prj.RootPath)
	}
	rec, env = doAPI(t, srv.Handler(), http.MethodGet, "/api/v1/projects/"+prj.ID, "")
	if rec.Code != http.StatusOK || !env.OK {
		t.Fatalf("get project: code=%d env=%+v", rec.Code, env)
	}

	// 非空非 git 目录 → 400 bad_request;重名 → 409 conflict。
	bad := filepath.Join(t.TempDir(), "content")
	if err := os.MkdirAll(bad, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bad, "keep.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec, env = doAPI(t, srv.Handler(), http.MethodPost, "/api/v1/companies/"+comp.ID+"/projects",
		`{"name":"bad-root","root_path":"`+bad+`"}`)
	if rec.Code != http.StatusBadRequest || env.Error == nil || env.Error.Code != "bad_request" {
		t.Fatalf("non-empty non-git root: code=%d env=%+v, want 400 bad_request", rec.Code, env)
	}
	rec, env = doAPI(t, srv.Handler(), http.MethodPost, "/api/v1/companies/"+comp.ID+"/projects",
		`{"name":"acme-web","root_path":"`+filepath.Join(t.TempDir(), "other")+`"}`)
	if rec.Code != http.StatusConflict || env.Error == nil || env.Error.Code != "conflict" {
		t.Fatalf("duplicate project name: code=%d env=%+v, want 409 conflict", rec.Code, env)
	}

	// 建流水线(bugfix)+ 列表。
	rec, env = doAPI(t, srv.Handler(), http.MethodPost, "/api/v1/projects/"+prj.ID+"/pipelines",
		`{"name":"fix-login","kind":"bugfix","description":"登录失败要可恢复","risk":"medium"}`)
	if rec.Code != http.StatusCreated || !env.OK {
		t.Fatalf("create pipeline: code=%d env=%+v body=%s", rec.Code, env, rec.Body.String())
	}
	var pl pipeline.Pipeline
	decodeData(t, env, &pl)
	if pl.ProjectID != prj.ID || pl.Kind != pipeline.KindBugfix || pl.Status != pipeline.StatusActive {
		t.Fatalf("pipeline = %+v", pl)
	}
	rec, env = doAPI(t, srv.Handler(), http.MethodGet, "/api/v1/projects/"+prj.ID+"/pipelines", "")
	var pls []pipeline.Pipeline
	decodeData(t, env, &pls)
	if rec.Code != http.StatusOK || len(pls) != 1 {
		t.Fatalf("pipelines = %+v code=%d", pls, rec.Code)
	}

	// run:live 判读档端点配齐 → 201 建单信封 {task_id, project_id, pipeline_id, task}。
	seedEngineEndpoints(t, st, comp.ID)
	rec, env = doAPI(t, srv.Handler(), http.MethodPost, "/api/v1/pipelines/"+pl.ID+"/run", `{}`)
	if rec.Code != http.StatusOK || !env.OK {
		t.Fatalf("run pipeline: code=%d ok=%v err=%+v body=%s", rec.Code, env.OK, env.Error, rec.Body.String())
	}
	var run struct {
		TaskID     string    `json:"task_id"`
		ProjectID  *string   `json:"project_id"`
		PipelineID string    `json:"pipeline_id"`
		Task       task.Task `json:"task"`
	}
	decodeData(t, env, &run)
	if run.TaskID == "" || run.PipelineID != pl.ID || run.ProjectID == nil || *run.ProjectID != prj.ID {
		t.Fatalf("run envelope = %+v", run)
	}
	tsk := run.Task
	if tsk.ProjectID == nil || *tsk.ProjectID != prj.ID || tsk.WorkspacePath != prj.RootPath ||
		tsk.ToolName != "engineering" || tsk.Title != pl.Name || tsk.Description != pl.Description {
		t.Fatalf("run task = %+v, want project-scoped engineering task in root", tsk)
	}

	// 同 project 活跃 run 再触发 → 409 conflict(串行守卫)。
	rec, env = doAPI(t, srv.Handler(), http.MethodPost, "/api/v1/pipelines/"+pl.ID+"/run", `{"request":"again"}`)
	if rec.Code != http.StatusConflict || env.Error == nil || env.Error.Code != "conflict" {
		t.Fatalf("serial guard: code=%d env=%+v, want 409 conflict", rec.Code, env)
	}

	// 完成历史 run → 不阻塞;request 覆盖 description。
	if _, err := st.CompleteTask(context.Background(), tsk.ID, "done"); err != nil {
		t.Fatalf("complete: %v", err)
	}
	rec, env = doAPI(t, srv.Handler(), http.MethodPost, "/api/v1/pipelines/"+pl.ID+"/run", `{"request":"紧急再跑一次"}`)
	if rec.Code != http.StatusOK || !env.OK {
		t.Fatalf("re-run: code=%d env=%+v", rec.Code, env)
	}
	decodeData(t, env, &run)
	if run.Task.Description != "紧急再跑一次" {
		t.Fatalf("run2 description = %q, want request override", run.Task.Description)
	}

	// 项目最近 runs(2 条,倒序)。
	rec, env = doAPI(t, srv.Handler(), http.MethodGet, "/api/v1/projects/"+prj.ID+"/tasks?limit=5", "")
	var runs []task.Task
	decodeData(t, env, &runs)
	if rec.Code != http.StatusOK || len(runs) != 2 {
		t.Fatalf("project tasks = %d, want 2 (code=%d)", len(runs), rec.Code)
	}

	// delete pipeline:历史 run 保留、项目仍在。
	rec, env = doAPI(t, srv.Handler(), http.MethodDelete, "/api/v1/pipelines/"+pl.ID, "")
	if rec.Code != http.StatusOK || !env.OK {
		t.Fatalf("delete pipeline: code=%d env=%+v", rec.Code, env)
	}
	if _, err := st.GetTask(context.Background(), tsk.ID); err != nil {
		t.Fatalf("pipeline delete must keep its run tasks: %v", err)
	}
	if _, err := st.GetProject(context.Background(), prj.ID); err != nil {
		t.Fatalf("pipeline delete must keep project: %v", err)
	}

	// delete project:活跃 run → 409;无活跃 → 200 删元数据,磁盘目录仍在。
	rec, env = doAPI(t, srv.Handler(), http.MethodDelete, "/api/v1/projects/"+prj.ID, "")
	if rec.Code != http.StatusConflict || env.Error == nil || env.Error.Code != "conflict" {
		t.Fatalf("delete busy project: code=%d env=%+v, want 409", rec.Code, env)
	}
	if _, err := st.CompleteTask(context.Background(), run.TaskID, "done"); err != nil {
		t.Fatalf("complete run2: %v", err)
	}
	rec, env = doAPI(t, srv.Handler(), http.MethodDelete, "/api/v1/projects/"+prj.ID, "")
	if rec.Code != http.StatusOK || !env.OK {
		t.Fatalf("delete idle project: code=%d env=%+v", rec.Code, env)
	}
	if _, err := os.Stat(prj.RootPath); err != nil {
		t.Fatalf("project delete must not touch disk directory: %v", err)
	}

	// 审计 actor = human:console(pipeline create/run/delete + task create 各链)。
	audits, err := srv.svc.ListAudits(context.Background(), "pipeline")
	if err != nil {
		t.Fatalf("list pipeline audits: %v", err)
	}
	want := map[string]bool{"create": false, "run": false, "delete": false}
	for _, a := range audits {
		if a.Actor != "human:console" {
			t.Fatalf("pipeline audit actor = %q, want human:console (action %s)", a.Actor, a.Action)
		}
		if a.Action == "create" || a.Action == "run" || a.Action == "delete" {
			want[a.Action] = true
		}
	}
	for act, seen := range want {
		if !seen {
			t.Fatalf("pipeline audit missing %q (console)", act)
		}
	}
}
