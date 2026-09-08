package service

// Phase 10.4 契约用例(契约 docs/phase10/design/run-synthesis.md §五,SY*;scripted 公司 = env seam
// (TestMain 开)+ OS_ENGINE_MODE=scripted,产品零 env):
//   - SY1 合成先审后干主链:synthesize(bugfix,risk=high)run → 建单 plan grow(未认领诚实空态)→
//     claim1(OS_SCRIPT_SYNTH=合法 3 相位 JSON)→ upfront + 3 pending + 未执行任何 do(产出未落盘)→
//     waiting_approval;approve → claim2 → 逐相位执行(do scripted 产出+commit → accept os 全过 → dispose
//     manual note)→ completed;断言相位状态/evidence/起止 + 审计 eng_synth/eng_start/eng_complete,无 eng_synth_fail
//   - SY2 合成失败降级:OS_SCRIPT_SYNTH 空 → audit eng_synth_fail → 计划保持 grow → 走 runEngineering
//     (scripted planner+writer+test+review)完成;断言 grow 记账照旧 + 无半成品 upfront
//   - SY3 OS 机械 accept 允许清单:直接调 osMechanicalAccept 断言四分支(产出在+净残留=0 → ok;
//     output 缺失 → 拒;stray 越出 output → 拒;相位提交含空白错误 → 拒);驱动级 stray → 返工 1 → 相位 fail → engFail
//   - SY4 judge accept 返工上限:OS_SCRIPT_ACCEPT=fail-once(≤1 返工放行)→ ok;=fail(两次判读失败)→ 相位 fail → engFail
//   - SY5 默认 adaptive 零漂移 + ops_patrol 模板优先:synthesize policy 建 ops_patrol → runPatrol(非合成);
//     bugfix 无 policy → plan_policy=adaptive + 走 runEngineering(grow),零 eng_synth
//   - SY6 计划校验降级:缺 accept / os accept 无 output / 超 12 相位 / kind 非法 → eng_synth_fail + materialized 保持 grow

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glacierzzz26/one-person-company-os/internal/approval"
	"github.com/glacierzzz26/one-person-company-os/internal/pipeline"
	"github.com/glacierzzz26/one-person-company-os/internal/plan"
	"github.com/glacierzzz26/one-person-company-os/internal/storage/repository"
)

// ---- harness ----

// synthPlan3 合法 3 相位计划:do(delegate)+ accept(os,output fix.go)+ dispose(manual)。
const synthPlan3 = `{"summary":"fix flaky login","phases":[
 {"kind":"do","title":"implement login fix","allocator":"delegate","acceptance":"apply the fix","output":""},
 {"kind":"accept","title":"verify artifact","allocator":"os","acceptance":"fix.go exists and tree clean","output":"fix.go"},
 {"kind":"dispose","title":"note for human","allocator":"manual","acceptance":"","output":""}
]}`

// synthPlanJudge 合法 2 相位计划:do + accept(judge,无 os output)。
const synthPlanJudge = `{"summary":"refactor retry","phases":[
 {"kind":"do","title":"implement retry backoff","allocator":"delegate","acceptance":"retry backoff wired","output":""},
 {"kind":"accept","title":"judge retry semantics","allocator":"judge","acceptance":"backoff exponential and jitter capped","output":""}
]}`

// mustSynthPipeline 建 plan_policy=synthesize 流水线(risk/kind 由参数;断言白名单落库)。
func mustSynthPipeline(t *testing.T, svc *Service, prjID, name, kind, risk string) pipeline.Pipeline {
	t.Helper()
	pl, err := svc.CreatePipeline(context.Background(), prjID, name, kind, "intent "+name, risk, "",
		PipelinePlanPolicy(pipeline.PlanPolicySynthesize))
	if err != nil {
		t.Fatalf("CreatePipeline synthesize %s: %v", name, err)
	}
	if pl.PlanPolicy != pipeline.PlanPolicySynthesize {
		t.Fatalf("plan_policy = %q, want synthesize", pl.PlanPolicy)
	}
	return pl
}

// getPlan 取 run 的计划(断言用)。
func getPlan(t *testing.T, svc *Service, taskID string) (plan.RunPlan, []plan.RunPlanPhase) {
	t.Helper()
	rp, phases, ok, err := svc.GetTaskPlan(context.Background(), taskID)
	if err != nil {
		t.Fatalf("GetTaskPlan: %v", err)
	}
	if !ok {
		t.Fatalf("GetTaskPlan task=%s: plan missing", taskID)
	}
	return rp, phases
}

// pendingApprovalFor 找该任务的 pending 审批。
func pendingApprovalFor(t *testing.T, st *repository.Store, taskID string) approval.Approval {
	t.Helper()
	list, err := st.ListApprovals(context.Background(), "pending")
	if err != nil {
		t.Fatalf("ListApprovals: %v", err)
	}
	for _, a := range list {
		if a.TaskID == taskID {
			return a
		}
	}
	t.Fatalf("no pending approval for task %s", taskID)
	return approval.Approval{}
}

// hasAudit 断言 task 实体存在某动作审计。
func hasAudit(t *testing.T, st *repository.Store, taskID, action string) bool {
	t.Helper()
	for _, a := range listAudits(t, st, "task") {
		if a.EntityID == taskID && a.Action == action {
			return true
		}
	}
	return false
}

// phaseAt 按 seq 找相位(plan_test.go 已占用 phaseBySeq 名,避开)。
func phaseAt(t *testing.T, svc *Service, taskID string, seq int64) plan.RunPlanPhase {
	t.Helper()
	_, phases := getPlan(t, svc, taskID)
	for _, ph := range phases {
		if ph.Seq == seq {
			return ph
		}
	}
	t.Fatalf("phase seq=%d missing", seq)
	return plan.RunPlanPhase{}
}

// ---- SY1:合成先审后干主链 ----

func TestSY1SynthesizeApproveThenExecute(t *testing.T) {
	t.Setenv("OS_ENGINE_MODE", "scripted")
	t.Setenv("OS_SCRIPT_SYNTH", synthPlan3)
	svc, st, prj := scriptedHarness(t, "sya")
	ctx := context.Background()
	pl := mustSynthPipeline(t, svc, prj.ID, "login-fix", pipeline.KindBugfix, pipeline.RiskHigh)

	run, err := svc.RunPipeline(ctx, pl.ID, "fix flaky login 500")
	if err != nil {
		t.Fatalf("SY1 run: %v", err)
	}
	// 建单诚实:materialized=grow(未认领不合成)。
	rp0, phases0 := getPlan(t, svc, run.ID)
	if rp0.Materialized != plan.MaterializedGrow {
		t.Fatalf("pre-claim materialized=%s, want grow (honest empty state)", rp0.Materialized)
	}
	if len(phases0) != 0 {
		t.Fatalf("pre-claim phases=%d, want 0", len(phases0))
	}

	// claim1:合成计划 → 落 upfront 账本 → 审批门(未执行任何 do)。
	if err := svc.ExecuteTask(ctx, "w1", run.ID); err != nil {
		t.Fatalf("SY1 claim1: %v", err)
	}
	got1, _ := svc.GetTask(ctx, run.ID)
	if got1.Status != "waiting_approval" {
		t.Fatalf("SY1 after claim1 status=%s, want waiting_approval (last=%s)", got1.Status, got1.LastError)
	}
	rp1, phases1 := getPlan(t, svc, run.ID)
	if rp1.Materialized != plan.MaterializedUpfront {
		t.Fatalf("SY1 post-synth materialized=%s, want upfront", rp1.Materialized)
	}
	if len(phases1) != 3 {
		t.Fatalf("SY1 post-synth phases=%d, want 3", len(phases1))
	}
	for _, ph := range phases1 {
		if ph.Status != plan.PhaseStatusPending {
			t.Fatalf("SY1 phase seq=%d kind=%s status=%s, want pending (先审后干:审批前不执行)", ph.Seq, ph.Kind, ph.Status)
		}
	}
	// 先审后干铁证:合成后 do 尚未产出(产出文件未落盘)。
	if _, err := os.Stat(filepath.Join(prj.RootPath, "fix.go")); !os.IsNotExist(err) {
		t.Fatalf("SY1 do must not run before approval (fix.go present): %v", err)
	}
	if !hasAudit(t, st, run.ID, "eng_synth") {
		t.Fatalf("SY1 missing audit eng_synth")
	}
	if hasAudit(t, st, run.ID, "eng_synth_fail") {
		t.Fatalf("SY1 unexpected eng_synth_fail")
	}

	// approve → 复认领执行(逐相位)。
	a := pendingApprovalFor(t, st, run.ID)
	if _, err := svc.DecideApprovalAs(ctx, a.ID, "approve", "", "human:console"); err != nil {
		t.Fatalf("SY1 approve: %v", err)
	}
	if err := svc.ExecuteTask(ctx, "w1", run.ID); err != nil {
		t.Fatalf("SY1 claim2: %v", err)
	}
	got2, _ := svc.GetTask(ctx, run.ID)
	if got2.Status != "completed" {
		t.Fatalf("SY1 final status=%s, want completed (last=%s)", got2.Status, got2.LastError)
	}
	if !strings.HasPrefix(got2.Result, "synth: 3 phases executed") {
		t.Fatalf("SY1 result=%q, want synth completion marker", got2.Result)
	}
	// 产出真落盘且相位终态齐。
	if _, err := os.Stat(filepath.Join(prj.RootPath, "fix.go")); err != nil {
		t.Fatalf("SY1 fix.go missing after execute: %v", err)
	}
	for _, seq := range []int64{1, 2, 3} {
		ph := phaseAt(t, svc, run.ID, seq)
		if ph.Status != plan.PhaseStatusOK {
			t.Fatalf("SY1 phase seq=%d status=%s, want ok", seq, ph.Status)
		}
		if ph.StartedAt == nil || ph.FinishedAt == nil {
			t.Fatalf("SY1 phase seq=%d timestamps missing (started=%v finished=%v)", seq, ph.StartedAt, ph.FinishedAt)
		}
	}
	do := phaseAt(t, svc, run.ID, 1)
	if strings.TrimSpace(do.Evidence) == "" {
		t.Fatalf("SY1 do evidence empty")
	}
	// 审计闭环。
	for _, want := range []string{"eng_start", "eng_synth", "eng_complete"} {
		if !hasAudit(t, st, run.ID, want) {
			t.Fatalf("SY1 missing audit %s", want)
		}
	}
}

// ---- SY2:合成失败降级(自适应 runEngineering 照常) ----

func TestSY2SynthFailDegradesToAdaptive(t *testing.T) {
	t.Setenv("OS_ENGINE_MODE", "scripted")
	t.Setenv("OS_SCRIPT_SYNTH", "") // 空 → 合成不可得 → 降级
	t.Setenv("OS_SCRIPT_TEST", "pass")
	t.Setenv("OS_SCRIPT_REVIEW", "approve")
	svc, st, prj := scriptedHarness(t, "syb")
	ctx := context.Background()
	pl := mustSynthPipeline(t, svc, prj.ID, "sync", pipeline.KindDevelop, pipeline.RiskLow)

	run, err := svc.RunPipeline(ctx, pl.ID, "make sync reliable")
	if err != nil {
		t.Fatalf("SY2 run: %v", err)
	}
	if err := svc.ExecuteTask(ctx, "w1", run.ID); err != nil {
		t.Fatalf("SY2 claim: %v", err)
	}
	got, _ := svc.GetTask(ctx, run.ID)
	if got.Status != "completed" {
		t.Fatalf("SY2 status=%s, want completed via degraded adaptive driver (last=%s)", got.Status, got.LastError)
	}
	rp, phases := getPlan(t, svc, run.ID)
	if rp.Materialized != plan.MaterializedGrow {
		t.Fatalf("SY2 materialized=%s, want grow (degrade must not flip to upfront)", rp.Materialized)
	}
	if len(phases) == 0 {
		t.Fatalf("SY2 degraded run must grow ledger phases via round appends")
	}
	if !hasAudit(t, st, run.ID, "eng_synth_fail") {
		t.Fatalf("SY2 missing audit eng_synth_fail")
	}
	if hasAudit(t, st, run.ID, "eng_synth") {
		t.Fatalf("SY2 must not have eng_synth audit after degrade")
	}
	// 兜底自适应完整:runEngineering 以 grow 记账追满回合(len 非空即已追账;materialized 不翻 upfront)。
	if len(phases) == 0 {
		t.Fatalf("SY2 degraded run produced no ledger phases:\n%+v", phases)
	}
}

// ---- SY3:OS 机械 accept 允许清单(只读确定性;直接断言四分支) ----

func TestSY3OSMechanicalAllowlist(t *testing.T) {
	svc, _ := newSvc(t)
	ctx := context.Background()

	// 干净基线仓库:base commit → 基线存在(HEAD^..HEAD 检查可达)。
	ws := seedGitWorkspace(t)

	// 分支1:产出在 + 净残留=0 → ok(产出单独 commit 为 HEAD,空白干净)。
	if err := os.WriteFile(filepath.Join(ws, "fix.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, ws, "add", "fix.go")
	runGit(t, ws, "commit", "-q", "-m", "phase output")
	accOK := plan.RunPlanPhase{Note: "acceptance: artifact present\noutput: fix.go"}
	pass1, detail1 := svc.osMechanicalAccept(ctx, ws, accOK)
	if !pass1 {
		t.Fatalf("SY3 ok branch failed: %s", detail1)
	}
	if !strings.Contains(detail1, "residue=0") {
		t.Fatalf("SY3 ok detail = %q", detail1)
	}

	// 分支2:期望产出缺失 → 拒。
	accMissing := plan.RunPlanPhase{Note: "output: nope.go"}
	pass2, reason2 := svc.osMechanicalAccept(ctx, ws, accMissing)
	if pass2 || !strings.Contains(reason2, "expected output missing") {
		t.Fatalf("SY3 missing-output branch: pass=%v reason=%q", pass2, reason2)
	}

	// 分支3:越界意外残留(stray 未跟踪,越出声明 output)→ 拒。
	accStray := plan.RunPlanPhase{Note: "output: fix.go"}
	if err := os.WriteFile(filepath.Join(ws, "stray.txt"), []byte("residue\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	pass3, reason3 := svc.osMechanicalAccept(ctx, ws, accStray)
	if pass3 || !strings.Contains(reason3, "residue") {
		t.Fatalf("SY3 stray branch: pass=%v reason=%q", pass3, reason3)
	}
	if err := os.Remove(filepath.Join(ws, "stray.txt")); err != nil {
		t.Fatal(err)
	}

	// 分支4:相位提交含空白错误(trailing whitespace)→ 拒(git diff --check HEAD^..HEAD)。
	ws2 := seedGitWorkspace(t)
	if err := os.WriteFile(filepath.Join(ws2, "bad.go"), []byte("package main\nvar x = 1 \n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, ws2, "add", "bad.go")
	runGit(t, ws2, "commit", "-q", "-m", "whitespace commit")
	accWS := plan.RunPlanPhase{Note: "output: bad.go"}
	pass4, reason4 := svc.osMechanicalAccept(ctx, ws2, accWS)
	if pass4 || !strings.Contains(reason4, "diff --check") {
		t.Fatalf("SY3 whitespace branch: pass=%v reason=%q", pass4, reason4)
	}
}

// SY3b:驱动级 stray 残留 → accept 拒 → 返工 1 仍残留 → 相位 fail → 任务 engFail。
func TestSY3bDriveLevelStrayResidueFails(t *testing.T) {
	t.Setenv("OS_ENGINE_MODE", "scripted")
	t.Setenv("OS_SCRIPT_SYNTH", synthPlan3)
	t.Setenv("OS_SCRIPT_SYNTH_STRAY", "stray.txt") // 每次 do 落盘一个越界未跟踪文件
	svc, st, prj := scriptedHarness(t, "syc")
	ctx := context.Background()
	pl := mustSynthPipeline(t, svc, prj.ID, "dirty", pipeline.KindBugfix, pipeline.RiskLow)

	run, err := svc.RunPipeline(ctx, pl.ID, "leave residue")
	if err != nil {
		t.Fatalf("SY3b run: %v", err)
	}
	if err := svc.ExecuteTask(ctx, "w1", run.ID); err != nil {
		t.Fatalf("SY3b claim: %v", err)
	}
	got, _ := svc.GetTask(ctx, run.ID)
	if got.Status != "failed" {
		rpD, _ := getPlan(t, svc, run.ID)
		var acts []string
		for _, a := range listAudits(t, st, "task") {
			if a.EntityID == run.ID {
				acts = append(acts, a.Action)
			}
		}
		t.Fatalf("SY3b status=%s, want failed (residue after rework); materialized=%s audits=%v result=%q\n phases:\n%+v",
			got.Status, rpD.Materialized, acts, got.Result, rpD)
	}
	if !strings.Contains(got.LastError, "acceptance failed") {
		t.Fatalf("SY3b last_error=%q, want acceptance-failed after reworks", got.LastError)
	}
	for _, seq := range []int64{1, 2} {
		ph := phaseAt(t, svc, run.ID, seq)
		if ph.Status != plan.PhaseStatusFail {
			t.Fatalf("SY3b phase seq=%d status=%s, want fail", seq, ph.Status)
		}
	}
	if _, err := os.Stat(filepath.Join(prj.RootPath, "fix.go")); err != nil {
		t.Fatalf("SY3b do committed its declared output but task still failed on residue (fix.go missing: %v)", err)
	}
	// stray 属越界残留,任务终败后留在磁盘(OS 不扫不删,留人复核)。
	if _, err := os.Stat(filepath.Join(prj.RootPath, "stray.txt")); err != nil {
		t.Fatalf("SY3b stray file must remain for human review: %v", err)
	}
	_ = st
}

// ---- SY4:judge accept 返工上限(fail-once ≤1 放行;恒 fail → engFail) ----

func TestSY4JudgeAcceptReworkCap(t *testing.T) {
	t.Setenv("OS_ENGINE_MODE", "scripted")
	t.Setenv("OS_SCRIPT_SYNTH", synthPlanJudge)
	ctx := context.Background()

	// 子场景 A:fail-once → 首判不过,返工 1(≤ synthMaxRework)后放行 → completed。
	t.Run("fail-once-pass-after-rework", func(t *testing.T) {
		t.Setenv("OS_SCRIPT_ACCEPT", "fail-once")
		svc, st, prj := scriptedHarness(t, "syd")
		pl := mustSynthPipeline(t, svc, prj.ID, "retry", pipeline.KindDevelop, pipeline.RiskLow)
		run, err := svc.RunPipeline(ctx, pl.ID, "make retry backoff")
		if err != nil {
			t.Fatal(err)
		}
		if err := svc.ExecuteTask(ctx, "w1", run.ID); err != nil {
			t.Fatalf("SY4-A claim: %v", err)
		}
		got, _ := svc.GetTask(ctx, run.ID)
		if got.Status != "completed" {
			t.Fatalf("SY4-A status=%s, want completed (≤1 rework frees); last=%s", got.Status, got.LastError)
		}
		if !hasAudit(t, st, run.ID, "eng_complete") {
			t.Fatalf("SY4-A missing eng_complete")
		}
	})

	// 子场景 B:恒 fail → 返工用尽 → 相位终败 → 任务 engFail。
	t.Run("permanent-fail-engfail", func(t *testing.T) {
		t.Setenv("OS_SCRIPT_ACCEPT", "fail")
		svc, _, prj := scriptedHarness(t, "sye")
		pl := mustSynthPipeline(t, svc, prj.ID, "retry2", pipeline.KindDevelop, pipeline.RiskLow)
		run, err := svc.RunPipeline(ctx, pl.ID, "make retry backoff")
		if err != nil {
			t.Fatal(err)
		}
		if err := svc.ExecuteTask(ctx, "w1", run.ID); err != nil {
			t.Fatalf("SY4-B claim: %v", err)
		}
		got, _ := svc.GetTask(ctx, run.ID)
		if got.Status != "failed" {
			t.Fatalf("SY4-B status=%s, want failed after rework cap", got.Status)
		}
		if !strings.Contains(got.LastError, "acceptance failed") {
			t.Fatalf("SY4-B last_error=%q, want acceptance-failed", got.LastError)
		}
		for _, seq := range []int64{1, 2} {
			ph := phaseAt(t, svc, run.ID, seq)
			if ph.Status != plan.PhaseStatusFail {
				t.Fatalf("SY4-B phase seq=%d status=%s, want fail", seq, ph.Status)
			}
		}
	})
}

// ---- SY5:默认 adaptive 零漂移 + ops_patrol 模板优先 ----

func TestSY5DefaultAdaptiveZeroDriftAndPatrolTemplateWins(t *testing.T) {
	t.Setenv("OS_ENGINE_MODE", "scripted")
	t.Setenv("OS_SCRIPT_TEST", "pass")
	t.Setenv("OS_SCRIPT_REVIEW", "approve")
	svc, st, prj := scriptedHarness(t, "syf")
	ctx := context.Background()

	// bugfix 无 policy → 默认 adaptive 落库零漂移 + run 走 runEngineering(grow),零 eng_synth。
	pl := mustCreatePipeline(t, svc, prj.ID, "fix", pipeline.KindBugfix, pipeline.RiskLow)
	if pl.PlanPolicy != pipeline.PlanPolicyAdaptive {
		t.Fatalf("SY5 default plan_policy=%q, want adaptive", pl.PlanPolicy)
	}
	runB, err := svc.RunPipeline(ctx, pl.ID, "fix something")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.ExecuteTask(ctx, "w1", runB.ID); err != nil {
		t.Fatalf("SY5 bugfix claim: %v", err)
	}
	gotB, _ := svc.GetTask(ctx, runB.ID)
	if gotB.Status != "completed" {
		t.Fatalf("SY5 bugfix status=%s, want completed via engineering driver", gotB.Status)
	}
	rpB, _ := getPlan(t, svc, runB.ID)
	if rpB.Materialized != plan.MaterializedGrow {
		t.Fatalf("SY5 bugfix materialized=%s, want grow", rpB.Materialized)
	}
	if hasAudit(t, st, runB.ID, "eng_synth") || hasAudit(t, st, runB.ID, "eng_synth_fail") {
		t.Fatalf("SY5 adaptive run must not touch synthesis driver")
	}

	// ops_patrol 即便建 synthesize policy → runPatrol(模板优先;policy 存而不用)。
	patrolPL := mustSynthPipeline(t, svc, prj.ID, "patrol-synth", pipeline.KindOpsPatrol, pipeline.RiskMedium)
	if patrolPL.PlanPolicy != pipeline.PlanPolicySynthesize {
		t.Fatalf("SY5 ops_patrol policy persisted = %q", patrolPL.PlanPolicy)
	}
	runP, err := svc.RunPipeline(ctx, patrolPL.ID, "check git hygiene")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.ExecuteTask(ctx, "w1", runP.ID); err != nil {
		t.Fatalf("SY5 patrol claim: %v", err)
	}
	gotP, _ := svc.GetTask(ctx, runP.ID)
	if gotP.Status != "completed" {
		t.Fatalf("SY5 patrol status=%s, want completed via patrol driver", gotP.Status)
	}
	if !strings.HasPrefix(gotP.Result, PatrolResultTag) {
		t.Fatalf("SY5 ops_patrol with synth policy must still produce patrol artifact:\n%s", gotP.Result)
	}
}

// ---- SY6:合成计划校验(非法 JSON → 降级,不落半成品 upfront) ----

func TestSY6PlanValidationDegrades(t *testing.T) {
	invalid := map[string]string{
		"no-accept":     `{"summary":"x","phases":[{"kind":"do","title":"only do","allocator":"delegate","acceptance":"","output":""}]}`,
		"accept-no-out": `{"summary":"x","phases":[{"kind":"do","title":"d","allocator":"delegate","acceptance":"","output":""},{"kind":"accept","title":"a","allocator":"os","acceptance":"c","output":""}]}`,
		"too-many":      tooManyPhases(),
		"bad-kind":      `{"summary":"x","phases":[{"kind":"accept","title":"a","allocator":"os","acceptance":"c","output":"x"},{"kind":"do","title":"d","allocator":"delegate","acceptance":"","output":""}]}`,
		"unknown-kind":  `{"summary":"x","phases":[{"kind":"build","title":"b","allocator":"delegate","acceptance":"","output":""}]}`,
		"unparseable":   `not json at all`,
	}
	for name, synthJSON := range invalid {
		t.Run(name, func(t *testing.T) {
			t.Setenv("OS_ENGINE_MODE", "scripted")
			t.Setenv("OS_SCRIPT_SYNTH", synthJSON)
			t.Setenv("OS_SCRIPT_TEST", "pass")
			t.Setenv("OS_SCRIPT_REVIEW", "approve")
			svc, st, prj := scriptedHarness(t, "syg")
			ctx := context.Background()
			pl := mustSynthPipeline(t, svc, prj.ID, "v-"+name, pipeline.KindDevelop, pipeline.RiskLow)
			run, err := svc.RunPipeline(ctx, pl.ID, "some intent")
			if err != nil {
				t.Fatal(err)
			}
			if err := svc.ExecuteTask(ctx, "w1", run.ID); err != nil {
				t.Fatalf("claim: %v", err)
			}
			got, _ := svc.GetTask(ctx, run.ID)
			if got.Status != "completed" {
				t.Fatalf("degraded run must complete via adaptive driver: %s (last=%s)", got.Status, got.LastError)
			}
			rp, _ := getPlan(t, svc, run.ID)
			if rp.Materialized != plan.MaterializedGrow {
				t.Fatalf("materialized=%s, want grow (invalid plan must not flip upfront)", rp.Materialized)
			}
			if !hasAudit(t, st, run.ID, "eng_synth_fail") {
				t.Fatalf("missing audit eng_synth_fail for %s", name)
			}
			if hasAudit(t, st, run.ID, "eng_synth") {
				t.Fatalf("invalid plan must not audit eng_synth (%s)", name)
			}
		})
	}
}

// tooManyPhases 造超上限(synthPhaseCap+1)相位计划。
func tooManyPhases() string {
	var b strings.Builder
	b.WriteString(`{"summary":"huge","phases":[`)
	for i := 0; i < 14; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(`{"kind":"do","title":"d`)
		b.WriteString(strings.Repeat("x", 1))
		b.WriteString(`","allocator":"delegate","acceptance":"","output":""}`)
	}
	b.WriteString(`]}`)
	return b.String()
}
