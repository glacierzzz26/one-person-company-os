package service

// Phase 10.1 契约用例(project-pipeline-foundation.md §五,P*):
//   - P1 project create 成功(git 现成目录)
//   - P2 空目录 → OS git init 后成功
//   - P3 不存在路径 → mkdir + git init 成功
//   - P4 非空非 git → ErrInvalid(400)且零写入
//   - P5 重名 → ErrConflict(409)
//   - P6 pipeline create(三种 kind + 缺省 risk medium + 非法 kind/risk 400)
//   - P7 流水线列表按 project
//   - P8 RunPipeline 建 engineering task(挂 project_id / workspace=root / description=request 覆盖)
//   - P9 同 project 已有活跃 task → ErrPipelineBusy(409)
//   - P10 完成态历史 task 不阻塞新 run
//   - P11 project delete:活跃 run → 409;仅历史任务 → 断引用 + 级联删 pipeline + project 删 + 磁盘目录仍存
//   - P12 pipeline delete(其历史 run 任务保留)
//   - P13 CLI run/delete 审计 actor=human:cli 可见(Web console actor 见 server 层用例)
// 状态推演用 scripted 公司(离线红线:建单不解析端点槽);project root 就绪用真 git(binary 需在环境)。

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/glacierzzz26/one-person-company-os/internal/pipeline"
	"github.com/glacierzzz26/one-person-company-os/internal/project"
)

func mustCreateProject(t *testing.T, svc *Service, compID, name, dir string) project.Project {
	t.Helper()
	p, err := svc.CreateProject(context.Background(), compID, name, dir, "desc")
	if err != nil {
		t.Fatalf("CreateProject(%s,%s): %v", name, dir, err)
	}
	return p
}

func mustCreatePipeline(t *testing.T, svc *Service, prjID, name, kind, risk string) pipeline.Pipeline {
	t.Helper()
	pl, err := svc.CreatePipeline(context.Background(), prjID, name, kind, "intent "+name, risk, "")
	if err != nil {
		t.Fatalf("CreatePipeline(%s): %v", name, err)
	}
	return pl
}

// ---- P1+P2+P3:root 就绪三成功分支 ----

func TestP1P2P3CreateProjectRootReady(t *testing.T) {
	svc, st := newSvc(t)
	compID := seedCompanyID(t, st)
	ctx := context.Background()

	// P1:已存在 git 仓库 → 直接用(不做 git init;seedGitWorkspace 已含基线提交)。
	gitDir := seedGitWorkspace(t)
	p1 := mustCreateProject(t, svc, compID, "already-git", gitDir)
	if p1.RootPath != gitDir {
		t.Fatalf("P1 root_path = %q, want %q", p1.RootPath, gitDir)
	}
	if !wsIsGit(ctx, p1.RootPath) {
		t.Fatalf("P1 root should be a git repo")
	}

	// P2:空目录(非 git)→ OS git init。
	emptyDir := filepath.Join(t.TempDir(), "empty")
	if err := os.MkdirAll(emptyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	p2 := mustCreateProject(t, svc, compID, "empty-gitinit", emptyDir)
	if !wsIsGit(ctx, p2.RootPath) {
		t.Fatalf("P2 root should have been git-inited: %s", p2.RootPath)
	}

	// P3:不存在路径 → mkdir + git init。
	nonexist := filepath.Join(t.TempDir(), "brand-new")
	p3 := mustCreateProject(t, svc, compID, "brand-new", nonexist)
	if _, err := os.Stat(p3.RootPath); err != nil {
		t.Fatalf("P3 root not created: %v", err)
	}
	if !wsIsGit(ctx, p3.RootPath) {
		t.Fatalf("P3 root should have been mkdir + git-inited")
	}
}

// ---- P4:非空非 git → ErrInvalid 且零写入 ----

func TestP4NonEmptyNonGitRejected(t *testing.T) {
	svc, st := newSvc(t)
	compID := seedCompanyID(t, st)
	ctx := context.Background()

	dir := filepath.Join(t.TempDir(), "content-but-not-git")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(dir, "keep.txt")
	if err := os.WriteFile(marker, []byte("user content"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := svc.CreateProject(ctx, compID, "bad-root", dir, "")
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("P4 want ErrInvalid, got %v", err)
	}
	// 零写入:用户文件仍在、无 .git 被创建。
	if _, serr := os.Stat(marker); serr != nil {
		t.Fatalf("P4 must not touch existing content: %v", serr)
	}
	if wsIsGit(ctx, dir) {
		t.Fatalf("P4 must not git init a non-empty non-git dir")
	}
	// 未落库。
	if list, lerr := svc.ListProjects(ctx, compID); lerr != nil || len(list) != 0 {
		t.Fatalf("P4 no project row expected, got %v err=%v", list, lerr)
	}
}

// ---- P5:重名 → ErrConflict(同公司;异公司同名合法)----

func TestP5DuplicateProjectNameConflict(t *testing.T) {
	svc, st := newSvc(t)
	compID := seedCompanyID(t, st)
	ctx := context.Background()

	mustCreateProject(t, svc, compID, "same-name", filepath.Join(t.TempDir(), "dup-a"))
	_, err := svc.CreateProject(ctx, compID, "same-name", filepath.Join(t.TempDir(), "dup-b"), "")
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("P5 want ErrConflict, got %v", err)
	}
	// 异公司同名合法。
	compB := seedCompanyID(t, st)
	if _, err := svc.CreateProject(ctx, compB, "same-name", filepath.Join(t.TempDir(), "dup-c"), ""); err != nil {
		t.Fatalf("P5 same name across companies must be ok: %v", err)
	}
}

// ---- P6+P7:pipeline create 校验 + 按 project 列表 ----

func TestP6P7PipelineCreateAndList(t *testing.T) {
	svc, st := newSvc(t)
	compID := seedCompanyID(t, st)
	ctx := context.Background()

	prjA := mustCreateProject(t, svc, compID, "A", filepath.Join(t.TempDir(), "A"))
	prjB := mustCreateProject(t, svc, compID, "B", filepath.Join(t.TempDir(), "B"))

	// P6:三种 kind + risk 缺省 medium + 显式 risk;默认 active。
	bug := mustCreatePipeline(t, svc, prjA.ID, "fix-login", pipeline.KindBugfix, "")
	if bug.Risk != pipeline.RiskMedium {
		t.Fatalf("P6 default risk = %q, want medium", bug.Risk)
	}
	if bug.Status != pipeline.StatusActive {
		t.Fatalf("P6 create default status = %q, want active", bug.Status)
	}
	mustCreatePipeline(t, svc, prjA.ID, "ship-v2", pipeline.KindDevelop, pipeline.RiskHigh)
	mustCreatePipeline(t, svc, prjA.ID, "patrol-daily", pipeline.KindOpsPatrol, pipeline.RiskLow)

	// 非法 kind / 非法 risk → ErrInvalid。
	if _, err := svc.CreatePipeline(ctx, prjA.ID, "bad-kind", "k8s", "", "medium", ""); !errors.Is(err, ErrInvalid) {
		t.Fatalf("P6 invalid kind want ErrInvalid, got %v", err)
	}
	if _, err := svc.CreatePipeline(ctx, prjA.ID, "bad-risk", pipeline.KindBugfix, "", "extreme", ""); !errors.Is(err, ErrInvalid) {
		t.Fatalf("P6 invalid risk want ErrInvalid, got %v", err)
	}
	// 重名流水线(同 project)→ ErrConflict。
	if _, err := svc.CreatePipeline(ctx, prjA.ID, "fix-login", pipeline.KindBugfix, "", "medium", ""); !errors.Is(err, ErrConflict) {
		t.Fatalf("P6 duplicate pipeline name want ErrConflict, got %v", err)
	}
	// 不存在 project → 404(sql.ErrNoRows 冒上)。
	if _, err := svc.CreatePipeline(ctx, "no-such-project", "x", pipeline.KindBugfix, "", "", ""); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("P6 create pipeline in missing project want sql.ErrNoRows, got %v", err)
	}

	// P7:列表按 project。
	listA, err := svc.ListPipelines(ctx, prjA.ID)
	if err != nil || len(listA) != 3 {
		t.Fatalf("P7 ListPipelines(A) len=3 want 3, got %d err=%v", len(listA), err)
	}
	listB, err := svc.ListPipelines(ctx, prjB.ID)
	if err != nil || len(listB) != 0 {
		t.Fatalf("P7 ListPipelines(B) len=0 want 0, got %d err=%v", len(listB), err)
	}
}

// ---- P8+P9+P10:RunPipeline 建单 + 串行守卫 + 完成态不阻塞 ----

func TestP8P9P10RunPipeline(t *testing.T) {
	t.Setenv("OS_ENGINE_MODE", "scripted") // 离线红线:建单不解析端点槽
	svc, st := newSvc(t)
	compID := seedCompanyID(t, st)
	ctx := context.Background()

	prj := mustCreateProject(t, svc, compID, "web", filepath.Join(t.TempDir(), "web"))
	pl := mustCreatePipeline(t, svc, prj.ID, "fix-login", pipeline.KindBugfix, pipeline.RiskMedium)

	// P8:run(无 request)→ 意图 = pipeline.description;task 挂 project_id、workspace=root、tool=engineering。
	run1, err := svc.RunPipeline(ctx, pl.ID, "")
	if err != nil {
		t.Fatalf("P8 RunPipeline: %v", err)
	}
	if run1.ProjectID == nil || *run1.ProjectID != prj.ID {
		t.Fatalf("P8 task.ProjectID = %v, want project %s", run1.ProjectID, prj.ID)
	}
	if run1.WorkspacePath != prj.RootPath {
		t.Fatalf("P8 task.WorkspacePath = %q, want root %q", run1.WorkspacePath, prj.RootPath)
	}
	if run1.ToolName != "engineering" {
		t.Fatalf("P8 task.ToolName = %q, want engineering", run1.ToolName)
	}
	if run1.Title != pl.Name {
		t.Fatalf("P8 task.Title = %q, want pipeline name", run1.Title)
	}
	if run1.Description != "intent fix-login" {
		t.Fatalf("P8 default intent = %q, want pipeline description", run1.Description)
	}
	if run1.Risk != pipeline.RiskMedium {
		t.Fatalf("P8 task.Risk = %q, want pipeline risk", run1.Risk)
	}
	if run1.Status != "pending" || run1.QStatus != "ready" {
		t.Fatalf("P8 run should be async queued pending/ready, got %s/%s", run1.Status, run1.QStatus)
	}

	// P9:同 project 已有活跃 run → ErrPipelineBusy。
	if _, err := svc.RunPipeline(ctx, pl.ID, "second while first active"); !errors.Is(err, ErrPipelineBusy) {
		t.Fatalf("P9 want ErrPipelineBusy, got %v", err)
	}

	// P10:完成态历史 task 不阻塞;request 覆盖 description。
	if _, err := st.CompleteTask(ctx, run1.ID, "ok"); err != nil {
		t.Fatalf("P10 complete first run: %v", err)
	}
	run2, err := svc.RunPipeline(ctx, pl.ID, "override intent now")
	if err != nil {
		t.Fatalf("P10 RunPipeline after completed: %v", err)
	}
	if run2.Description != "override intent now" {
		t.Fatalf("P10 request override: description = %q, want request text", run2.Description)
	}
}

// ---- P11:project delete 受控 ----

func TestP11DeleteProject(t *testing.T) {
	t.Setenv("OS_ENGINE_MODE", "scripted")
	svc, st := newSvc(t)
	compID := seedCompanyID(t, st)
	ctx := context.Background()

	prj := mustCreateProject(t, svc, compID, "web", filepath.Join(t.TempDir(), "web"))
	pl := mustCreatePipeline(t, svc, prj.ID, "fix", pipeline.KindBugfix, "")

	// 有活跃 run → ErrProjectBusy(409)。
	run, err := svc.RunPipeline(ctx, pl.ID, "")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if err := svc.DeleteProject(ctx, prj.ID); !errors.Is(err, ErrProjectBusy) {
		t.Fatalf("P11 active run delete want ErrProjectBusy, got %v", err)
	}

	// 完成 run → delete 成功:任务 project_id 断引用(任务保留)、流水线级联删、项目删、磁盘仍在。
	if _, err := st.CompleteTask(ctx, run.ID, "done"); err != nil {
		t.Fatalf("complete: %v", err)
	}
	if err := svc.DeleteProject(ctx, prj.ID); err != nil {
		t.Fatalf("P11 delete after completion: %v", err)
	}
	if _, err := svc.GetProject(ctx, prj.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("P11 project row should be gone, got %v", err)
	}
	if list, err := svc.ListPipelines(ctx, prj.ID); err != nil || len(list) != 0 {
		t.Fatalf("P11 pipelines should cascade-delete, got %v err=%v", list, err)
	}
	kept, err := svc.GetTask(ctx, run.ID)
	if err != nil {
		t.Fatalf("P11 historical task must remain: %v", err)
	}
	if kept.ProjectID != nil {
		t.Fatalf("P11 historical task project_id must be nulled, got %v", *kept.ProjectID)
	}
	if _, serr := os.Stat(prj.RootPath); serr != nil {
		t.Fatalf("P11 disk directory must remain untouched: %v", serr)
	}
}

// ---- P12:pipeline delete(其历史 run 任务保留,project 不删)----

func TestP12DeletePipelineKeepsTask(t *testing.T) {
	t.Setenv("OS_ENGINE_MODE", "scripted")
	svc, st := newSvc(t)
	compID := seedCompanyID(t, st)
	ctx := context.Background()

	prj := mustCreateProject(t, svc, compID, "web", filepath.Join(t.TempDir(), "web"))
	pl := mustCreatePipeline(t, svc, prj.ID, "fix", pipeline.KindBugfix, "")
	run, err := svc.RunPipeline(ctx, pl.ID, "")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if _, err := st.CompleteTask(ctx, run.ID, "done"); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeletePipeline(ctx, pl.ID); err != nil {
		t.Fatalf("P12 delete pipeline: %v", err)
	}
	if _, err := svc.GetPipeline(ctx, pl.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("P12 pipeline row should be gone, got %v", err)
	}
	kept, err := svc.GetTask(ctx, run.ID)
	if err != nil {
		t.Fatalf("P12 run task must remain after pipeline delete: %v", err)
	}
	if kept.ProjectID == nil || *kept.ProjectID != prj.ID {
		t.Fatalf("P12 task project_id should still point to project, got %v", kept.ProjectID)
	}
	if _, err := svc.GetProject(ctx, prj.ID); err != nil {
		t.Fatalf("P12 project must not be deleted: %v", err)
	}
}

// ---- P13:CLI actor 审计(Web console actor 在 server 层用例)----

func TestP13CLIActorAudit(t *testing.T) {
	t.Setenv("OS_ENGINE_MODE", "scripted")
	svc, st := newSvc(t)
	compID := seedCompanyID(t, st)
	ctx := context.Background()

	prj := mustCreateProject(t, svc, compID, "web", filepath.Join(t.TempDir(), "web"))
	pl := mustCreatePipeline(t, svc, prj.ID, "fix", pipeline.KindBugfix, "")
	if _, err := svc.RunPipeline(ctx, pl.ID, ""); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeletePipeline(ctx, pl.ID); err != nil {
		t.Fatal(err)
	}

	audits, err := st.ListAudits(ctx, "pipeline")
	if err != nil {
		t.Fatalf("ListAudits: %v", err)
	}
	want := map[string]bool{"create": false, "run": false, "delete": false}
	for _, a := range audits {
		if a.Actor != "human:cli" {
			t.Fatalf("P13 actor = %q, want human:cli (action %s)", a.Actor, a.Action)
		}
		if a.Action == "create" || a.Action == "run" || a.Action == "delete" {
			want[a.Action] = true
		}
	}
	for act, seen := range want {
		if !seen {
			t.Fatalf("P13 pipeline audit missing action %q (actor human:cli)", act)
		}
	}
	// run 建出的 task 也带 human:cli create 审计(scripted 下 detail 无端点落档)。
	taskAudits, err := st.ListAudits(ctx, "task")
	if err != nil {
		t.Fatal(err)
	}
	foundCreate := false
	for _, a := range taskAudits {
		if a.Action == "create" && a.Actor == "human:cli" {
			foundCreate = true
		}
	}
	if !foundCreate {
		t.Fatalf("P13 task create audit (human:cli) not found")
	}
}
