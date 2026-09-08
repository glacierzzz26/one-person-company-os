package server

// Phase 10.3 契约用例(phase-plan-contract.md §五,Tsrv-*;server 层 = 真 svc + 真 http,
// 信封/鉴权沿用;live 默认引擎(env seam 关)→ run 只建单不执行,阶段状态由 store 直铺,聚焦读端点形状):
//   - Tsrv-1 GET /tasks/{id}/plan:
//       patrol 预铺完成 run → 200 形状 {task_id, plan:{kind=patrol, materialized=upfront,
//         phases[5]{seq,kind,title,allocator,status,evidence,note,started_at,finished_at}}};
//       grow 未执行 run → 200 plan{materialized=grow, phases:[]};
//       非流水线手动任务 → plan:null;未知 task → 404;无令牌 → 401。
//   - Tsrv-2 approvals 决策弹窗计划展示 = Web 承接(tsc/vite 由 make ui 冒烟覆盖),server 无新端点,
//     语义由 Tsrv-1(读形状)+ T5(先审后干翻转)共同覆盖 → 本文件不重复。

import (
	"context"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/glacierzzz26/one-person-company-os/internal/pipeline"
	"github.com/glacierzzz26/one-person-company-os/internal/project"
	"github.com/glacierzzz26/one-person-company-os/internal/service"
	"github.com/glacierzzz26/one-person-company-os/internal/storage/repository"
	"github.com/google/uuid"
)

// planReadData 契约 §3.4 响应镜像(task_id 平铺 + plan 可 null)。
type planReadData struct {
	TaskID string          `json:"task_id"`
	Plan   *planViewMirror `json:"plan"`
}

type planViewMirror struct {
	Kind         string            `json:"kind"`
	Materialized string            `json:"materialized"`
	CreatedAt    int64             `json:"created_at"`
	UpdatedAt    int64             `json:"updated_at"`
	Phases       []phaseViewMirror `json:"phases"`
}

type phaseViewMirror struct {
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

// getPlanBearer 带 console token GET /api/v1/tasks/{id}/plan,解出 200 读数据。
func getPlanBearer(t *testing.T, srv *Server, token, taskID string) planReadData {
	t.Helper()
	rec, env := doBearer(t, srv.Handler(), http.MethodGet, "/api/v1/tasks/"+taskID+"/plan", token, "")
	if rec.Code != http.StatusOK || !env.OK {
		t.Fatalf("GET plan task=%s: code=%d ok=%v err=%+v body=%s", taskID, rec.Code, env.OK, env.Error, rec.Body.String())
	}
	var d planReadData
	decodeData(t, env, &d)
	return d
}

// flipPhaseOK 把某条预铺阶段直铺为 ok(写 evidence/note + 起止)。server 层只验读端点形状,
// 状态翻转语义由 service T2/T3 覆盖;此处仅为让 status/evidence/时间戳真实流过 HTTP。
func flipPhaseOK(t *testing.T, srv *Server, st *repository.Store, planID string, seq int64, evidence string) {
	t.Helper()
	phases, err := st.ListPlanPhases(context.Background(), planID)
	if err != nil {
		t.Fatalf("list phases: %v", err)
	}
	for _, ph := range phases {
		if ph.Seq != seq {
			continue
		}
		now := nowUnix()
		stt := "ok"
		ev := evidence
		nt := "ok (fixture flip)"
		if _, err := st.UpdatePlanPhase(context.Background(), ph.ID, &stt, &ev, &nt, &now, &now); err != nil {
			t.Fatalf("flip phase seq%d: %v", seq, err)
		}
		return
	}
	t.Fatalf("phase seq%d not found in plan %s", seq, planID)
}

func TestTsrv1GetTaskPlanReadOnly(t *testing.T) {
	srv, st := newSetupServer(t)
	h := srv.Handler()
	if rec, env := doAPI(t, h, http.MethodPost, "/api/v1/setup", `{"console_token":"sekret-token"}`); rec.Code != http.StatusOK || !env.OK {
		t.Fatalf("setup: code=%d env=%+v", rec.Code, env)
	}
	token := "sekret-token"
	comp := seedCompany(t, st, "ACME", "")
	seedEngineEndpoints(t, st, comp.ID) // live 建单 8.4 默认落槽(live 模式 env seam 关,不触网)

	// ---- 无令牌 → 401(PlanBlock/DecideModal 拉计划即 401 弹 AuthModal)----
	if rec, _ := doAPI(t, h, http.MethodGet, "/api/v1/tasks/"+uuid.NewString()+"/plan", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("no-token plan read: code=%d, want 401", rec.Code)
	}

	// ---- 1) patrol 预铺 run → 200 全形状(phase status/evidence 流过)----
	prjP, plP := seedTsrvProject(t, srv, comp.ID, "acme-patrol", "patrol-daily",
		pipeline.KindOpsPatrol, "check: git hygiene + build + lockfile")
	patrolRun, err := srv.svc.RunPipeline(context.Background(), plP.ID, "")
	if err != nil {
		t.Fatalf("run patrol: %v", err)
	}
	rp, _, ok, err := srv.svc.GetTaskPlan(context.Background(), patrolRun.ID)
	if err != nil || !ok {
		t.Fatalf("patrol plan must pre-materialize on run create (ok=%v err=%v)", ok, err)
	}
	// 模拟完成态 seq1..4(置 ok+evidence);seq5 保持 pending(未决行 keep 空态)。
	for seq := int64(1); seq <= 4; seq++ {
		flipPhaseOK(t, srv, st, rp.ID, seq, "evidence-for-seq")
	}
	d := getPlanBearer(t, srv, token, patrolRun.ID)
	if d.Plan == nil || d.Plan.Kind != "patrol" || d.Plan.Materialized != "upfront" {
		t.Fatalf("patrol plan = %+v, want kind=patrol materialized=upfront", d.Plan)
	}
	if len(d.Plan.Phases) != 5 {
		t.Fatalf("patrol phases = %d, want 5", len(d.Plan.Phases))
	}
	for i, ph := range d.Plan.Phases {
		if ph.Seq != int64(i+1) || ph.Kind == "" || ph.Title == "" || ph.Allocator == "" || ph.Status == "" {
			t.Fatalf("phase %d shape = %+v(缺 seq/kind/title/allocator/status)", i, ph)
		}
		if i < 4 {
			if ph.Status != "ok" || ph.Evidence != "evidence-for-seq" || ph.StartedAt == nil || ph.FinishedAt == nil {
				t.Fatalf("phase seq%d = %+v, want ok + evidence + 起止时间戳", ph.Seq, ph)
			}
		} else if ph.Status != "pending" {
			t.Fatalf("phase seq5 = %s, want pending(未决行 keep 空态)", ph.Status)
		}
	}

	// ---- 2) grow 未执行 bugfix run → 200 phases 空 + materialized=grow ----
	_, plG := seedTsrvProject(t, srv, comp.ID, "acme-grow", "fix-misc",
		pipeline.KindBugfix, "fix a bug")
	growRun, err := srv.svc.RunPipeline(context.Background(), plG.ID, "")
	if err != nil {
		t.Fatalf("run bugfix: %v", err)
	}
	dg := getPlanBearer(t, srv, token, growRun.ID)
	if dg.Plan == nil || dg.Plan.Kind != "engineering" || dg.Plan.Materialized != "grow" {
		t.Fatalf("grow plan = %+v, want kind=engineering materialized=grow", dg.Plan)
	}
	if len(dg.Plan.Phases) != 0 {
		t.Fatalf("grow phases = %d, want 0(执行中 append)", len(dg.Plan.Phases))
	}

	// ---- 3) 非流水线手动任务 → plan:null ----
	manual, err := srv.svc.CreateTaskAs(context.Background(), service.TaskParams{
		CompanyID: comp.ID, Title: "manual request", Description: "not a pipeline run",
		ToolName: "engineering", Risk: "medium", Workspace: prjP.RootPath,
	}, "test")
	if err != nil {
		t.Fatalf("create manual task: %v", err)
	}
	dm := getPlanBearer(t, srv, token, manual.ID)
	if dm.Plan != nil {
		t.Fatalf("non-pipeline task plan = %+v, want null", dm.Plan)
	}

	// ---- 4) 未知 task → 404(GetTask miss)----
	rec, env := doBearer(t, h, http.MethodGet, "/api/v1/tasks/"+uuid.NewString()+"/plan", token, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown task plan: code=%d env=%+v, want 404", rec.Code, env)
	}
}

// seedTsrvProject 建一个 git 就绪项目 + 指定形态流水线(直接走 service 写;读端点鉴权仍经 HTTP)。
func seedTsrvProject(t *testing.T, srv *Server, compID, prjName, plName, kind, desc string) (project.Project, pipeline.Pipeline) {
	t.Helper()
	prj, err := srv.svc.CreateProject(context.Background(), compID, prjName, filepath.Join(t.TempDir(), prjName), "Tsrv project")
	if err != nil {
		t.Fatalf("create project %s: %v", prjName, err)
	}
	pl, err := srv.svc.CreatePipeline(context.Background(), prj.ID, plName, kind, desc, "medium", "")
	if err != nil {
		t.Fatalf("create pipeline %s: %v", plName, err)
	}
	return prj, pl
}
