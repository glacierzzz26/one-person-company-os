package service

// Phase 10.2 契约用例(ops-patrol-schedule.md §五,S*;scripted 公司 = env seam 打开 + OS_ENGINE_MODE=scripted,
// 产品零 env;DB 默认 live 的行为在 server 层 Srv-* 覆盖):
//   - S1 schedule cron 校验(service 建口):合法 5 段存原文;空/off = 不调度;非法 → ErrInvalid(带指引)
//   - S2 Next 纯函数已由 internal/schedule/cron_test.go 全量覆盖(本文件不重复)
//   - S3 UpdatePipelineScheduleAs:改调度 + 审计(human:cli 与 human:console 各一,detail 含 old→new);非法 → ErrInvalid
//   - S4 RunPipelineAs(actor=system:schedule)→ pipeline run 审计 actor=system:schedule;task 反链 pipeline/project
//   - S5 scripted 巡检 run(runPatrol):patrol/<taskID>.md 真落盘 + OS git commit(工作树归 clean、HEAD=os-patrol:
//     提交)→ 完成;result 以 patrol: 开头含路径+ok;审计 patrol_start/patrol_delegate/patrol_complete 齐
//   - S6 verdict ok → 绿、无处置(无 chain/manual 审计、不建新任务)
//   - S7 finding-high+fix+同项目 active bugfix → 先完成 patrol → 链拉 bugfix(desc=summary、同 project、
//     pipeline_id=bugfix、actor=system:patrol_chain);两条任务齐
//   - S8 finding-high 无 active bugfix / finding-low → 仅 notify(best-effort,无 webhook 静默)+ 审计 patrol_manual,不建任务
//   - S9 verdict malformed(不可解析)/ 报告产出失败 → 任务 failed(engFail),不默认绿
//   - S10 分流兜底:kind≠ops_patrol / 流水线已删(pipeline_id 置空)→ runClaimed 落回 runEngineering(完成、非 patrol 产物)
//   - S11 删除断引用:DeletePipeline → run 历史 task 保留且 pipeline_id 置空(project_id 仍指项目);
//     DeleteProject → 双清 project_id+pipeline_id、任务保留、磁盘仍在
//
// 说明:patrol 提交 = OS 唯一提交者,真实 git commit(gitDirCmd → commitDelegation)。项目 root 由 CreateProject
// git init(无身份)→ 用例先注入本地 git identity,报告提交才可达。

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glacierzzz26/one-person-company-os/internal/audit"
	"github.com/glacierzzz26/one-person-company-os/internal/pipeline"
	"github.com/glacierzzz26/one-person-company-os/internal/project"
	"github.com/glacierzzz26/one-person-company-os/internal/storage/repository"
	"github.com/glacierzzz26/one-person-company-os/internal/task"
)

// gitIdentity 给项目 root 注入提交身份(patrol 报告提交需要;仅测试仓库,不触碰用户配置)。
func gitIdentity(t *testing.T, ws string) {
	t.Helper()
	runGit(t, ws, "config", "user.email", "test@opos.local")
	runGit(t, ws, "config", "user.name", "opos-test")
}

// scriptedHarness 起 scripted 模式 svc + 一个 git 就绪项目(identity 已注入,patrol 提交可达)。
func scriptedHarness(t *testing.T, name string) (*Service, *repository.Store, project.Project) {
	t.Helper()
	svc, st := newSvc(t)
	compID := seedCompanyID(t, st)
	prj := mustCreateProject(t, svc, compID, name, filepath.Join(t.TempDir(), name))
	gitIdentity(t, prj.RootPath)
	return svc, st, prj
}

// mustCreateOpsPatrol 建 ops_patrol 流水线(description = 巡检检查项意图;risk medium 免审批门)。
func mustCreateOpsPatrol(t *testing.T, svc *Service, prjID, name, schedule string) pipeline.Pipeline {
	t.Helper()
	pl, err := svc.CreatePipeline(context.Background(), prjID, name, pipeline.KindOpsPatrol,
		"check: git hygiene + build + lockfile drift", pipeline.RiskMedium, schedule)
	if err != nil {
		t.Fatalf("CreatePipeline ops_patrol %s: %v", name, err)
	}
	return pl
}

// auditsOf 汇总某实体/动作的 audit 明细(action → detail;重复动作取最后一条)。
func auditsOf(t *testing.T, st *repository.Store, entity string) map[string]string {
	t.Helper()
	all, err := st.ListAudits(context.Background(), entity)
	if err != nil {
		t.Fatalf("ListAudits(%s): %v", entity, err)
	}
	out := map[string]string{}
	for _, a := range all {
		out[a.Action] = a.Detail
	}
	return out
}

// ---- S1:schedule cron 校验(建口;off/空合法,非法 → ErrInvalid 不落库)----

func TestS1ScheduleValidationOnCreate(t *testing.T) {
	svc, st := newSvc(t)
	compID := seedCompanyID(t, st)
	ctx := context.Background()
	prj := mustCreateProject(t, svc, compID, "web", filepath.Join(t.TempDir(), "web"))

	valid := []struct{ sched, name string }{
		{"0 9 * * *", "s-daily"},
		{"*/30 * * * *", "s-every30"},
		{"0 9 * * 1-5", "s-weekdays"},
		{"0 9 1 * *", "s-monthly"},
		{"0 0 * * 7", "s-sunday7"}, // 7 = 周日别名
	}
	for _, c := range valid {
		pl, err := svc.CreatePipeline(ctx, prj.ID, c.name, pipeline.KindBugfix, "", "", c.sched)
		if err != nil {
			t.Fatalf("S1 create with %q: %v", c.sched, err)
		}
		if pl.Schedule != c.sched {
			t.Fatalf("S1 schedule stored = %q, want original %q", pl.Schedule, c.sched)
		}
	}
	// 空 / off = 不调度(合法,存原文)。
	for i, off := range []string{"", "off"} {
		name := "s-empty"
		if i == 1 {
			name = "s-off"
		}
		pl, err := svc.CreatePipeline(ctx, prj.ID, name, pipeline.KindBugfix, "", "", off)
		if err != nil {
			t.Fatalf("S1 create with %q should be valid (not scheduled): %v", off, err)
		}
		if pl.Schedule != off {
			t.Fatalf("S1 schedule stored = %q, want original %q", pl.Schedule, off)
		}
	}

	// 非法 → ErrInvalid 带指引(不落库)。
	invalid := []string{"0 9 * *", "60 * * * *", "* 24 * * *", "0 9 * * MON", "@daily", "abc"}
	for _, bad := range invalid {
		if _, err := svc.CreatePipeline(ctx, prj.ID, "s-bad", pipeline.KindBugfix, "", "", bad); !errors.Is(err, ErrInvalid) {
			t.Fatalf("S1 invalid schedule %q want ErrInvalid, got %v", bad, err)
		}
	}
	list, err := svc.ListPipelines(ctx, prj.ID)
	if err != nil {
		t.Fatalf("S1 list: %v", err)
	}
	if len(list) != len(valid)+2 {
		t.Fatalf("S1 pipelines = %d, want %d (invalid schedules must not persist)", len(list), len(valid)+2)
	}
}

// ---- S3:UpdatePipelineScheduleAs(改调度 + 审计 actor/detail)----

func TestS3UpdatePipelineSchedule(t *testing.T) {
	svc, st := newSvc(t)
	compID := seedCompanyID(t, st)
	ctx := context.Background()
	prj := mustCreateProject(t, svc, compID, "web", filepath.Join(t.TempDir(), "web"))
	pl := mustCreatePipeline(t, svc, prj.ID, "fix", pipeline.KindBugfix, "")

	// human:cli 改 '' → daily;human:console 改 daily → weekdays。
	upd, err := svc.UpdatePipelineScheduleAs(ctx, pl.ID, "0 9 * * *", "human:cli")
	if err != nil {
		t.Fatalf("S3 update cli: %v", err)
	}
	if upd.Schedule != "0 9 * * *" {
		t.Fatalf("S3 schedule = %q, want 0 9 * * *", upd.Schedule)
	}
	if _, err := svc.UpdatePipelineScheduleAs(ctx, pl.ID, "0 9 * * 1-5", "human:console"); err != nil {
		t.Fatalf("S3 update console: %v", err)
	}
	// 非法 → ErrInvalid;不存在 → sql.ErrNoRows。
	if _, err := svc.UpdatePipelineScheduleAs(ctx, pl.ID, "60 * * * *", "human:console"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("S3 invalid schedule want ErrInvalid, got %v", err)
	}
	if _, err := svc.UpdatePipelineScheduleAs(ctx, "no-such-pipeline", "0 9 * * *", "human:console"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("S3 missing pipeline want sql.ErrNoRows, got %v", err)
	}

	audits, err := st.ListAudits(ctx, "pipeline")
	if err != nil {
		t.Fatalf("S3 audits: %v", err)
	}
	var cliEdit, consoleEdit int
	for _, a := range audits {
		if a.EntityID != pl.ID || a.Action != "edit" {
			continue
		}
		switch {
		case a.Actor == "human:cli" && strings.Contains(a.Detail, " → 0 9 * * *"):
			cliEdit++
		case a.Actor == "human:console" && strings.Contains(a.Detail, "0 9 * * * → 0 9 * * 1-5"):
			consoleEdit++
		}
	}
	if cliEdit != 1 || consoleEdit != 1 {
		t.Fatalf("S3 edit audits cli=%d console=%d, want 1/1 (detail must record old → new)", cliEdit, consoleEdit)
	}
}

// ---- S4:RunPipelineAs 以 system:schedule 触发 → 审计 actor + task 反链 ----

func TestS4ScheduleActorRun(t *testing.T) {
	t.Setenv("OS_ENGINE_MODE", "scripted")
	svc, st := newSvc(t)
	compID := seedCompanyID(t, st)
	ctx := context.Background()
	prj := mustCreateProject(t, svc, compID, "web", filepath.Join(t.TempDir(), "web"))
	pl := mustCreatePipeline(t, svc, prj.ID, "fix", pipeline.KindBugfix, "")

	tk, err := svc.RunPipelineAs(ctx, pl.ID, "", ActorSchedule)
	if err != nil {
		t.Fatalf("S4 RunPipelineAs(system:schedule): %v", err)
	}
	if tk.PipelineID == nil || *tk.PipelineID != pl.ID {
		t.Fatalf("S4 task.pipeline_id = %v, want %s", tk.PipelineID, pl.ID)
	}
	if tk.ProjectID == nil || *tk.ProjectID != prj.ID {
		t.Fatalf("S4 task.project_id = %v, want %s", tk.ProjectID, prj.ID)
	}
	// pipeline run 审计 actor=system:schedule;task create 审计 actor=system:schedule。
	pFound := false
	for _, a := range listAudits(t, st, "pipeline") {
		if a.EntityID == pl.ID && a.Action == "run" && a.Actor == ActorSchedule {
			pFound = true
		}
	}
	if !pFound {
		t.Fatalf("S4 pipeline run audit (actor %s) not found", ActorSchedule)
	}
	tFound := false
	for _, a := range listAudits(t, st, "task") {
		if a.EntityID == tk.ID && a.Action == "create" && a.Actor == ActorSchedule {
			tFound = true
		}
	}
	if !tFound {
		t.Fatalf("S4 task create audit (actor %s) not found", ActorSchedule)
	}
}

func listAudits(t *testing.T, st *repository.Store, entity string) []audit.Audit {
	t.Helper()
	all, err := st.ListAudits(context.Background(), entity)
	if err != nil {
		t.Fatalf("ListAudits(%s): %v", entity, err)
	}
	return all
}

// ---- S5:scripted 巡检 run 端到端(报告真落盘 + OS git 提交 + 完成 + 审计)----

func TestS5ScriptedPatrolRunCommitsReport(t *testing.T) {
	t.Setenv("OS_ENGINE_MODE", "scripted")
	svc, st, prj := scriptedHarness(t, "web")
	ctx := context.Background()
	pl := mustCreateOpsPatrol(t, svc, prj.ID, "patrol-daily", "0 9 * * *")

	tk, err := svc.RunPipelineAs(ctx, pl.ID, "", ActorSchedule)
	if err != nil {
		t.Fatalf("S5 run patrol: %v", err)
	}
	if err := svc.ExecuteTask(ctx, "w1", tk.ID); err != nil {
		t.Fatalf("S5 ExecuteTask: %v", err)
	}

	got, err := svc.GetTask(ctx, tk.ID)
	if err != nil {
		t.Fatalf("S5 GetTask: %v", err)
	}
	if got.Status != "completed" {
		t.Fatalf("S5 status = %s, want completed (last_error=%s)", got.Status, got.LastError)
	}
	// result 以 patrol: 开头,含报告路径 + ok。
	wantPrefix := PatrolResultTag + filepath.ToSlash(filepath.Join(PatrolDirName, tk.ID+".md")) + "|ok=true"
	if !strings.HasPrefix(got.Result, wantPrefix) {
		t.Fatalf("S5 result = %q, want prefix %q", got.Result, wantPrefix)
	}

	// 报告文件真落盘(项目 root 下 patrol/<taskID>.md)。
	reportAbs := filepath.Join(prj.RootPath, PatrolDirName, tk.ID+".md")
	data, err := os.ReadFile(reportAbs)
	if err != nil {
		t.Fatalf("S5 report file missing on disk: %v", err)
	}
	if !strings.Contains(string(data), "# Patrol Report") || !strings.Contains(string(data), tk.ID) {
		t.Fatalf("S5 report content unexpected:\n%s", data)
	}

	// OS git 提交:工作树归 clean;HEAD 最近提交 = os-patrol。
	if !wsPorcelainClean(ctx, prj.RootPath) {
		t.Fatalf("S5 workspace must be clean after OS patrol commit:\n%s", mustPorcelain(ctx, prj.RootPath))
	}
	headMsg, err := gitDirCmd(ctx, prj.RootPath, "log", "-1", "--pretty=%s")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(headMsg, "os-patrol: ") {
		t.Fatalf("S5 HEAD commit = %q, want os-patrol: prefix", headMsg)
	}

	// 审计三连:patrol_start / patrol_delegate / patrol_complete(齐)。
	all := listAudits(t, st, "task")
	aud := map[string]string{}
	for _, a := range all {
		if a.EntityID == tk.ID {
			aud[a.Action] = a.Detail
		}
	}
	for _, act := range []string{"patrol_start", "patrol_delegate", "patrol_complete"} {
		if _, ok := aud[act]; !ok {
			t.Fatalf("S5 audit %q missing for task %s (have %v)", act, tk.ID, aud)
		}
	}
	reportRelSlash := filepath.ToSlash(filepath.Join(PatrolDirName, tk.ID+".md"))
	if !strings.Contains(aud["patrol_delegate"], "report="+reportRelSlash) {
		t.Fatalf("S5 patrol_delegate detail = %q", aud["patrol_delegate"])
	}
	if !strings.Contains(aud["patrol_complete"], "ok=true") {
		t.Fatalf("S5 patrol_complete detail = %q", aud["patrol_complete"])
	}
}

// ---- S6:verdict ok → 绿、无处置(无 chain/manual 审计、不建新任务)----

func TestS6PatrolOKNoDisposition(t *testing.T) {
	t.Setenv("OS_ENGINE_MODE", "scripted")
	svc, st, prj := scriptedHarness(t, "web")
	ctx := context.Background()
	pl := mustCreateOpsPatrol(t, svc, prj.ID, "patrol-daily", "")
	tk, err := svc.RunPipeline(ctx, pl.ID, "")
	if err != nil {
		t.Fatalf("S6 run: %v", err)
	}
	if err := svc.ExecuteTask(ctx, "w1", tk.ID); err != nil {
		t.Fatalf("S6 ExecuteTask: %v", err)
	}
	if got, _ := svc.GetTask(ctx, tk.ID); got.Status != "completed" {
		t.Fatalf("S6 status = %s, want completed", got.Status)
	}
	all := listAudits(t, st, "task")
	aud := map[string]string{}
	for _, a := range all {
		if a.EntityID == tk.ID {
			aud[a.Action] = a.Detail
		}
	}
	if _, ok := aud["patrol_manual"]; ok {
		t.Fatalf("S6 ok verdict must not produce patrol_manual audit: %+v", aud)
	}
	if _, ok := aud["patrol_chain"]; ok {
		t.Fatalf("S6 ok verdict must not chain: %+v", aud)
	}
	tasks, err := svc.ListTasksByProject(ctx, prj.ID, 20)
	if err != nil || len(tasks) != 1 {
		t.Fatalf("S6 project tasks = %d (err=%v), want 1 (no chained run)", len(tasks), err)
	}
}

// ---- S7:finding-high + fix + 同项目 active bugfix → 链拉 bugfix(先完成 patrol)----

func TestS7PatrolFindingHighChainsBugfix(t *testing.T) {
	t.Setenv("OS_ENGINE_MODE", "scripted")
	t.Setenv("OS_SCRIPT_PATROL", "finding-high")
	svc, st, prj := scriptedHarness(t, "web")
	ctx := context.Background()
	patrolPL := mustCreateOpsPatrol(t, svc, prj.ID, "patrol-daily", "")
	// 同项目 active bugfix 流水线(链拉目标)。
	bugfixPL, err := svc.CreatePipeline(ctx, prj.ID, "fix-any", pipeline.KindBugfix,
		"apply remediation for patrol findings", pipeline.RiskMedium, "")
	if err != nil {
		t.Fatalf("S7 create bugfix: %v", err)
	}

	patrolTk, err := svc.RunPipeline(ctx, patrolPL.ID, "")
	if err != nil {
		t.Fatalf("S7 run patrol: %v", err)
	}
	if err := svc.ExecuteTask(ctx, "w1", patrolTk.ID); err != nil {
		t.Fatalf("S7 ExecuteTask: %v", err)
	}
	patrol, err := svc.GetTask(ctx, patrolTk.ID)
	if err != nil {
		t.Fatal(err)
	}
	if patrol.Status != "completed" {
		t.Fatalf("S7 patrol status = %s (chain requires completion first)", patrol.Status)
	}
	if !strings.Contains(patrol.Result, "ok=false") {
		t.Fatalf("S7 patrol result should be non-ok:\n%s", patrol.Result)
	}

	// 链拉:project 内第二条任务 = bugfix 工程 run(desc=verdict summary、挂 bugfix pipeline/project)。
	tasks, err := svc.ListTasksByProject(ctx, prj.ID, 20)
	if err != nil || len(tasks) != 2 {
		t.Fatalf("S7 project tasks = %d (err=%v), want patrol + chained bugfix = 2", len(tasks), err)
	}
	var chain *task.Task
	for i := range tasks {
		if tasks[i].ID != patrolTk.ID {
			chain = &tasks[i]
		}
	}
	if chain == nil {
		t.Fatalf("S7 chained bugfix task not found: %+v", tasks)
	}
	if chain.PipelineID == nil || *chain.PipelineID != bugfixPL.ID {
		t.Fatalf("S7 chain.pipeline_id = %v, want bugfix %s", chain.PipelineID, bugfixPL.ID)
	}
	if chain.ProjectID == nil || *chain.ProjectID != prj.ID {
		t.Fatalf("S7 chain.project_id = %v, want %s", chain.ProjectID, prj.ID)
	}
	if chain.Status != "pending" {
		t.Fatalf("S7 chain task should be pending (queued, not auto-executed): %s", chain.Status)
	}
	wantDesc := "scripted high finding: untracked build artifact residue in project dir"
	if chain.Description != wantDesc {
		t.Fatalf("S7 chain description = %q, want verdict summary %q", chain.Description, wantDesc)
	}
	// 审计:patrol_chain(actor system:patrol_chain)+ bugfix pipeline run 审计 actor=system:patrol_chain。
	all := listAudits(t, st, "task")
	chainAudit := map[string]string{}
	for _, a := range all {
		if a.EntityID == patrolTk.ID {
			chainAudit[a.Action] = a.Detail
		}
	}
	if d, ok := chainAudit["patrol_chain"]; !ok || !strings.Contains(d, "bugfix ") {
		t.Fatalf("S7 patrol_chain audit missing/wrong: %q ok=%v", d, ok)
	}
	chainRunSeen := false
	for _, a := range listAudits(t, st, "pipeline") {
		if a.EntityID == bugfixPL.ID && a.Action == "run" && a.Actor == actorChain {
			chainRunSeen = true
		}
	}
	if !chainRunSeen {
		t.Fatalf("S7 bugfix pipeline run audit actor=%s not found", actorChain)
	}
}

// ---- S8:非 ok 但不足链拉条件 → 仅通知 + 审计 patrol_manual,不建任务 ----

func TestS8PatrolFindingNoChain(t *testing.T) {
	// 子场景 A:finding-high+fix 但项目无 active bugfix → manual(no active bugfix)。
	t.Setenv("OS_ENGINE_MODE", "scripted")
	t.Setenv("OS_SCRIPT_PATROL", "finding-high")
	svc, st, prj := scriptedHarness(t, "web")
	ctx := context.Background()
	pl := mustCreateOpsPatrol(t, svc, prj.ID, "patrol-only", "")
	tk, err := svc.RunPipeline(ctx, pl.ID, "")
	if err != nil {
		t.Fatalf("S8-A run: %v", err)
	}
	if err := svc.ExecuteTask(ctx, "w1", tk.ID); err != nil {
		t.Fatalf("S8-A ExecuteTask: %v", err)
	}
	if got, _ := svc.GetTask(ctx, tk.ID); got.Status != "completed" {
		t.Fatalf("S8-A patrol should still complete (disposition best-effort): %s", got.Status)
	}
	all := listAudits(t, st, "task")
	aud := map[string]string{}
	for _, a := range all {
		if a.EntityID == tk.ID {
			aud[a.Action] = a.Detail
		}
	}
	if d, ok := aud["patrol_manual"]; !ok || !strings.Contains(d, "no active bugfix") {
		t.Fatalf("S8-A want patrol_manual(no active bugfix), got %q ok=%v", d, ok)
	}
	if tasks, err := svc.ListTasksByProject(ctx, prj.ID, 20); err != nil || len(tasks) != 1 {
		t.Fatalf("S8-A must not create a task: got %d (err=%v)", len(tasks), err)
	}

	// 子场景 B:finding-low(action=none)→ 直接 manual(notify only),不看 bugfix(有 active bugfix 也不链)。
	t.Setenv("OS_SCRIPT_PATROL", "finding-low")
	svcB, stB, prjB := scriptedHarness(t, "webb")
	ctxB := context.Background()
	if _, err := svcB.CreatePipeline(ctxB, prjB.ID, "fix-present", pipeline.KindBugfix, "x", pipeline.RiskMedium, ""); err != nil {
		t.Fatal(err)
	}
	plB := mustCreateOpsPatrol(t, svcB, prjB.ID, "patrol-low", "")
	tkB, err := svcB.RunPipeline(ctxB, plB.ID, "")
	if err != nil {
		t.Fatalf("S8-B run: %v", err)
	}
	if err := svcB.ExecuteTask(ctxB, "w1", tkB.ID); err != nil {
		t.Fatalf("S8-B ExecuteTask: %v", err)
	}
	allB := listAudits(t, stB, "task")
	audB := map[string]string{}
	for _, a := range allB {
		if a.EntityID == tkB.ID {
			audB[a.Action] = a.Detail
		}
	}
	if d, ok := audB["patrol_manual"]; !ok || !strings.Contains(d, "notify only") {
		t.Fatalf("S8-B want patrol_manual(notify only), got %q ok=%v", d, ok)
	}
	if _, ok := audB["patrol_chain"]; ok {
		t.Fatalf("S8-B low must not chain")
	}
	if tasks, err := svcB.ListTasksByProject(ctxB, prjB.ID, 20); err != nil || len(tasks) != 1 {
		t.Fatalf("S8-B must not create a task: got %d (err=%v)", len(tasks), err)
	}
}

// ---- S9:verdict 不可解析 / 报告无法产出 → 任务失败,不默认绿 ----

func TestS9PatrolMalformedVerdictFails(t *testing.T) {
	t.Setenv("OS_ENGINE_MODE", "scripted")
	t.Setenv("OS_SCRIPT_PATROL", "malformed")
	svc, _, prj := scriptedHarness(t, "web")
	ctx := context.Background()
	pl := mustCreateOpsPatrol(t, svc, prj.ID, "patrol", "")
	tk, err := svc.RunPipeline(ctx, pl.ID, "")
	if err != nil {
		t.Fatalf("S9 run: %v", err)
	}
	if err := svc.ExecuteTask(ctx, "w1", tk.ID); err != nil {
		t.Fatalf("S9 ExecuteTask: %v", err)
	}
	got, _ := svc.GetTask(ctx, tk.ID)
	if got.Status != "failed" {
		t.Fatalf("S9 malformed verdict: status = %s, want failed (no auto-green)", got.Status)
	}
	if !strings.Contains(got.LastError, "cannot parse patrol verdict") {
		t.Fatalf("S9 last_error = %q", got.LastError)
	}
}

func TestS9PatrolReportWriteFailureFails(t *testing.T) {
	t.Setenv("OS_ENGINE_MODE", "scripted")
	svc, _, prj := scriptedHarness(t, "web")
	ctx := context.Background()
	pl := mustCreateOpsPatrol(t, svc, prj.ID, "patrol", "")
	tk, err := svc.RunPipeline(ctx, pl.ID, "")
	if err != nil {
		t.Fatalf("S9b run: %v", err)
	}
	// 用同名文件占住 patrol/ 目录位 → MkdirAll 失败 → 报告产出失败 → engFail(不默认绿)。
	if err := os.WriteFile(filepath.Join(tk.WorkspacePath, PatrolDirName), []byte("not a dir"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := svc.ExecuteTask(ctx, "w1", tk.ID); err != nil {
		t.Fatalf("S9b ExecuteTask: %v", err)
	}
	got, _ := svc.GetTask(ctx, tk.ID)
	if got.Status != "failed" {
		t.Fatalf("S9b report write failure: status = %s, want failed (no report → no green)", got.Status)
	}
	if !strings.Contains(got.LastError, "mkdir patrol dir") {
		t.Fatalf("S9b last_error = %q", got.LastError)
	}
}

// ---- S10:分流兜底(kind≠ops_patrol / 流水线已删 → runEngineering)----

func TestS10DispatchFallback(t *testing.T) {
	t.Setenv("OS_ENGINE_MODE", "scripted")
	// 子场景 A:kind=bugfix → runClaimed 走 runEngineering(scripted 确定性完成,非 patrol 产物)。
	svcA, _, prjA := scriptedHarness(t, "weba")
	ctxA := context.Background()
	bugPL, err := svcA.CreatePipeline(ctxA, prjA.ID, "fix", pipeline.KindBugfix, "intent fix", pipeline.RiskMedium, "")
	if err != nil {
		t.Fatal(err)
	}
	runA, err := svcA.RunPipeline(ctxA, bugPL.ID, "")
	if err != nil {
		t.Fatalf("S10-A run: %v", err)
	}
	if err := svcA.ExecuteTask(ctxA, "w1", runA.ID); err != nil {
		t.Fatalf("S10-A ExecuteTask: %v", err)
	}
	gotA, _ := svcA.GetTask(ctxA, runA.ID)
	if gotA.Status != "completed" {
		t.Fatalf("S10-A bugfix run should complete via engineering driver: %s (last=%s)", gotA.Status, gotA.LastError)
	}
	if strings.HasPrefix(gotA.Result, PatrolResultTag) {
		t.Fatalf("S10-A bugfix run result must not be a patrol artifact:\n%s", gotA.Result)
	}

	// 子场景 B:pipeline 已删(断引用置空 pipeline_id)→ 落回 runEngineering。
	svcB, stB, prjB := scriptedHarness(t, "webb")
	ctxB := context.Background()
	patrolPL := mustCreateOpsPatrol(t, svcB, prjB.ID, "patrol", "")
	runB, err := svcB.RunPipeline(ctxB, patrolPL.ID, "")
	if err != nil {
		t.Fatalf("S10-B run: %v", err)
	}
	if err := svcB.DeletePipeline(ctxB, patrolPL.ID); err != nil {
		t.Fatalf("S10-B delete pipeline: %v", err)
	}
	if err := svcB.ExecuteTask(ctxB, "w1", runB.ID); err != nil {
		t.Fatalf("S10-B ExecuteTask: %v", err)
	}
	gotB, _ := svcB.GetTask(ctxB, runB.ID)
	if gotB.Status != "completed" {
		t.Fatalf("S10-B deleted-pipeline run should fall back to engineering driver: %s", gotB.Status)
	}
	if strings.HasPrefix(gotB.Result, PatrolResultTag) {
		t.Fatalf("S10-B fallback result must not be a patrol artifact:\n%s", gotB.Result)
	}
	// 无 patrol_start(走 runEngineering,而非 runPatrol)。
	for _, a := range listAudits(t, stB, "task") {
		if a.EntityID == runB.ID && a.Action == "patrol_start" {
			t.Fatalf("S10-B must not run patrol driver after pipeline deletion")
		}
	}
}

// ---- S11:删除断引用(pipeline/project 双清 + 任务保留 + 磁盘不动)----

func TestS11DeleteClearsRunLinks(t *testing.T) {
	t.Setenv("OS_ENGINE_MODE", "scripted")
	svc, st, prj := scriptedHarness(t, "web")
	ctx := context.Background()
	bugPL := mustCreatePipeline(t, svc, prj.ID, "fix", pipeline.KindBugfix, "")
	patrolPL := mustCreateOpsPatrol(t, svc, prj.ID, "patrol", "")

	// (a) bugfix run → complete → DeletePipeline → pipeline_id 置空、project_id 仍在、任务保留。
	runA, err := svc.RunPipeline(ctx, bugPL.ID, "")
	if err != nil {
		t.Fatalf("S11 run bugfix: %v", err)
	}
	if _, err := st.CompleteTask(ctx, runA.ID, "done"); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeletePipeline(ctx, bugPL.ID); err != nil {
		t.Fatalf("S11 delete bugfix pipeline: %v", err)
	}
	keptA, err := svc.GetTask(ctx, runA.ID)
	if err != nil {
		t.Fatalf("S11 task must be kept after pipeline delete: %v", err)
	}
	if keptA.PipelineID != nil {
		t.Fatalf("S11 task.pipeline_id after pipeline delete = %v, want nil", *keptA.PipelineID)
	}
	if keptA.ProjectID == nil || *keptA.ProjectID != prj.ID {
		t.Fatalf("S11 task.project_id should survive pipeline delete, got %v", keptA.ProjectID)
	}

	// (b) patrol run → complete → DeleteProject → 双清 + 任务保留 + 流水线级联删 + 磁盘仍在。
	runB, err := svc.RunPipeline(ctx, patrolPL.ID, "")
	if err != nil {
		t.Fatalf("S11 run patrol: %v", err)
	}
	if _, err := st.CompleteTask(ctx, runB.ID, "done"); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteProject(ctx, prj.ID); err != nil {
		t.Fatalf("S11 delete project: %v", err)
	}
	keptB, err := svc.GetTask(ctx, runB.ID)
	if err != nil {
		t.Fatalf("S11 task must survive project delete: %v", err)
	}
	if keptB.ProjectID != nil || keptB.PipelineID != nil {
		t.Fatalf("S11 project delete must clear both project_id+pipeline_id, got project=%v pipeline=%v",
			keptB.ProjectID, keptB.PipelineID)
	}
	if _, err := svc.GetProject(ctx, prj.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("S11 project row should be gone, got %v", err)
	}
	if pipes, err := svc.ListPipelines(ctx, prj.ID); err != nil || len(pipes) != 0 {
		t.Fatalf("S11 pipelines should cascade-delete, got %v err=%v", pipes, err)
	}
	if _, serr := os.Stat(prj.RootPath); serr != nil {
		t.Fatalf("S11 disk directory must remain untouched: %v", serr)
	}
}
