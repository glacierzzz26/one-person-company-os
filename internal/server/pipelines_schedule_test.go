package server

// Phase 10.2 契约用例(ops-patrol-schedule.md §五,Srv-*;server 无 env seam → 公司 DB 默认 live:
// 建单须配齐 8.4 判读档端点,行存在即可不触网;执行验证在 service/scripted 层)。
//   - Srv-1 PUT /pipelines/{id} schedule 200 + read-back + audit human:console;非法 → 400;不存在 → 404
//   - Srv-2 建 ops_patrol(带 cron)+ run(判读档端点配齐)→ 信封 task.pipeline_id=流水线 id;活跃再 run → 409
//   - Srv-3 scheduleOnce 单 tick:seed 到点 cron(`* * * * *`,scheduleNext 预置过去)→ 触发建单 + audit
//     system:schedule;同一次命中不双发(advance 后 next 未来);SchedulePoll=0 → ScheduleLoop 即返
//   - Srv-4 GET /projects/{id}/patrol/{taskID}:scripted 完成的巡检 run(报告已落盘)→ 200 正文;非巡检 run /
//     未完成 / project 不匹配 → 404/400;超大报告截断标记

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/glacierzzz26/one-person-company-os/internal/pipeline"
	"github.com/glacierzzz26/one-person-company-os/internal/service"
	"github.com/glacierzzz26/one-person-company-os/internal/task"
)

// makeProjViaAPI 经 HTTP 建项目(nonexistent root → OS mkdir/git init),返回服务端快照。
func makeProjViaAPI(t *testing.T, srv *Server, compID, name string) projectSnap {
	t.Helper()
	h := srv.Handler()
	root := filepath.Join(t.TempDir(), name)
	rec, env := doAPI(t, h, http.MethodPost, "/api/v1/companies/"+compID+"/projects",
		`{"name":"`+name+`","root_path":"`+root+`","description":"srv seed"}`)
	if rec.Code != http.StatusCreated || !env.OK {
		t.Fatalf("create project %s: code=%d env=%+v", name, rec.Code, env)
	}
	var p projectSnap
	decodeData(t, env, &p)
	return p
}

// makePipeViaAPI 经 HTTP 建流水线(返回服务端快照)。
func makePipeViaAPI(t *testing.T, srv *Server, prjID, name, kind, schedule string) pipeline.Pipeline {
	t.Helper()
	rec, env := doAPI(t, srv.Handler(), http.MethodPost, "/api/v1/projects/"+prjID+"/pipelines",
		`{"name":"`+name+`","kind":"`+kind+`","description":"checks: hygiene+build+lockfile","risk":"medium","schedule":"`+schedule+`"}`)
	if rec.Code != http.StatusCreated || !env.OK {
		t.Fatalf("create pipeline %s: code=%d env=%+v", name, rec.Code, env)
	}
	var pl pipeline.Pipeline
	decodeData(t, env, &pl)
	return pl
}

// runViaAPI 经 HTTP 触发一次 run,返回 {task, pipeline_id, project_id}。
func runViaAPI(t *testing.T, srv *Server, plID, body string) (task.Task, string) {
	t.Helper()
	rec, env := doAPI(t, srv.Handler(), http.MethodPost, "/api/v1/pipelines/"+plID+"/run", body)
	if rec.Code != http.StatusOK || !env.OK {
		t.Fatalf("run pipeline %s: code=%d env=%+v body=%s", plID, rec.Code, env, rec.Body.String())
	}
	var run struct {
		TaskID     string    `json:"task_id"`
		PipelineID string    `json:"pipeline_id"`
		Task       task.Task `json:"task"`
	}
	decodeData(t, env, &run)
	return run.Task, run.PipelineID
}

// completePatrol 手动把 run 变成「完成的巡检」:报告真落盘 + st.CompleteTask(result 带 patrol: 前缀)。
func completePatrol(t *testing.T, st storeLike, ws, taskID, content string) {
	t.Helper()
	dir := filepath.Join(ws, service.PatrolDirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, taskID+".md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	rel := filepath.Join(service.PatrolDirName, taskID+".md")
	result := service.PatrolResultTag + filepath.ToSlash(rel) + "|ok=false|severity=high|action=fix|drift found"
	if _, err := st.CompleteTask(context.Background(), taskID, result); err != nil {
		t.Fatalf("complete patrol run: %v", err)
	}
}

// ---- Srv-1:PUT /pipelines/{id} schedule(改调度 + 审计;非法 400 / 不存在 404)----

func TestSrv1UpdateScheduleEndpoint(t *testing.T) {
	srv, st := newTestServer(t)
	ctx := context.Background()
	comp := seedCompany(t, st, "ACME", "")
	prj := makeProjViaAPI(t, srv, comp.ID, "web")
	pl := makePipeViaAPI(t, srv, prj.ID, "fix", string(pipeline.KindBugfix), "")

	// PUT schedule → 200,read-back schedule;审计 actor=human:console(detail 记 old → new)。
	rec, env := doAPI(t, srv.Handler(), http.MethodPut, "/api/v1/pipelines/"+pl.ID, `{"schedule":"0 9 * * *"}`)
	if rec.Code != http.StatusOK || !env.OK {
		t.Fatalf("Srv1 put schedule: code=%d env=%+v body=%s", rec.Code, env, rec.Body.String())
	}
	var upd pipeline.Pipeline
	decodeData(t, env, &upd)
	if upd.Schedule != "0 9 * * *" {
		t.Fatalf("Srv1 schedule = %q, want 0 9 * * *", upd.Schedule)
	}
	audits, err := srv.svc.ListAudits(ctx, "pipeline")
	if err != nil {
		t.Fatal(err)
	}
	var editFound bool
	for _, a := range audits {
		if a.EntityID == pl.ID && a.Action == "edit" {
			if a.Actor != "human:console" {
				t.Fatalf("Srv1 edit actor = %q, want human:console", a.Actor)
			}
			if !strings.Contains(a.Detail, " → 0 9 * * *") {
				t.Fatalf("Srv1 edit detail = %q, want old → new", a.Detail)
			}
			editFound = true
		}
	}
	if !editFound {
		t.Fatalf("Srv1 pipeline edit audit not found")
	}

	// 非法 cron → 400 bad_request(不落库);不存在 → 404。
	rec, env = doAPI(t, srv.Handler(), http.MethodPut, "/api/v1/pipelines/"+pl.ID, `{"schedule":"60 * * * *"}`)
	if rec.Code != http.StatusBadRequest || env.Error == nil || env.Error.Code != "bad_request" {
		t.Fatalf("Srv1 invalid schedule: code=%d env=%+v, want 400 bad_request", rec.Code, env)
	}
	rec, env = doAPI(t, srv.Handler(), http.MethodPut, "/api/v1/pipelines/no-such-pipeline", `{"schedule":"0 9 * * *"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("Srv1 missing pipeline: code=%d env=%+v, want 404", rec.Code, env)
	}
}

// ---- Srv-2:建 ops_patrol(带 cron)+ run → pipeline_id;活跃再 run → 409 ----

func TestSrv2OpsPatrolRunEndpoint(t *testing.T) {
	srv, st := newTestServer(t)
	comp := seedCompany(t, st, "ACME", "")
	prj := makeProjViaAPI(t, srv, comp.ID, "web")

	// 建 ops_patrol 带 cron → 201,read-back schedule。
	pl := makePipeViaAPI(t, srv, prj.ID, "patrol-daily", string(pipeline.KindOpsPatrol), "0 9 * * *")
	if pl.Kind != pipeline.KindOpsPatrol || pl.Schedule != "0 9 * * *" {
		t.Fatalf("Srv2 ops_patrol = kind %q schedule %q", pl.Kind, pl.Schedule)
	}
	if pl.ProjectID != prj.ID {
		t.Fatalf("Srv2 pipeline project = %q, want %q", pl.ProjectID, prj.ID)
	}

	// run(判读档端点配齐)→ 信封 task.pipeline_id=流水线 id、project 归属。
	seedEngineEndpoints(t, st, comp.ID)
	tk, plID := runViaAPI(t, srv, pl.ID, `{}`)
	if plID != pl.ID {
		t.Fatalf("Srv2 run pipeline_id = %q, want %q", plID, pl.ID)
	}
	if tk.PipelineID == nil || *tk.PipelineID != pl.ID {
		t.Fatalf("Srv2 task.pipeline_id = %v, want %s", tk.PipelineID, pl.ID)
	}
	if tk.ProjectID == nil || *tk.ProjectID != prj.ID {
		t.Fatalf("Srv2 task.project_id = %v, want %s", tk.ProjectID, prj.ID)
	}
	// 活跃再 run → 409 conflict(串行守卫)。
	rec, env := doAPI(t, srv.Handler(), http.MethodPost, "/api/v1/pipelines/"+pl.ID+"/run", `{}`)
	if rec.Code != http.StatusConflict || env.Error == nil || env.Error.Code != "conflict" {
		t.Fatalf("Srv2 busy re-run: code=%d env=%+v, want 409 conflict", rec.Code, env)
	}
}

// ---- Srv-3:scheduleOnce 单 tick(到点触发 + 审计;advance 不双发;poll=0 即返)----

func TestSrv3ScheduleOnceTick(t *testing.T) {
	srv, st := newTestServer(t)
	ctx := context.Background()
	comp := seedCompany(t, st, "ACME", "")
	seedEngineEndpoints(t, st, comp.ID)

	prj, err := srv.svc.CreateProject(ctx, comp.ID, "web", filepath.Join(t.TempDir(), "web"), "", "")
	if err != nil {
		t.Fatalf("Srv3 create project: %v", err)
	}
	pl, err := srv.svc.CreatePipeline(ctx, prj.ID, "patrol-every-min", pipeline.KindOpsPatrol,
		"checks: hygiene+build", pipeline.RiskLow, "* * * * *")
	if err != nil {
		t.Fatalf("Srv3 create scheduled pipeline: %v", err)
	}

	// seed 到点:scheduleNext 预置为「已过的整分边界」(cron 只在整分命中)→ 单 tick 应触发建单。
	start := time.Now()
	srv.scheduleNext = map[string]time.Time{pl.ID: start.Truncate(time.Minute)}
	srv.scheduleOnce(ctx)

	tasks, err := srv.svc.ListTasksByProject(ctx, prj.ID, 10)
	if err != nil || len(tasks) != 1 {
		t.Fatalf("Srv3 after due tick: tasks = %d (err=%v), want 1 fired run", len(tasks), err)
	}
	if tasks[0].PipelineID == nil || *tasks[0].PipelineID != pl.ID {
		t.Fatalf("Srv3 fired task.pipeline_id = %v, want %s", tasks[0].PipelineID, pl.ID)
	}
	// audit: pipeline run actor=system:schedule(调度 actor,非 human:console)。
	audits, err := srv.svc.ListAudits(ctx, "pipeline")
	if err != nil {
		t.Fatal(err)
	}
	runFound := false
	for _, a := range audits {
		if a.EntityID == pl.ID && a.Action == "run" && a.Actor == service.ActorSchedule {
			runFound = true
		}
	}
	if !runFound {
		t.Fatalf("Srv3 pipeline run audit actor=%s not found", service.ActorSchedule)
	}
	// advance 后 next 未来 → 同一次命中不双发。
	if next, ok := srv.scheduleNext[pl.ID]; !ok || !next.After(start) {
		t.Fatalf("Srv3 scheduleNext after fire = %v ok=%v, want future", next, ok)
	}
	srv.scheduleOnce(ctx)
	tasks2, _ := srv.svc.ListTasksByProject(ctx, prj.ID, 10)
	if len(tasks2) != 1 {
		t.Fatalf("Srv3 second tick must not double-fire: tasks = %d", len(tasks2))
	}

	// QueueWork 关不影响触发(SchedulePoll 独立):未开 queue,任务仅 pending 堆叠 = 自动拉单不自动干。
	if got, _ := srv.svc.GetTask(ctx, tasks2[0].ID); got.Status != "pending" {
		t.Fatalf("Srv3 fired run should stay pending (queue-work off): %s", got.Status)
	}

	// SchedulePollSec=0 → ScheduleLoop 即返(不开循环)。
	srv.SetSchedulePoll(0)
	if srv.ScheduleEnabled() {
		t.Fatal("Srv3 ScheduleEnabled should be false after SetSchedulePoll(0)")
	}
	srv.ScheduleLoop(ctx) // schedulePoll<=0 → 日志 + 立即返回,不阻塞
}

// ---- Srv-4:GET /projects/{id}/patrol/{taskID}(正文/越界门/截断)----

func TestSrv4GetPatrolReport(t *testing.T) {
	srv, st := newTestServer(t)
	ctx := context.Background()
	comp := seedCompany(t, st, "ACME", "")
	seedEngineEndpoints(t, st, comp.ID)

	prj := makeProjViaAPI(t, srv, comp.ID, "web")
	patrolPL := makePipeViaAPI(t, srv, prj.ID, "patrol", string(pipeline.KindOpsPatrol), "")
	fixPL := makePipeViaAPI(t, srv, prj.ID, "fix", string(pipeline.KindBugfix), "")
	// 第二项目(project 不匹配用例)。
	prj2 := makeProjViaAPI(t, srv, comp.ID, "web2")

	// (1) 完成的巡检 run(小报告)→ 200 正文 + truncated=false。
	runSmall, _ := runViaAPI(t, srv, patrolPL.ID, `{}`)
	smallContent := "# Patrol Report\n- check: hygiene\nNo findings.\n"
	completePatrol(t, st, prj.RootPath, runSmall.ID, smallContent)

	rec, env := doAPI(t, srv.Handler(), http.MethodGet,
		"/api/v1/projects/"+prj.ID+"/patrol/"+runSmall.ID, "")
	if rec.Code != http.StatusOK || !env.OK {
		t.Fatalf("Srv4 small report: code=%d env=%+v", rec.Code, env)
	}
	var rep struct {
		TaskID    string `json:"task_id"`
		Path      string `json:"path"`
		Truncated bool   `json:"truncated"`
		Content   string `json:"content"`
	}
	decodeData(t, env, &rep)
	if rep.TaskID != runSmall.ID || rep.Truncated || rep.Content != smallContent {
		t.Fatalf("Srv4 small report = task %s truncated=%v content=%q, want full body", rep.TaskID, rep.Truncated, rep.Content)
	}
	if rep.Path != filepath.ToSlash(filepath.Join(service.PatrolDirName, runSmall.ID+".md")) {
		t.Fatalf("Srv4 path = %q", rep.Path)
	}

	// (2) 超大报告 → 200 正文截断带标记(截到 PatrolReportCap)。
	runBig, _ := runViaAPI(t, srv, patrolPL.ID, `{}`)
	bigContent := strings.Repeat("drift evidence line\n", (int(service.PatrolReportCap)/18)+10)
	completePatrol(t, st, prj.RootPath, runBig.ID, bigContent)
	rec, env = doAPI(t, srv.Handler(), http.MethodGet,
		"/api/v1/projects/"+prj.ID+"/patrol/"+runBig.ID, "")
	if rec.Code != http.StatusOK || !env.OK {
		t.Fatalf("Srv4 big report: code=%d env=%+v", rec.Code, env)
	}
	decodeData(t, env, &rep)
	if !rep.Truncated || len(rep.Content) != int(service.PatrolReportCap) {
		t.Fatalf("Srv4 big report truncated=%v len=%d, want true + cap %d", rep.Truncated, len(rep.Content), service.PatrolReportCap)
	}

	// (3) 非巡检 run(普通 bugfix 完成)→ 400 bad_request。
	runFix, _ := runViaAPI(t, srv, fixPL.ID, `{}`)
	if _, err := st.CompleteTask(ctx, runFix.ID, "diff: one-line fix"); err != nil {
		t.Fatal(err)
	}
	rec, env = doAPI(t, srv.Handler(), http.MethodGet,
		"/api/v1/projects/"+prj.ID+"/patrol/"+runFix.ID, "")
	if rec.Code != http.StatusBadRequest || env.Error == nil || env.Error.Code != "bad_request" {
		t.Fatalf("Srv4 non-patrol run: code=%d env=%+v, want 400", rec.Code, env)
	}

	// (4) 未完成的巡检 run(pending)→ 400 bad_request(先于文件读)。
	runPending, _ := runViaAPI(t, srv, patrolPL.ID, `{}`)
	rec, env = doAPI(t, srv.Handler(), http.MethodGet,
		"/api/v1/projects/"+prj.ID+"/patrol/"+runPending.ID, "")
	if rec.Code != http.StatusBadRequest || env.Error == nil || env.Error.Code != "bad_request" {
		t.Fatalf("Srv4 pending run: code=%d env=%+v, want 400", rec.Code, env)
	}

	// (5) project 不匹配 → 404(不泄漏存在性);路径服务端推导,无客户端路径入参。
	rec, env = doAPI(t, srv.Handler(), http.MethodGet,
		"/api/v1/projects/"+prj2.ID+"/patrol/"+runSmall.ID, "")
	if rec.Code != http.StatusNotFound || env.Error == nil || env.Error.Code != "not_found" {
		t.Fatalf("Srv4 cross-project: code=%d env=%+v, want 404", rec.Code, env)
	}
}

// ---- server 测试内的小工具 ----

type storeLike interface {
	CompleteTask(ctx context.Context, taskID, result string) (task.Task, error)
}

type projectSnap struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	RootPath string `json:"root_path"`
}
