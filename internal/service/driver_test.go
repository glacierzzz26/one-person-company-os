package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---- 用例 5:scripted 回归闭环(writer/test/review 全走 engScripted;委派/网关零命中) ----

func TestScriptedRoundLoopRegression(t *testing.T) {
	t.Setenv("OS_ENGINE_MODE", "scripted")
	t.Setenv("OS_SCRIPT_TEST", "pass")
	t.Setenv("OS_SCRIPT_REVIEW", "approve")

	svc, st := newSvc(t)
	ctx := context.Background()
	compID := seedCompanyID(t, st)

	// workspace 用非 git 目录:证明 scripted 下 writer 不落委派、前置不校验 git。
	fake := &writingDelegator{writeRel: "nope.txt"}
	svc.delegator = fake

	tk := seedEngineChildTask(t, svc, compID, t.TempDir(), nil)
	if err := svc.ExecuteTask(ctx, "w1", tk.ID); err != nil {
		t.Fatalf("ExecuteTask: %v", err)
	}
	got, err := st.GetTask(ctx, tk.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.Status != "completed" {
		t.Fatalf("status = %s, want completed (last_error=%s)", got.Status, got.LastError)
	}
	if !strings.Contains(got.Result, "fix (writer deterministic, retry=0)") {
		t.Fatalf("result should be the deterministic scripted diff, got:\n%s", got.Result)
	}
	if fake.count() != 0 {
		t.Fatalf("scripted must not invoke the delegator (calls=%d)", fake.count())
	}
}

// ---- 用例 6:writer 前置失败(非 git / 认领起点脏且无本任务委派 → engFail→failed) ----

func TestDelegateBaselineFailsViaExecute(t *testing.T) {
	cases := []struct {
		name    string
		setup   func(t *testing.T) string // 返回任务 workspace
		errPart string
	}{
		{
			name:    "workspace not a git repo",
			setup:   func(t *testing.T) string { return t.TempDir() }, // 空目录,非 git
			errPart: "git workspace",
		},
		{
			name: "dirty workspace with no prior delegation",
			setup: func(t *testing.T) string {
				ws := seedGitWorkspace(t)
				if err := os.WriteFile(filepath.Join(ws, "intruder.txt"), []byte("pre-existing\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				return ws
			},
			errPart: "has not delegated before",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("OS_ENGINE_MODE", "live")
			svc, st := newSvc(t)
			ctx := context.Background()
			compID := seedCompanyID(t, st)
			ws := tc.setup(t)
			// 8.4:判读档端点补全(writer 前置失败用例,失败点在委派门,到不了判读网关;端点行仅为建单默认解析)。
			seedJudgeDefaults(t, st, compID)
			tk := seedEngineChildTask(t, svc, compID, ws, nil)
			if err := svc.ExecuteTask(ctx, "w1", tk.ID); err != nil {
				t.Fatalf("ExecuteTask: %v", err)
			}
			got, err := st.GetTask(ctx, tk.ID)
			if err != nil {
				t.Fatalf("GetTask: %v", err)
			}
			if got.Status != "failed" {
				t.Fatalf("status = %s, want failed", got.Status)
			}
			if !strings.Contains(got.LastError, tc.errPart) {
				t.Fatalf("last_error = %q, want to contain %q", got.LastError, tc.errPart)
			}
		})
	}
}

// 对照:干净 git workspace + 委派前置通过 → 走到委派闭环(证明上面失败确实由前置触发)。
func TestDelegateBaselineCleanPassesToWriter(t *testing.T) {
	t.Setenv("OS_ENGINE_MODE", "live")
	svc, st := newSvc(t)
	ctx := context.Background()
	compID := seedCompanyID(t, st)
	ws := seedGitWorkspace(t)
	fake := &writingDelegator{writeRel: "fix.txt"}
	svc.delegator = fake
	// 8.4:判读档端点补全(只证 clean 前置放行委派,writer 委派后即止,不触网判读)。
	seedJudgeDefaults(t, st, compID)
	tk := seedEngineChildTask(t, svc, compID, ws, nil)

	// 走到 writer 即因缺 test/review 端点 fail —— 这证明已越过 writer 委派前置。
	// (完整一轮在 TestDelegateWriterRoundLoopGateway 已覆盖;这里只证明 clean 前置放行委派)
	if err := svc.delegateBaseline(ctx, tk); err != nil {
		t.Fatalf("clean workspace should pass baseline, got %v", err)
	}
	// 直接触发一次 writer 委派(不经 test/review),断言 diff 落回、委派计一次。
	if _, err := svc.delegateWriter(ctx, tk, engCallCtx{role: engRoleWriter, round: 0}); err != nil {
		t.Fatalf("delegateWriter: %v", err)
	}
	if fake.count() != 1 {
		t.Fatalf("delegator calls = %d, want 1", fake.count())
	}
	got, err := st.GetTask(ctx, tk.ID)
	if err != nil {
		t.Fatal(err)
	}
	_ = got // delegateWriter 不改 task;diff 在返回里
}
