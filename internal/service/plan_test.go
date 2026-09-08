package service

// Phase 10.3 契约用例(phase-plan-contract.md §五,T*;service 全在 scripted 公司 = envSeam 门 + OS_ENGINE_MODE
// =scripted;产品零 env,OS_SCRIPT_* 仅 scripted 分支可达):
//   - T1 RunPipelineAs 建单即铺账:ops_patrol → task_plan(kind=patrol,materialized=upfront,5 条 pending
//     seq1..5);bugfix/develop(grow)→ 只建 plan 行(materialized=grow)无 phase;非流水线手动 createTask 无 plan。
//   - T2 patrol 全流程账本镜像(scripted + 真 git 项目):ok → seq1/2/3/4 全 ok + evidence(报告 rel/提交/
//     机械行/verdict 行)、seq5.dispose=skipped;finding-high + 同项目 active bugfix → seq5.dispose ok +
//     note 链拉(既有 S5-S11 语义零 body 改,仅新增翻状态副作用)。
//   - T3 机械层(D4):报告后/pre 后塞残留 → seq3.accept fail → engFail 不判读(seq4 未到、OS_SCRIPT_PATROL
//     judge 未消费);报告缺/空 → fail(mechanicalPatrolCheck 单测);委派前已有用户脏项(pre 快照含)→ 不误伤。
//   - T4 engineering grow 记账:direct 跑通 → do 写 R / accept 测试 R / accept 评审 R ok 链;needs_changes 一轮
//     → 评审 fail + 下一 R 行;熔断 → 尾行 fail + note fuse → waiting_approval;approve 续跑 → 新 R 行;
//     split → planner do 行 + N 子任务(子任务无 plan);ask → pending do 行 note。
//   - T5 计划审批(risk=high ops_patrol):建单可跑但 claim 即 waiting_approval、5 行全 pending(GET 可见);
//     Decide approve → requeue → 执行推进(completed 时 1-4 ok + 5 依 ok 绿 skipped);reject → failed。
//     risk=medium patrol → 不 waiting,直接完成。
//   - T6 删除断引用:project/pipeline delete 后任务 + task_plan/phase 保留(task.project_id 置空 /
//     pipeline_id 置空)、pipelines/project 行删、磁盘目录仍在(plan 最小 FK task,无级联)。

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glacierzzz26/one-person-company-os/internal/pipeline"
	"github.com/glacierzzz26/one-person-company-os/internal/plan"
)

// loadPlan 取 task 计划 + 阶段(不存在即测试失败)。
func loadPlan(t *testing.T, svc *Service, taskID string) (plan.RunPlan, []plan.RunPlanPhase) {
	t.Helper()
	rp, phases, ok, err := svc.GetTaskPlan(context.Background(), taskID)
	if err != nil {
		t.Fatalf("GetTaskPlan(%s): %v", short8(taskID), err)
	}
	if !ok {
		t.Fatalf("GetTaskPlan(%s): plan not found (want pre-materialized)", short8(taskID))
	}
	return rp, phases
}

// phaseBySeq 把阶段列表按 seq 建索引(seq1..5)。
func phaseBySeq(phases []plan.RunPlanPhase) map[int64]plan.RunPlanPhase {
	m := make(map[int64]plan.RunPlanPhase, len(phases))
	for _, p := range phases {
		m[p.Seq] = p
	}
	return m
}

// ---- T1:建单即铺账(三形态 + 非流水线无 plan)----

func TestT1PlanShapeOnRunCreate(t *testing.T) {
	t.Setenv("OS_ENGINE_MODE", "scripted")
	svc, st := newSvc(t)
	compID := seedCompanyID(t, st)
	ctx := context.Background()
	prj := mustCreateProject(t, svc, compID, "web", filepath.Join(t.TempDir(), "web"))

	patrolPL := mustCreateOpsPatrol(t, svc, prj.ID, "patrol-daily", "")
	bugfixPL := mustCreatePipeline(t, svc, prj.ID, "fix", pipeline.KindBugfix, "")
	devPL, err := svc.CreatePipeline(ctx, prj.ID, "ship", pipeline.KindDevelop, "ship a feature", pipeline.RiskMedium, "")
	if err != nil {
		t.Fatalf("T1 create develop pipeline: %v", err)
	}

	// ops_patrol → upfront 铺全 5 条 pending(seq1..5)。
	pTk, err := svc.RunPipeline(ctx, patrolPL.ID, "")
	if err != nil {
		t.Fatalf("T1 run patrol: %v", err)
	}
	rp, phases := loadPlan(t, svc, pTk.ID)
	if rp.Kind != plan.PlanKindPatrol || rp.Materialized != plan.MaterializedUpfront {
		t.Fatalf("T1 patrol plan kind/mat = %s/%s, want patrol/upfront", rp.Kind, rp.Materialized)
	}
	if len(phases) != 5 {
		t.Fatalf("T1 patrol phases = %d, want 5 pre-materialized", len(phases))
	}
	pm := phaseBySeq(phases)
	for seq := int64(1); seq <= 5; seq++ {
		ph, ok := pm[seq]
		if !ok {
			t.Fatalf("T1 patrol seq %d missing (have %d phases)", seq, len(phases))
		}
		if ph.Status != plan.PhaseStatusPending {
			t.Fatalf("T1 patrol seq%d status = %s, want pending (先审后干)", seq, ph.Status)
		}
	}
	// 模板行类型/分配器逐段核对(契约 §3.3)。
	want := []struct {
		kind      string
		allocator string
	}{
		{plan.PhaseKindDo, plan.AllocatorDelegate},
		{plan.PhaseKindDo, plan.AllocatorOS},
		{plan.PhaseKindAccept, plan.AllocatorOS},
		{plan.PhaseKindAccept, plan.AllocatorJudge},
		{plan.PhaseKindDispose, plan.AllocatorManual},
	}
	for seq := int64(1); seq <= 5; seq++ {
		if pm[seq].Kind != want[seq-1].kind || pm[seq].Allocator != want[seq-1].allocator {
			t.Fatalf("T1 seq%d kind/alloc = %s/%s, want %s/%s", seq, pm[seq].Kind, pm[seq].Allocator, want[seq-1].kind, want[seq-1].allocator)
		}
	}
	// 完成历史 run 后继续建(串行守卫)。
	if _, err := st.CompleteTask(ctx, pTk.ID, "done"); err != nil {
		t.Fatal(err)
	}

	// bugfix + develop(grow):只建 plan 行,materialized=grow、无 phase。
	for i, pl := range []pipeline.Pipeline{bugfixPL, devPL} {
		gTk, err := svc.RunPipeline(ctx, pl.ID, "")
		if err != nil {
			t.Fatalf("T1 run grow pipeline #%d: %v", i, err)
		}
		grp, gph := loadPlan(t, svc, gTk.ID)
		if grp.Kind != plan.PlanKindEngineering || grp.Materialized != plan.MaterializedGrow {
			t.Fatalf("T1 grow kind/mat = %s/%s, want engineering/grow", grp.Kind, grp.Materialized)
		}
		if len(gph) != 0 {
			t.Fatalf("T1 grow phases = %d, want 0 (执行中 append)", len(gph))
		}
		if _, err := st.CompleteTask(ctx, gTk.ID, "done"); err != nil {
			t.Fatal(err)
		}
	}

	// 非流水线手动 createTask → 无 plan(即使 engineering 家族)。
	manual, err := svc.CreateTaskAs(ctx, TaskParams{
		CompanyID: compID, Title: "manual request", Description: "not a pipeline run",
		ToolName: "engineering", Risk: "medium", Workspace: prj.RootPath,
	}, "test")
	if err != nil {
		t.Fatalf("T1 manual createTask: %v", err)
	}
	if _, ok, err := st.GetTaskPlanByTask(ctx, manual.ID); err != nil {
		t.Fatalf("T1 manual plan lookup: %v", err)
	} else if ok {
		t.Fatal("T1 manual (non-pipeline) task must have no run plan")
	}
}

// ---- T2:patrol 全流程后账本镜像 ----

func TestT2PatrolLedgerMirrorOK(t *testing.T) {
	t.Setenv("OS_ENGINE_MODE", "scripted")
	svc, _, prj := scriptedHarness(t, "web")
	ctx := context.Background()
	pl := mustCreateOpsPatrol(t, svc, prj.ID, "patrol", "")
	tk, err := svc.RunPipeline(ctx, pl.ID, "")
	if err != nil {
		t.Fatalf("T2 run: %v", err)
	}
	if err := svc.ExecuteTask(ctx, "w1", tk.ID); err != nil {
		t.Fatalf("T2 ExecuteTask: %v", err)
	}
	got, _ := svc.GetTask(ctx, tk.ID)
	if got.Status != "completed" {
		t.Fatalf("T2 status = %s, want completed (last=%s)", got.Status, got.LastError)
	}

	_, phases := loadPlan(t, svc, tk.ID)
	pm := phaseBySeq(phases)
	reportRel := filepath.ToSlash(filepath.Join(PatrolDirName, tk.ID+".md"))
	// seq1..4 → ok + evidence;seq5 ok 巡检 → skipped。
	for _, seq := range []int64{1, 2, 3, 4} {
		if pm[seq].Status != plan.PhaseStatusOK {
			t.Fatalf("T2 seq%d status = %s, want ok (evidence=%q)", seq, pm[seq].Status, pm[seq].Evidence)
		}
	}
	if !strings.Contains(pm[1].Evidence, reportRel) {
		t.Fatalf("T2 seq1 evidence = %q, want report rel %q", pm[1].Evidence, reportRel)
	}
	if !strings.Contains(pm[2].Evidence, "committed") {
		t.Fatalf("T2 seq2 evidence = %q, want committed", pm[2].Evidence)
	}
	if !strings.Contains(pm[3].Evidence, "mechanical: report") || !strings.Contains(pm[3].Evidence, "residue=0") {
		t.Fatalf("T2 seq3 evidence = %q, want mechanical residue=0", pm[3].Evidence)
	}
	if !strings.Contains(pm[4].Evidence, "ok=true") {
		t.Fatalf("T2 seq4 evidence = %q, want verdict ok=true", pm[4].Evidence)
	}
	if pm[5].Status != plan.PhaseStatusSkipped || !strings.Contains(pm[5].Note, "ok — no disposition") {
		t.Fatalf("T2 seq5 status/note = %s/%q, want skipped/ok — no disposition", pm[5].Status, pm[5].Note)
	}
}

func TestT2PatrolLedgerMirrorFindingChain(t *testing.T) {
	t.Setenv("OS_ENGINE_MODE", "scripted")
	t.Setenv("OS_SCRIPT_PATROL", "finding-high")
	svc, _, prj := scriptedHarness(t, "web")
	ctx := context.Background()
	patrolPL := mustCreateOpsPatrol(t, svc, prj.ID, "patrol", "")
	if _, err := svc.CreatePipeline(ctx, prj.ID, "fix-any", pipeline.KindBugfix,
		"apply remediation for patrol findings", pipeline.RiskMedium, ""); err != nil {
		t.Fatalf("T2 create bugfix chain target: %v", err)
	}
	tk, err := svc.RunPipeline(ctx, patrolPL.ID, "")
	if err != nil {
		t.Fatalf("T2 run: %v", err)
	}
	if err := svc.ExecuteTask(ctx, "w1", tk.ID); err != nil {
		t.Fatalf("T2 ExecuteTask: %v", err)
	}
	got, _ := svc.GetTask(ctx, tk.ID)
	if got.Status != "completed" {
		t.Fatalf("T2 status = %s, want completed", got.Status)
	}

	_, phases := loadPlan(t, svc, tk.ID)
	pm := phaseBySeq(phases)
	// seq1..4 同 ok(裁决非 ok 不影响机械/判读行 ok)。
	for _, seq := range []int64{1, 2, 3, 4} {
		if pm[seq].Status != plan.PhaseStatusOK {
			t.Fatalf("T2 finding seq%d status = %s, want ok", seq, pm[seq].Status)
		}
	}
	// seq5.dispose → ok(链拉),evidence=severity/action,note 记链目标。
	if pm[5].Status != plan.PhaseStatusOK {
		t.Fatalf("T2 finding seq5 status = %s, want ok (disposition ran)", pm[5].Status)
	}
	if !strings.Contains(pm[5].Evidence, "severity=high") || !strings.Contains(pm[5].Evidence, "action=fix") {
		t.Fatalf("T2 finding seq5 evidence = %q, want severity=high action=fix", pm[5].Evidence)
	}
	if !strings.Contains(pm[5].Note, "chain → bugfix") {
		t.Fatalf("T2 finding seq5 note = %q, want chain → bugfix", pm[5].Note)
	}
}

// ---- T3:机械卫生预检层(D4)----

func TestT3MechanicalResidueFailsBeforeJudge(t *testing.T) {
	t.Setenv("OS_ENGINE_MODE", "scripted")
	t.Setenv("OS_SCRIPT_PATROL", "residue") // 写完报告再植越界残留(仅 scripted 分支可达)
	svc, _, prj := scriptedHarness(t, "web")
	ctx := context.Background()
	pl := mustCreateOpsPatrol(t, svc, prj.ID, "patrol", "")
	tk, err := svc.RunPipeline(ctx, pl.ID, "")
	if err != nil {
		t.Fatalf("T3 residue run: %v", err)
	}
	if err := svc.ExecuteTask(ctx, "w1", tk.ID); err != nil {
		t.Fatalf("T3 residue ExecuteTask: %v", err)
	}
	got, _ := svc.GetTask(ctx, tk.ID)
	if got.Status != "failed" {
		t.Fatalf("T3 residue: status = %s, want failed (委派越界 = fail-closed)", got.Status)
	}
	if !strings.Contains(got.LastError, "inspector residue") {
		t.Fatalf("T3 residue last_error = %q, want inspector residue", got.LastError)
	}

	// 账本:seq3.accept = fail(带 why);seq1/2 ok;seq4/5 仍 pending(判读未到)。
	_, phases := loadPlan(t, svc, tk.ID)
	pm := phaseBySeq(phases)
	if pm[1].Status != plan.PhaseStatusOK || pm[2].Status != plan.PhaseStatusOK {
		t.Fatalf("T3 seq1/2 status = %s/%s, want ok (报告产出与提交在先)", pm[1].Status, pm[2].Status)
	}
	if pm[3].Status != plan.PhaseStatusFail || !strings.Contains(pm[3].Note, "inspector residue") {
		t.Fatalf("T3 seq3 status/note = %s/%q, want fail + residue note", pm[3].Status, pm[3].Note)
	}
	if pm[4].Status != plan.PhaseStatusPending || pm[5].Status != plan.PhaseStatusPending {
		t.Fatalf("T3 seq4/5 status = %s/%s, want pending (judge/dispose never reached)", pm[4].Status, pm[5].Status)
	}
	// 残留文件仍躺在工作树(未被吞/提交)。
	if _, err := os.Stat(filepath.Join(prj.RootPath, "residue-inspector.txt")); err != nil {
		t.Fatalf("T3 residue file should remain on disk: %v", err)
	}
}

func TestT3MechanicalPreExistingDirtyNoFalsePositive(t *testing.T) {
	t.Setenv("OS_ENGINE_MODE", "scripted")
	svc, _, prj := scriptedHarness(t, "web")
	ctx := context.Background()
	pl := mustCreateOpsPatrol(t, svc, prj.ID, "patrol", "")
	tk, err := svc.RunPipeline(ctx, pl.ID, "")
	if err != nil {
		t.Fatalf("T3 dirty run: %v", err)
	}
	// 委派前已有用户脏项(untracked)→ pre 快照含 → 复检不误伤。
	userDirty := filepath.Join(prj.RootPath, "user-note.txt")
	if err := os.WriteFile(userDirty, []byte("my scratch\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := svc.ExecuteTask(ctx, "w1", tk.ID); err != nil {
		t.Fatalf("T3 dirty ExecuteTask: %v", err)
	}
	got, _ := svc.GetTask(ctx, tk.ID)
	if got.Status != "completed" {
		t.Fatalf("T3 dirty: status = %s, want completed (user dirty not residue; last=%s)", got.Status, got.LastError)
	}
	_, phases := loadPlan(t, svc, tk.ID)
	pm := phaseBySeq(phases)
	if pm[3].Status != plan.PhaseStatusOK || !strings.Contains(pm[3].Evidence, "residue=0") {
		t.Fatalf("T3 dirty seq3 status/evidence = %s/%q, want ok residue=0", pm[3].Status, pm[3].Evidence)
	}
}

// TestT3MechanicalPatrolCheckUnit 机械判定纯函数(报告缺/空 → fail;正常 → 放行;新增残留 → fail;
// pre 已有脏项 → 放行)零网关、确定性。
func TestT3MechanicalPatrolCheckUnit(t *testing.T) {
	ctx := context.Background()
	ws := seedGitWorkspace(t) // 干净 git 工作区(base.txt/gone.txt 已提交)

	// 报告缺/空(空白正文)→ 必 fail,不进 git 复检。
	if r := mechanicalPatrolCheck(ctx, ws, nil, "   \n", "patrol/x.md"); r == "" || !strings.Contains(r, "empty") {
		t.Fatalf("T3 empty report reason = %q, want empty-report fail", r)
	}

	// 报告正常 + 工作树干净 → 放行。
	if r := mechanicalPatrolCheck(ctx, ws, nil, "# Patrol Report\nok", "patrol/x.md"); r != "" {
		t.Fatalf("T3 clean pass should be empty reason, got %q", r)
	}

	// 报告写好后塞越界残留(不在 pre)→ fail 列出条目。
	pre, err := gitPorcelainSet(ctx, ws)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, "stray-inspector.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := mechanicalPatrolCheck(ctx, ws, pre, "# Patrol Report\nok", "patrol/x.md")
	if r == "" || !strings.Contains(r, "stray-inspector.txt") {
		t.Fatalf("T3 residue reason = %q, want listing stray-inspector.txt", r)
	}

	// 用户 pre 已存在脏项(pre 快照含)→ 不误伤。
	pre2, err := gitPorcelainSet(ctx, ws)
	if err != nil {
		t.Fatal(err)
	}
	if !pre2["?? stray-inspector.txt"] {
		t.Fatalf("T3 pre2 should carry the stray entry (porcelain=%v)", pre2)
	}
	if r := mechanicalPatrolCheck(ctx, ws, pre2, "# Patrol Report\nok", "patrol/x.md"); r != "" {
		t.Fatalf("T3 pre-existing dirty must not be flagged, got %q", r)
	}
}

// ---- T4:engineering(grow)记账 ----

func TestT4EngineeringGrowDirect(t *testing.T) {
	t.Setenv("OS_ENGINE_MODE", "scripted")
	svc, _, prj := scriptedHarness(t, "web")
	ctx := context.Background()
	pl := mustCreatePipeline(t, svc, prj.ID, "fix", pipeline.KindBugfix, "")
	tk, err := svc.RunPipeline(ctx, pl.ID, "")
	if err != nil {
		t.Fatalf("T4 run: %v", err)
	}
	if err := svc.ExecuteTask(ctx, "w1", tk.ID); err != nil {
		t.Fatalf("T4 ExecuteTask: %v", err)
	}
	got, _ := svc.GetTask(ctx, tk.ID)
	if got.Status != "completed" {
		t.Fatalf("T4 direct status = %s, want completed (last=%s)", got.Status, got.LastError)
	}
	_, phases := loadPlan(t, svc, tk.ID)
	if len(phases) != 3 {
		t.Fatalf("T4 direct phases = %d, want 3 (写/测/审 round 0)", len(phases))
	}
	assertPhase := func(seq int64, kind, title, status string) {
		p := phaseBySeq(phases)[seq]
		if p.Kind != kind || p.Title != title || p.Status != status {
			t.Fatalf("T4 seq%d = %s/%q/%s, want %s/%q/%s", seq, p.Kind, p.Title, p.Status, kind, title, status)
		}
	}
	assertPhase(1, plan.PhaseKindDo, "写 round 0", plan.PhaseStatusOK)
	assertPhase(2, plan.PhaseKindAccept, "测试 round 0", plan.PhaseStatusOK)
	assertPhase(3, plan.PhaseKindAccept, "评审 round 0", plan.PhaseStatusOK)
}

func TestT4EngineeringGrowNeedsChangesAppend(t *testing.T) {
	t.Setenv("OS_ENGINE_MODE", "scripted")
	t.Setenv("OS_SCRIPT_REVIEW", "reject:1") // round 0 驳回 1 次 → round 1 放行
	svc, _, prj := scriptedHarness(t, "web")
	ctx := context.Background()
	pl := mustCreatePipeline(t, svc, prj.ID, "fix", pipeline.KindBugfix, "")
	tk, err := svc.RunPipeline(ctx, pl.ID, "")
	if err != nil {
		t.Fatalf("T4 run: %v", err)
	}
	if err := svc.ExecuteTask(ctx, "w1", tk.ID); err != nil {
		t.Fatalf("T4 ExecuteTask: %v", err)
	}
	got, _ := svc.GetTask(ctx, tk.ID)
	if got.Status != "completed" {
		t.Fatalf("T4 rework status = %s, want completed (last=%s)", got.Status, got.LastError)
	}
	_, phases := loadPlan(t, svc, tk.ID)
	if len(phases) != 6 {
		t.Fatalf("T4 rework phases = %d, want 6 (round0 fail 链 + round1 ok 链)", len(phases))
	}
	pm := phaseBySeq(phases)
	if pm[3].Status != plan.PhaseStatusFail {
		t.Fatalf("T4 rework seq3 (评审 round 0) status = %s, want fail", pm[3].Status)
	}
	if pm[4].Title != "写 round 1" || pm[4].Status != plan.PhaseStatusOK {
		t.Fatalf("T4 rework seq4 = %q/%s, want 写 round 1/ok (下一 R 追加)", pm[4].Title, pm[4].Status)
	}
	if pm[6].Title != "评审 round 1" || pm[6].Status != plan.PhaseStatusOK {
		t.Fatalf("T4 rework seq6 = %q/%s, want 评审 round 1/ok", pm[6].Title, pm[6].Status)
	}
}

func TestT4EngineeringFuseResume(t *testing.T) {
	t.Setenv("OS_ENGINE_MODE", "scripted")
	t.Setenv("OS_SCRIPT_REVIEW", "reject") // 持续驳回 → round2 达熔断阈值 → waiting_approval
	svc, _, prj := scriptedHarness(t, "web")
	ctx := context.Background()
	pl := mustCreatePipeline(t, svc, prj.ID, "fix", pipeline.KindBugfix, "")
	tk, err := svc.RunPipeline(ctx, pl.ID, "")
	if err != nil {
		t.Fatalf("T4 fuse run: %v", err)
	}
	if err := svc.ExecuteTask(ctx, "w1", tk.ID); err != nil {
		t.Fatalf("T4 fuse ExecuteTask: %v", err)
	}
	got, _ := svc.GetTask(ctx, tk.ID)
	if got.Status != "waiting_approval" {
		t.Fatalf("T4 fuse status = %s, want waiting_approval (conflict hit fuse)", got.Status)
	}
	_, phases := loadPlan(t, svc, tk.ID)
	if len(phases) != 9 {
		t.Fatalf("T4 fuse phases = %d, want 9 (3 rounds × 写/测/审)", len(phases))
	}
	tail := phases[len(phases)-1]
	if tail.Title != "评审 round 2" || tail.Status != plan.PhaseStatusFail || !strings.Contains(tail.Note, "fuse → waiting_approval") {
		t.Fatalf("T4 fuse tail = %q/%s note=%q, want 评审 round 2 fail + fuse note", tail.Title, tail.Status, tail.Note)
	}

	// approve → requeue → 续跑按新 R(round 3)追加并完成。
	appr, err := svc.ListApprovals(ctx, "pending")
	if err != nil {
		t.Fatal(err)
	}
	var apID string
	for _, a := range appr {
		if a.TaskID == tk.ID {
			apID = a.ID
		}
	}
	if apID == "" {
		t.Fatalf("T4 fuse approval row not found for task %s", short8(tk.ID))
	}
	if _, err := svc.DecideApprovalAs(ctx, apID, "approve", "approved after fuse", "human:test"); err != nil {
		t.Fatalf("T4 fuse approve: %v", err)
	}
	if err := svc.ExecuteTask(ctx, "w1", tk.ID); err != nil {
		t.Fatalf("T4 resume ExecuteTask: %v", err)
	}
	got, _ = svc.GetTask(ctx, tk.ID)
	if got.Status != "completed" {
		t.Fatalf("T4 resume status = %s, want completed (last=%s)", got.Status, got.LastError)
	}
	_, phases = loadPlan(t, svc, tk.ID)
	if len(phases) != 12 {
		t.Fatalf("T4 resume phases = %d, want 12 (9 + round 3)", len(phases))
	}
	tail = phases[len(phases)-1]
	if tail.Title != "评审 round 3" || tail.Status != plan.PhaseStatusOK {
		t.Fatalf("T4 resume tail = %q/%s, want 评审 round 3 ok", tail.Title, tail.Status)
	}
}

func TestT4EngineeringSplitSubtaskNoPlan(t *testing.T) {
	t.Setenv("OS_ENGINE_MODE", "scripted")
	t.Setenv("OS_SCRIPT_PLAN", "split:2")
	svc, st, prj := scriptedHarness(t, "web")
	ctx := context.Background()
	pl := mustCreatePipeline(t, svc, prj.ID, "big-request", pipeline.KindBugfix, "")
	tk, err := svc.RunPipeline(ctx, pl.ID, "")
	if err != nil {
		t.Fatalf("T4 split run: %v", err)
	}
	if err := svc.ExecuteTask(ctx, "w1", tk.ID); err != nil {
		t.Fatalf("T4 split ExecuteTask: %v", err)
	}
	got, _ := svc.GetTask(ctx, tk.ID)
	if got.Status != "completed" {
		t.Fatalf("T4 split status = %s, want completed (all children aggregated; last=%s)", got.Status, got.LastError)
	}
	// 父 run 计划 = 1 行 planner 拆解序曲。
	_, phases := loadPlan(t, svc, tk.ID)
	if len(phases) != 1 {
		t.Fatalf("T4 split parent phases = %d, want 1 (planner do 行)", len(phases))
	}
	ph := phases[0]
	if ph.Title != "planner 拆解序曲" || ph.Allocator != plan.AllocatorPlanner || ph.Status != plan.PhaseStatusOK ||
		!strings.Contains(ph.Evidence, "split → 2 subtasks") {
		t.Fatalf("T4 split parent phase = %q/%s ev=%q, want planner 拆解序曲 ok split → 2", ph.Title, ph.Status, ph.Evidence)
	}
	// 子任务 2 条且各自无 plan(非流水线产物)。
	children, err := st.ListTasksByParent(ctx, tk.ID)
	if err != nil || len(children) != 2 {
		t.Fatalf("T4 split children = %d (err=%v), want 2", len(children), err)
	}
	for _, c := range children {
		if _, ok, err := st.GetTaskPlanByTask(ctx, c.ID); err != nil {
			t.Fatalf("T4 child plan lookup: %v", err)
		} else if ok {
			t.Fatal("T4 subtask must have no run plan")
		}
	}
}

func TestT4EngineeringAskPendingRow(t *testing.T) {
	t.Setenv("OS_ENGINE_MODE", "scripted")
	t.Setenv("OS_SCRIPT_PLAN", "ask:needs human decision")
	svc, _, prj := scriptedHarness(t, "web")
	ctx := context.Background()
	pl := mustCreatePipeline(t, svc, prj.ID, "vague", pipeline.KindBugfix, "")
	tk, err := svc.RunPipeline(ctx, pl.ID, "")
	if err != nil {
		t.Fatalf("T4 ask run: %v", err)
	}
	if err := svc.ExecuteTask(ctx, "w1", tk.ID); err != nil {
		t.Fatalf("T4 ask ExecuteTask: %v", err)
	}
	got, _ := svc.GetTask(ctx, tk.ID)
	if got.Status != "waiting_approval" {
		t.Fatalf("T4 ask status = %s, want waiting_approval (planner ask → human)", got.Status)
	}
	_, phases := loadPlan(t, svc, tk.ID)
	if len(phases) != 1 {
		t.Fatalf("T4 ask phases = %d, want 1 (pending do 行)", len(phases))
	}
	ph := phases[0]
	if ph.Status != plan.PhaseStatusPending || !strings.Contains(ph.Note, "ask → 人工审批:") {
		t.Fatalf("T4 ask phase status/note = %s/%q, want pending + ask → 人工审批", ph.Status, ph.Note)
	}
}

// ---- T5:计划审批(risk=high ops_patrol = 既有单门 + upfront 计划可见)----

func TestT5HighRiskPatrolApproveThenRun(t *testing.T) {
	t.Setenv("OS_ENGINE_MODE", "scripted")
	svc, _, prj := scriptedHarness(t, "web")
	ctx := context.Background()
	// risk=high ops_patrol(needsApproval 门,reason=risk=high)。
	pl, err := svc.CreatePipeline(ctx, prj.ID, "patrol-high", pipeline.KindOpsPatrol,
		"check: git hygiene + build", pipeline.RiskHigh, "")
	if err != nil {
		t.Fatalf("T5 create high patrol: %v", err)
	}
	tk, err := svc.RunPipeline(ctx, pl.ID, "")
	if err != nil {
		t.Fatalf("T5 run: %v", err)
	}
	if err := svc.ExecuteTask(ctx, "w1", tk.ID); err != nil {
		t.Fatalf("T5 ExecuteTask(claim): %v", err)
	}
	got, _ := svc.GetTask(ctx, tk.ID)
	if got.Status != "waiting_approval" {
		t.Fatalf("T5 status = %s, want waiting_approval (先审后干)", got.Status)
	}
	// waiting_approval 时 plan 5 行全 pending(GET 可见 = loadPlan)。
	rp, phases := loadPlan(t, svc, tk.ID)
	if rp.Materialized != plan.MaterializedUpfront || len(phases) != 5 {
		t.Fatalf("T5 plan mat/len = %s/%d, want upfront/5", rp.Materialized, len(phases))
	}
	for _, ph := range phases {
		if ph.Status != plan.PhaseStatusPending {
			t.Fatalf("T5 pre-approval seq%d status = %s, want pending (nothing ran yet)", ph.Seq, ph.Status)
		}
	}

	// approve → requeue → 执行推进:completed 时 1-4 ok + 5 skipped(ok 巡检)。
	appr, err := svc.ListApprovals(ctx, "pending")
	if err != nil {
		t.Fatal(err)
	}
	var apID string
	for _, a := range appr {
		if a.TaskID == tk.ID {
			apID = a.ID
		}
	}
	if apID == "" {
		t.Fatalf("T5 approval row missing for %s", short8(tk.ID))
	}
	if _, err := svc.DecideApprovalAs(ctx, apID, "approve", "proceed with pre-approved plan", "human:test"); err != nil {
		t.Fatalf("T5 approve: %v", err)
	}
	if err := svc.ExecuteTask(ctx, "w1", tk.ID); err != nil {
		t.Fatalf("T5 ExecuteTask(resume): %v", err)
	}
	got, _ = svc.GetTask(ctx, tk.ID)
	if got.Status != "completed" {
		t.Fatalf("T5 approved status = %s, want completed (last=%s)", got.Status, got.LastError)
	}
	_, phases = loadPlan(t, svc, tk.ID)
	pm := phaseBySeq(phases)
	for _, seq := range []int64{1, 2, 3, 4} {
		if pm[seq].Status != plan.PhaseStatusOK {
			t.Fatalf("T5 approved seq%d status = %s, want ok", seq, pm[seq].Status)
		}
	}
	if pm[5].Status != plan.PhaseStatusSkipped {
		t.Fatalf("T5 approved seq5 status = %s, want skipped (ok → no disposition)", pm[5].Status)
	}
}

func TestT5HighRiskPatrolRejectFails(t *testing.T) {
	t.Setenv("OS_ENGINE_MODE", "scripted")
	svc, _, prj := scriptedHarness(t, "web")
	ctx := context.Background()
	pl, err := svc.CreatePipeline(ctx, prj.ID, "patrol-high", pipeline.KindOpsPatrol,
		"check: git hygiene + build", pipeline.RiskHigh, "")
	if err != nil {
		t.Fatalf("T5 create high patrol: %v", err)
	}
	tk, err := svc.RunPipeline(ctx, pl.ID, "")
	if err != nil {
		t.Fatalf("T5 run: %v", err)
	}
	if err := svc.ExecuteTask(ctx, "w1", tk.ID); err != nil {
		t.Fatalf("T5 ExecuteTask(claim): %v", err)
	}
	got, _ := svc.GetTask(ctx, tk.ID)
	if got.Status != "waiting_approval" {
		t.Fatalf("T5 status = %s, want waiting_approval", got.Status)
	}
	appr, err := svc.ListApprovals(ctx, "pending")
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range appr {
		if a.TaskID == tk.ID {
			if _, err := svc.DecideApprovalAs(ctx, a.ID, "reject", "not authorized now", "human:test"); err != nil {
				t.Fatalf("T5 reject: %v", err)
			}
		}
	}
	got, _ = svc.GetTask(ctx, tk.ID)
	if got.Status != "failed" {
		t.Fatalf("T5 reject status = %s, want failed", got.Status)
	}
	// 计划行保持 pending(从未执行;先审后干被驳 → 零阶段推进)。
	_, phases := loadPlan(t, svc, tk.ID)
	for _, ph := range phases {
		if ph.Status != plan.PhaseStatusPending {
			t.Fatalf("T5 rejected seq%d status = %s, want pending (nothing ran)", ph.Seq, ph.Status)
		}
	}
}

func TestT5MediumPatrolNoWaiting(t *testing.T) {
	t.Setenv("OS_ENGINE_MODE", "scripted")
	svc, _, prj := scriptedHarness(t, "web")
	ctx := context.Background()
	pl := mustCreateOpsPatrol(t, svc, prj.ID, "patrol-medium", "")
	tk, err := svc.RunPipeline(ctx, pl.ID, "")
	if err != nil {
		t.Fatalf("T5 run: %v", err)
	}
	// 直接 claim → 完成,不经过 waiting_approval。
	if err := svc.ExecuteTask(ctx, "w1", tk.ID); err != nil {
		t.Fatalf("T5 ExecuteTask: %v", err)
	}
	got, _ := svc.GetTask(ctx, tk.ID)
	if got.Status != "completed" {
		t.Fatalf("T5 medium status = %s, want completed (no approval gate)", got.Status)
	}
	// 无 pending 审批挂在该 run。
	appr, err := svc.ListApprovals(ctx, "pending")
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range appr {
		if a.TaskID == tk.ID {
			t.Fatalf("T5 medium run must not create an approval")
		}
	}
}

// ---- T6:删除断引用(plan 随任务保留)----

func TestT6DeleteProjectKeepsPlan(t *testing.T) {
	t.Setenv("OS_ENGINE_MODE", "scripted")
	svc, st, prj := scriptedHarness(t, "web")
	ctx := context.Background()
	pl := mustCreateOpsPatrol(t, svc, prj.ID, "patrol", "")
	tk, err := svc.RunPipeline(ctx, pl.ID, "")
	if err != nil {
		t.Fatalf("T6 run: %v", err)
	}
	if err := svc.ExecuteTask(ctx, "w1", tk.ID); err != nil {
		t.Fatalf("T6 ExecuteTask: %v", err)
	}
	got, _ := svc.GetTask(ctx, tk.ID)
	if got.Status != "completed" {
		t.Fatalf("T6 patrol should complete before delete: %s", got.Status)
	}
	rp, phases := loadPlan(t, svc, tk.ID)
	if len(phases) != 5 {
		t.Fatalf("T6 pre-delete phases = %d, want 5", len(phases))
	}

	if err := svc.DeleteProject(ctx, prj.ID); err != nil {
		t.Fatalf("T6 delete project: %v", err)
	}
	// 任务保留 + project_id 置空;pipelines/project 行删。
	kept, err := svc.GetTask(ctx, tk.ID)
	if err != nil {
		t.Fatalf("T6 task must survive project delete: %v", err)
	}
	if kept.ProjectID != nil {
		t.Fatalf("T6 task.project_id = %v, want nil after project delete", *kept.ProjectID)
	}
	if _, err := svc.GetPipeline(ctx, pl.ID); err == nil {
		t.Fatal("T6 pipeline should cascade-delete with project")
	}
	if _, err := os.Stat(prj.RootPath); err != nil {
		t.Fatalf("T6 disk dir must remain: %v", err)
	}
	// plan + phases 随任务保留(最小 FK task,无级联)。
	if _, ok, err := st.GetTaskPlanByTask(ctx, tk.ID); err != nil || !ok {
		t.Fatalf("T6 plan should survive with task (ok=%v err=%v)", ok, err)
	}
	left, err := st.ListPlanPhases(ctx, rp.ID)
	if err != nil || len(left) != 5 {
		t.Fatalf("T6 phases after project delete = %d (err=%v), want 5 kept", len(left), err)
	}
}

func TestT6DeletePipelineKeepsPlan(t *testing.T) {
	t.Setenv("OS_ENGINE_MODE", "scripted")
	svc, st, prj := scriptedHarness(t, "web")
	ctx := context.Background()
	pl := mustCreateOpsPatrol(t, svc, prj.ID, "patrol", "")
	tk, err := svc.RunPipeline(ctx, pl.ID, "")
	if err != nil {
		t.Fatalf("T6 run: %v", err)
	}
	if err := svc.ExecuteTask(ctx, "w1", tk.ID); err != nil {
		t.Fatalf("T6 ExecuteTask: %v", err)
	}
	if got, _ := svc.GetTask(ctx, tk.ID); got.Status != "completed" {
		t.Fatalf("T6 patrol should complete before delete: %s", got.Status)
	}
	rp, _ := loadPlan(t, svc, tk.ID)

	if err := svc.DeletePipeline(ctx, pl.ID); err != nil {
		t.Fatalf("T6 delete pipeline: %v", err)
	}
	// 任务保留,pipeline_id 置空、project_id 仍在;project 行在。
	kept, err := svc.GetTask(ctx, tk.ID)
	if err != nil {
		t.Fatalf("T6 task must survive pipeline delete: %v", err)
	}
	if kept.PipelineID != nil {
		t.Fatalf("T6 task.pipeline_id = %v, want nil after pipeline delete", *kept.PipelineID)
	}
	if kept.ProjectID == nil || *kept.ProjectID != prj.ID {
		t.Fatalf("T6 task.project_id = %v, want project kept", kept.ProjectID)
	}
	if _, err := svc.GetProject(ctx, prj.ID); err != nil {
		t.Fatalf("T6 project must not be deleted: %v", err)
	}
	// plan + phases 保留。
	if _, ok, err := st.GetTaskPlanByTask(ctx, tk.ID); err != nil || !ok {
		t.Fatalf("T6 plan should survive pipeline delete (ok=%v err=%v)", ok, err)
	}
	if left, err := st.ListPlanPhases(ctx, rp.ID); err != nil || len(left) != 5 {
		t.Fatalf("T6 phases after pipeline delete = %d (err=%v), want 5", len(left), err)
	}
}
