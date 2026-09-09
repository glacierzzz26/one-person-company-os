package service

// Phase 10.6 契约用例(契约 docs/phase10/design/os-mechanical-verify.md §五,V*;scripted 公司 = env seam
// (TestMain 开)+ OS_ENGINE_MODE=scripted,产品零 env;真 git 项目根 + 注入 fake runner,离线确定):
//   - V1 verify PASS 主链:合成 3 相位(do → accept(os, verify: go-test)→ dispose manual),项目根带已提交
//     go.mod;fake runner exit 0 → completed;acc evidence 含 `verify go-test: PASS`;runner 恰调 1 次
//     (name=go argv=[test ./...] dir=ws);逐相位 ok + eng_complete 审计
//   - V2 verify FAIL → 返工 ≤1 → 仍 FAIL → 相位 fail → 任务 failed 不复活(fake 恒非零);acc evidence
//     首段含 `verify go-test: FAIL` + 输出首行;runner 调 2 次;do/acc fail 落账本
//   - V3 verify FAIL → 返工 → PASS(fake fail-once)→ completed;runner 调 2 次;返工放行语义
//   - V4 词表外 verify(python-pytest)→ validate 错 → eng_synth_fail → 降级 runEngineering 完成;runner 不触;
//     materialized 保持 grow;无 eng_synth
//   - V5 verify 位置约束(judge-accept / do / dispose 上出现)→ 计划无效降级(同 V4 断言)
//   - V6a verify untracked 残留回收:fake 落一个 untracked 临时文件 → PASS 后该文件被回收、evidence
//     `residue cleaned=1`、工作树归净(直接调 runMechanicalVerify)
//   - V6b verify 改动 tracked → fail `dirtied tracked file`(直接调,不自动回滚)
//   - V7 无 go.mod 门:项目根无 go.mod + verify go-test → 返工用尽 → 任务 failed;acc evidence 含
//     `no go.mod`;runner 不调用(调 0 次)

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/glacierzzz26/one-person-company-os/internal/pipeline"
	"github.com/glacierzzz26/one-person-company-os/internal/plan"
)

// synthPlanVerifyGoTest 合法 3 相位计划(os accept 显式 verify: go-test):do + accept(os, output fix.go
// + verify go-test)+ dispose(manual)。
const synthPlanVerifyGoTest = `{"summary":"fix and verify go test","phases":[
 {"kind":"do","title":"implement login fix","allocator":"delegate","acceptance":"apply the fix","output":""},
 {"kind":"accept","title":"os verify go test","allocator":"os","acceptance":"fix.go exists and go test passes","output":"fix.go","verify":"go-test"},
 {"kind":"dispose","title":"note for human","allocator":"manual","acceptance":"","output":""}
]}`

// verifyRunnerRecorder 注入 s.runner 的 fake:记录调用(dir + name+argv),按次喂 canned 结果。
type verifyRunnerRecorder struct {
	mu    sync.Mutex
	calls int
	dirs  []string
	argv  []string // 每次调用 "name arg1 arg2"
	// run 为 nil → 每次返回 exit 0 空输出(PASS 默认)。
	run func(call int, dir string, args []string) (string, error)
}

func (r *verifyRunnerRecorder) Run(_ context.Context, dir, name string, args ...string) (string, error) {
	r.mu.Lock()
	r.calls++
	r.dirs = append(r.dirs, dir)
	r.argv = append(r.argv, strings.Join(append([]string{name}, args...), " "))
	r.mu.Unlock()
	if r.run == nil {
		return "", nil
	}
	return r.run(r.calls-1, dir, args)
}

func (r *verifyRunnerRecorder) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

// seedGoModule 往 git 项目根写并提交 go.mod(首个提交;verify go.mod 门 + 净残留都要求它已提交)。
func seedGoModule(t *testing.T, ws string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(ws, "go.mod"), []byte("module opostest\n\ngo 1.21\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	runGit(t, ws, "add", "go.mod")
	runGit(t, ws, "commit", "-q", "-m", "seed go module")
}

// mustCompleteSynthRun claim 一次并断言 completed + result 前缀。
func mustCompleteSynthRun(t *testing.T, svc *Service, runID, prefix string) {
	t.Helper()
	if err := svc.ExecuteTask(context.Background(), "w1", runID); err != nil {
		t.Fatalf("claim: %v", err)
	}
	got, _ := svc.GetTask(context.Background(), runID)
	if got.Status != "completed" {
		t.Fatalf("status=%s, want completed (last=%s)", got.Status, got.LastError)
	}
	if !strings.HasPrefix(got.Result, prefix) {
		t.Fatalf("result=%q, want prefix %q", got.Result, prefix)
	}
}

// ---- V1:verify go-test PASS 主链 ----

func TestV1VerifyGoTestPassMainline(t *testing.T) {
	t.Setenv("OS_ENGINE_MODE", "scripted")
	t.Setenv("OS_SCRIPT_SYNTH", synthPlanVerifyGoTest)
	svc, st, prj := scriptedHarness(t, "v1a")
	seedGoModule(t, prj.RootPath)
	rr := &verifyRunnerRecorder{}
	svc.runner = rr
	ctx := context.Background()
	pl := mustSynthPipeline(t, svc, prj.ID, "vfix", pipeline.KindBugfix, pipeline.RiskLow)
	run, err := svc.RunPipeline(ctx, pl.ID, "fix flaky + verify go test")
	if err != nil {
		t.Fatalf("V1 run: %v", err)
	}
	mustCompleteSynthRun(t, svc, run.ID, "synth: 3 phases executed")

	acc := phaseAt(t, svc, run.ID, 2)
	if !strings.Contains(acc.Evidence, "verify go-test: PASS") {
		t.Fatalf("V1 acc evidence=%q, want verify go-test: PASS", acc.Evidence)
	}
	for _, seq := range []int64{1, 2, 3} {
		ph := phaseAt(t, svc, run.ID, seq)
		if ph.Status != plan.PhaseStatusOK {
			t.Fatalf("V1 phase seq=%d status=%s, want ok", seq, ph.Status)
		}
	}
	if n := rr.callCount(); n != 1 {
		t.Fatalf("V1 runner calls=%d, want 1", n)
	}
	if rr.dirs[0] != prj.RootPath {
		t.Fatalf("V1 runner dir=%q, want ws %q", rr.dirs[0], prj.RootPath)
	}
	if rr.argv[0] != "go test ./..." {
		t.Fatalf("V1 runner argv=%q, want 'go test ./...'", rr.argv[0])
	}
	if !hasAudit(t, st, run.ID, "eng_complete") {
		t.Fatalf("V1 missing eng_complete")
	}
}

// ---- V2:verify FAIL → 返工 → 仍 FAIL → 不复活 ----

func TestV2VerifyFailPermanentEngfails(t *testing.T) {
	t.Setenv("OS_ENGINE_MODE", "scripted")
	t.Setenv("OS_SCRIPT_SYNTH", synthPlanVerifyGoTest)
	svc, _, prj := scriptedHarness(t, "v2a")
	seedGoModule(t, prj.RootPath)
	rr := &verifyRunnerRecorder{run: func(call int, _ string, _ []string) (string, error) {
		return "--- FAIL: TestFlaky (0.0s)\n\tx_test.go:12: flaky\nFAIL\nFAIL\tm/p\t0.1s", errors.New("exit status 1")
	}}
	svc.runner = rr
	ctx := context.Background()
	pl := mustSynthPipeline(t, svc, prj.ID, "vfail", pipeline.KindBugfix, pipeline.RiskLow)
	run, err := svc.RunPipeline(ctx, pl.ID, "flaky forever")
	if err != nil {
		t.Fatalf("V2 run: %v", err)
	}
	if err := svc.ExecuteTask(ctx, "w1", run.ID); err != nil {
		t.Fatalf("V2 claim: %v", err)
	}
	got, _ := svc.GetTask(ctx, run.ID)
	if got.Status != "failed" {
		t.Fatalf("V2 status=%s, want failed after rework cap (last=%s)", got.Status, got.LastError)
	}
	if !strings.Contains(got.LastError, "acceptance failed") {
		t.Fatalf("V2 last_error=%q, want acceptance-failed", got.LastError)
	}
	if n := rr.callCount(); n != 2 {
		t.Fatalf("V2 runner calls=%d, want 2 (verify + 1 rework)", n)
	}
	for _, seq := range []int64{1, 2} {
		ph := phaseAt(t, svc, run.ID, seq)
		if ph.Status != plan.PhaseStatusFail {
			t.Fatalf("V2 phase seq=%d status=%s, want fail", seq, ph.Status)
		}
	}
	acc := phaseAt(t, svc, run.ID, 2)
	if !strings.Contains(acc.Evidence, "verify go-test: FAIL") || !strings.Contains(acc.Evidence, "TestFlaky") {
		t.Fatalf("V2 acc evidence=%q, want verify FAIL + first output line", acc.Evidence)
	}
}

// ---- V3:verify FAIL → 返工 → PASS(fail-once 放行) ----

func TestV3VerifyFailThenReworkPass(t *testing.T) {
	t.Setenv("OS_ENGINE_MODE", "scripted")
	t.Setenv("OS_SCRIPT_SYNTH", synthPlanVerifyGoTest)
	svc, _, prj := scriptedHarness(t, "v3a")
	seedGoModule(t, prj.RootPath)
	rr := &verifyRunnerRecorder{run: func(call int, _ string, _ []string) (string, error) {
		if call == 0 {
			return "--- FAIL: TestFlaky (0.0s)\n\tx_test.go:12: flaky\nFAIL", errors.New("exit status 1")
		}
		return "ok m/p 1.2s", nil
	}}
	svc.runner = rr
	ctx := context.Background()
	pl := mustSynthPipeline(t, svc, prj.ID, "vretry", pipeline.KindBugfix, pipeline.RiskLow)
	run, err := svc.RunPipeline(ctx, pl.ID, "flaky once then fine")
	if err != nil {
		t.Fatalf("V3 run: %v", err)
	}
	mustCompleteSynthRun(t, svc, run.ID, "synth: 3 phases executed")
	if n := rr.callCount(); n != 2 {
		t.Fatalf("V3 runner calls=%d, want 2 (verify + 1 rework)", n)
	}
	acc := phaseAt(t, svc, run.ID, 2)
	if !strings.Contains(acc.Evidence, "verify go-test: PASS") {
		t.Fatalf("V3 acc evidence=%q, want PASS after rework", acc.Evidence)
	}
}

// ---- V4:词表外 verify → 计划无效降级 ----

func TestV4VerifyOutOfVocabularyDegrades(t *testing.T) {
	badPlan := strings.Replace(synthPlanVerifyGoTest, `"verify":"go-test"`, `"verify":"python-pytest"`, 1)
	t.Setenv("OS_ENGINE_MODE", "scripted")
	t.Setenv("OS_SCRIPT_SYNTH", badPlan)
	t.Setenv("OS_SCRIPT_TEST", "pass")
	t.Setenv("OS_SCRIPT_REVIEW", "approve")
	svc, st, prj := scriptedHarness(t, "v4a")
	rr := &verifyRunnerRecorder{}
	svc.runner = rr
	ctx := context.Background()
	pl := mustSynthPipeline(t, svc, prj.ID, "badverify", pipeline.KindDevelop, pipeline.RiskLow)
	run, err := svc.RunPipeline(ctx, pl.ID, "some intent")
	if err != nil {
		t.Fatalf("V4 run: %v", err)
	}
	if err := svc.ExecuteTask(ctx, "w1", run.ID); err != nil {
		t.Fatalf("V4 claim: %v", err)
	}
	got, _ := svc.GetTask(ctx, run.ID)
	if got.Status != "completed" {
		t.Fatalf("V4 status=%s, want completed via degraded adaptive driver (last=%s)", got.Status, got.LastError)
	}
	rp, _ := getPlan(t, svc, run.ID)
	if rp.Materialized != plan.MaterializedGrow {
		t.Fatalf("V4 materialized=%s, want grow (invalid verify must not flip upfront)", rp.Materialized)
	}
	if !hasAudit(t, st, run.ID, "eng_synth_fail") {
		t.Fatalf("V4 missing audit eng_synth_fail")
	}
	if hasAudit(t, st, run.ID, "eng_synth") {
		t.Fatalf("V4 invalid plan must not audit eng_synth")
	}
	if n := rr.callCount(); n != 0 {
		t.Fatalf("V4 runner must never run (calls=%d)", n)
	}
}

// ---- V5:verify 位置约束(judge-accept / do / dispose)→ 降级 ----

func TestV5VerifyPositionConstraintsDegrade(t *testing.T) {
	plans := map[string]string{
		"judge-accept": `{"summary":"x","phases":[` +
			`{"kind":"do","title":"d","allocator":"delegate","acceptance":"","output":""},` +
			`{"kind":"accept","title":"a","allocator":"judge","acceptance":"c","output":"","verify":"go-test"}]}`,
		"do": `{"summary":"x","phases":[` +
			`{"kind":"do","title":"d","allocator":"delegate","acceptance":"","output":"","verify":"go-build"},` +
			`{"kind":"accept","title":"a","allocator":"os","acceptance":"c","output":"fix.go"}]}`,
		"dispose": `{"summary":"x","phases":[` +
			`{"kind":"do","title":"d","allocator":"delegate","acceptance":"","output":""},` +
			`{"kind":"accept","title":"a","allocator":"os","acceptance":"c","output":"fix.go"},` +
			`{"kind":"dispose","title":"dp","allocator":"manual","acceptance":"","output":"","verify":"go-build"}]}`,
	}
	for name, synthJSON := range plans {
		t.Run(name, func(t *testing.T) {
			t.Setenv("OS_ENGINE_MODE", "scripted")
			t.Setenv("OS_SCRIPT_SYNTH", synthJSON)
			t.Setenv("OS_SCRIPT_TEST", "pass")
			t.Setenv("OS_SCRIPT_REVIEW", "approve")
			svc, st, prj := scriptedHarness(t, "v5-"+name)
			rr := &verifyRunnerRecorder{}
			svc.runner = rr
			ctx := context.Background()
			pl := mustSynthPipeline(t, svc, prj.ID, "pos-"+name, pipeline.KindDevelop, pipeline.RiskLow)
			run, err := svc.RunPipeline(ctx, pl.ID, "some intent")
			if err != nil {
				t.Fatal(err)
			}
			if err := svc.ExecuteTask(ctx, "w1", run.ID); err != nil {
				t.Fatalf("claim: %v", err)
			}
			got, _ := svc.GetTask(ctx, run.ID)
			if got.Status != "completed" {
				t.Fatalf("status=%s, want completed via degrade (last=%s)", got.Status, got.LastError)
			}
			rp, _ := getPlan(t, svc, run.ID)
			if rp.Materialized != plan.MaterializedGrow {
				t.Fatalf("materialized=%s, want grow", rp.Materialized)
			}
			if !hasAudit(t, st, run.ID, "eng_synth_fail") || hasAudit(t, st, run.ID, "eng_synth") {
				t.Fatalf("audit mismatch for %s (want eng_synth_fail, no eng_synth)", name)
			}
			if n := rr.callCount(); n != 0 {
				t.Fatalf("runner must never run (calls=%d)", n)
			}
		})
	}
}

// ---- V6a:verify untracked 残留回收(直接调 runMechanicalVerify) ----

func TestV6aVerifyCleansUntrackedResidue(t *testing.T) {
	svc, _ := newSvc(t)
	ws := seedGitWorkspace(t) // base commit + base.txt
	seedGoModule(t, ws)
	rr := &verifyRunnerRecorder{run: func(call int, dir string, _ []string) (string, error) {
		if err := os.WriteFile(filepath.Join(dir, "coverage.out"), []byte("mode: set\n"), 0o644); err != nil {
			return "", err
		}
		return "ok m/p 0.3s", nil
	}}
	svc.runner = rr
	ctx := context.Background()
	pass, detail := svc.runMechanicalVerify(ctx, ws, "go-test")
	if !pass {
		t.Fatalf("V6a verify pass=false: %s", detail)
	}
	if !strings.Contains(detail, "residue cleaned=1") {
		t.Fatalf("V6a detail=%q, want residue cleaned=1", detail)
	}
	if _, err := os.Stat(filepath.Join(ws, "coverage.out")); !os.IsNotExist(err) {
		t.Fatalf("V6a coverage.out must be cleaned up (err=%v)", err)
	}
	if !wsPorcelainClean(ctx, ws) {
		t.Fatalf("V6a workspace must be porcelain clean after verify cleanup")
	}
}

// ---- V6b:verify 改动 tracked → fail(不自动回滚) ----

func TestV6bVerifyDirtiedTrackedFileFails(t *testing.T) {
	svc, _ := newSvc(t)
	ws := seedGitWorkspace(t)
	seedGoModule(t, ws)
	rr := &verifyRunnerRecorder{run: func(call int, dir string, _ []string) (string, error) {
		// 改动已提交的 go.mod(tracked)→ 对账须判 dirtied fail,不回收、不自动回滚。
		f, err := os.OpenFile(filepath.Join(dir, "go.mod"), os.O_APPEND|os.O_WRONLY, 0)
		if err != nil {
			return "", err
		}
		defer f.Close()
		_, werr := f.WriteString("\n// verify tamper\n")
		return "ok", werr
	}}
	svc.runner = rr
	ctx := context.Background()
	pass, detail := svc.runMechanicalVerify(ctx, ws, "go-test")
	if pass {
		t.Fatalf("V6b verify must fail on dirtied tracked file (detail=%q)", detail)
	}
	if !strings.Contains(detail, "dirtied tracked file") || !strings.Contains(detail, "go.mod") {
		t.Fatalf("V6b detail=%q, want dirtied tracked file go.mod", detail)
	}
	// tracked 改动不自动回滚(留作人/委派处理)→ 工作树应保持 dirty(verify 不修烂摊子)。
	if wsPorcelainClean(ctx, ws) {
		t.Fatalf("V6b verify must NOT auto-revert the dirtied tracked file (workspace unexpectedly clean)")
	}
}

// ---- V7:无 go.mod 门(accept fail;runner 不调用)→ 任务 failed ----

func TestV7VerifyWithoutGoModFailsRunnerNotCalled(t *testing.T) {
	t.Setenv("OS_ENGINE_MODE", "scripted")
	t.Setenv("OS_SCRIPT_SYNTH", synthPlanVerifyGoTest)
	svc, st, prj := scriptedHarness(t, "v7a") // 项目根 git init,但无 go.mod(不 seed)
	rr := &verifyRunnerRecorder{}
	svc.runner = rr
	ctx := context.Background()
	pl := mustSynthPipeline(t, svc, prj.ID, "nogomod", pipeline.KindBugfix, pipeline.RiskLow)
	run, err := svc.RunPipeline(ctx, pl.ID, "verify without module")
	if err != nil {
		t.Fatalf("V7 run: %v", err)
	}
	if err := svc.ExecuteTask(ctx, "w1", run.ID); err != nil {
		t.Fatalf("V7 claim: %v", err)
	}
	got, _ := svc.GetTask(ctx, run.ID)
	if got.Status != "failed" {
		t.Fatalf("V7 status=%s, want failed (no go.mod; accept fails after rework) last=%s", got.Status, got.LastError)
	}
	if n := rr.callCount(); n != 0 {
		t.Fatalf("V7 runner must not be called without go.mod (calls=%d)", n)
	}
	acc := phaseAt(t, svc, run.ID, 2)
	if acc.Status != plan.PhaseStatusFail || !strings.Contains(acc.Evidence, "no go.mod") {
		t.Fatalf("V7 acc status=%s evidence=%q, want fail with no go.mod", acc.Status, acc.Evidence)
	}
	if hasAudit(t, st, run.ID, "eng_synth_fail") {
		// 计划本身合法(go-test 在词表内)→ 不应降级;失败来自验收,不进 eng_synth_fail。
		t.Fatalf("V7 must not have eng_synth_fail (plan is valid; verify gate failed)")
	}
}
