package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/glacierzzz26/one-person-company-os/internal/company"
	"github.com/glacierzzz26/one-person-company-os/internal/endpoint"
	"github.com/glacierzzz26/one-person-company-os/internal/storage"
	"github.com/glacierzzz26/one-person-company-os/internal/storage/repository"
	"github.com/glacierzzz26/one-person-company-os/internal/task"
	"github.com/google/uuid"
)

// ---- harness ---- (Phase 8.2 用例:storage.Open → NewStore → service.New,同 server 包)

func newSvc(t *testing.T) (*Service, *repository.Store) {
	t.Helper()
	db, err := storage.Open(filepath.Join(t.TempDir(), "os-test.db"))
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	st := repository.NewStore(db)
	return New(st), st
}

func seedCompanyID(t *testing.T, st *repository.Store) string {
	t.Helper()
	now := time.Now().Unix()
	c, err := st.CreateCompany(context.Background(), company.Company{
		ID: uuid.NewString(), Name: "Acme", Vision: "build things",
		CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("seed company: %v", err)
	}
	return c.ID
}

// ---- git 工具 ----

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	if _, err := gitDirCmd(context.Background(), dir, args...); err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
}

// seedGitWorkspace 建一个干净的 git 工作区(含 base.txt 基线提交)。
func seedGitWorkspace(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init", "-q", "-b", "main")
	runGit(t, dir, "config", "user.email", "test@opos.local")
	runGit(t, dir, "config", "user.name", "opos-test")
	if err := os.WriteFile(filepath.Join(dir, "base.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write base: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "gone.txt"), []byte("will be deleted\n"), 0o644); err != nil {
		t.Fatalf("write gone: %v", err)
	}
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-q", "-m", "seed")
	return dir
}

// seedEngineChildTask 建一条父 id 非空的 engineering 任务(planEngineering 对子任务直接跳过),
// 返回可 runEngineering/ExecuteTask 的 task。
func seedEngineChildTask(t *testing.T, svc *Service, compID, ws string, reviewerID *string) task.Task {
	t.Helper()
	parent := "parent-dummy-" + uuid.NewString()
	tk, err := svc.CreateTaskAs(context.Background(), TaskParams{
		CompanyID: compID, Title: "impl feature", Description: "make it work",
		ToolName: "engineering", Risk: "low", MaxAttempts: 1, TimeoutSec: 60,
		Workspace: ws, ParentTaskID: &parent, ReviewerEndpointID: reviewerID,
	}, "test")
	if err != nil {
		t.Fatalf("seed engineering task: %v", err)
	}
	return tk
}

func epRef(s string) *string { return &s }

// seedEndpoint 落一条端点(proto=openai 判读网关;带 Bearer token 加密落库)。
func seedEndpoint(t *testing.T, st *repository.Store, compID, baseURL string) endpoint.Endpoint {
	t.Helper()
	t.Setenv("OS_ENDPOINT_KEY", strings.Repeat("ab", 32))
	tok, err := endpoint.SealToken("sk-test-123")
	if err != nil {
		t.Fatalf("seal token: %v", err)
	}
	now := time.Now().Unix()
	ep, err := st.CreateEndpoint(context.Background(), endpoint.Endpoint{
		ID: uuid.NewString(), CompanyID: compID, Name: "judge-gw",
		BaseURL: baseURL, TokenEnc: tok, Proto: "openai", Vendor: "gateway",
		SelectedModel: "judge-model", Role: "pool", Status: "active",
		CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("seed endpoint: %v", err)
	}
	return ep
}

// ---- fakes ----

// writingDelegator 注入缝:每次委派向 spec.Workspace 落一个文件并返回 canned report;
// calls 记录全部 spec(可断言 brief/env/workspace)。
type writingDelegator struct {
	mu       sync.Mutex
	calls    []DelegateSpec
	writeRel string // 落盘文件;空 = 不写(空 diff 用例)
}

func (d *writingDelegator) Delegate(ctx context.Context, spec DelegateSpec) (DelegateResult, error) {
	d.mu.Lock()
	d.calls = append(d.calls, spec)
	i := len(d.calls)
	d.mu.Unlock()
	if d.writeRel != "" {
		if err := os.WriteFile(filepath.Join(spec.Workspace, d.writeRel),
			[]byte(fmt.Sprintf("fake fix attempt %d\n", i)), 0o644); err != nil {
			return DelegateResult{}, err
		}
	}
	return DelegateResult{Report: fmt.Sprintf("fake delegator report #%d; changed %s", i, d.writeRel)}, nil
}

func (d *writingDelegator) count() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.calls)
}

// gatewayRec 记录网关收到的请求;handler 按 body 内容回 test/review 判读文本。
type gatewayRec struct {
	srv   *httptest.Server
	mu    sync.Mutex
	calls []gatewayCall
	// respond 覆盖默认分类(长度截断等用例);返回 (content, finish_reason)。
	respond func(body string) (string, string)
}

type gatewayCall struct {
	path, method, auth, body string
}

func newJudgeGateway(t *testing.T) *gatewayRec {
	t.Helper()
	g := &gatewayRec{}
	g.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := readReqBody(r)
		g.mu.Lock()
		g.calls = append(g.calls, gatewayCall{
			path: r.URL.Path, method: r.Method, auth: r.Header.Get("Authorization"), body: body,
		})
		g.mu.Unlock()
		content, fr := "TEST OK", "stop"
		if g.respond != nil {
			content, fr = g.respond(body)
		} else if strings.Contains(body, "senior reviewer") {
			content, fr = "VERDICT: approve", "stop"
		} else if strings.Contains(body, "You are QA.") {
			content, fr = "TEST OK", "stop"
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"choices":[{"message":{"content":%q},"finish_reason":%q}]}`, content, fr)
	}))
	t.Cleanup(g.srv.Close)
	return g
}

func readReqBody(r *http.Request) string {
	b, _ := io.ReadAll(r.Body)
	return string(b)
}

func (g *gatewayRec) snapshot() []gatewayCall {
	g.mu.Lock()
	defer g.mu.Unlock()
	out := make([]gatewayCall, len(g.calls))
	copy(out, g.calls)
	return out
}

// ---- 用例 1:git 助手 ----

func TestGitHelpers(t *testing.T) {
	ctx := context.Background()
	dir := seedGitWorkspace(t)

	if !wsIsGit(ctx, dir) {
		t.Fatalf("wsIsGit(%s) = false, want true", dir)
	}
	if !wsPorcelainClean(ctx, dir) {
		t.Fatalf("fresh workspace should be clean")
	}
	if wsIsGit(ctx, t.TempDir()) {
		t.Fatalf("non-git dir should not be git")
	}

	// fake 写入 新增+修改+删除
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	added := filepath.Join(sub, "added.txt")
	modified := filepath.Join(dir, "base.txt")
	removed := filepath.Join(dir, "gone.txt")
	if err := os.WriteFile(added, []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(removed, []byte("to delete\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(modified, []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(removed); err != nil {
		t.Fatal(err)
	}

	if wsPorcelainClean(ctx, dir) {
		t.Fatalf("workspace with changes must not be clean")
	}
	diff, err := captureChanges(ctx, dir)
	if err != nil {
		t.Fatalf("captureChanges: %v", err)
	}
	for _, want := range []string{"added.txt", "base.txt", "gone.txt", "+changed", "+new"} {
		if !strings.Contains(diff, want) {
			t.Fatalf("captured diff missing %q:\n%s", want, diff)
		}
	}
	if !strings.Contains(diff, "deleted file mode") && !strings.Contains(diff, "-to delete") {
		t.Fatalf("captured diff should record deletion:\n%s", diff)
	}
}

// ---- 用例 2:writer 委派闭环(fake delegator + 假网关判读) ----

func TestDelegateWriterRoundLoopGateway(t *testing.T) {
	t.Setenv("OS_ENGINE_MODE", "live")
	t.Setenv("OS_SCRIPT_TEST", "")
	t.Setenv("OS_SCRIPT_REVIEW", "")

	svc, st := newSvc(t)
	ctx := context.Background()
	compID := seedCompanyID(t, st)
	ws := seedGitWorkspace(t)

	gw := newJudgeGateway(t)
	ep := seedEndpoint(t, st, compID, gw.srv.URL)

	fake := &writingDelegator{writeRel: "fix.txt"}
	svc.delegator = fake

	tk := seedEngineChildTask(t, svc, compID, ws, epRef(ep.ID))
	if err := svc.ExecuteTask(ctx, "w1", tk.ID); err != nil {
		t.Fatalf("ExecuteTask: %v", err)
	}

	// 任务完成,result = 真实 diff(含 fake 落盘文件)
	got, err := st.GetTask(ctx, tk.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.Status != "completed" {
		t.Fatalf("status = %s, want completed (last_error=%s)", got.Status, got.LastError)
	}
	if !strings.Contains(got.Result, "fix.txt") || !strings.Contains(got.Result, "fake fix attempt 1") {
		t.Fatalf("task result should be the real captured diff:\n%s", got.Result)
	}

	// writer execution 完成,结果 = diff
	execs, err := st.ListExecutions(ctx, tk.ID, "completed")
	if err != nil {
		t.Fatalf("ListExecutions: %v", err)
	}
	var writerExec bool
	for _, e := range execs {
		if strings.Contains(e.Result, "eng_writer") {
			writerExec = true
			if !strings.Contains(e.Result, "fix.txt") {
				t.Fatalf("writer execution result should embed diff:\n%s", e.Result)
			}
		}
	}
	if !writerExec {
		t.Fatalf("no completed writer execution found: %+v", execs)
	}

	// 委派只发生一次(live writer);audit eng_delegate 存在,actor 沿工程语义
	audits, err := st.ListAudits(ctx, "task")
	if err != nil {
		t.Fatalf("ListAudits: %v", err)
	}
	var foundDel bool
	for _, a := range audits {
		if a.EntityID == tk.ID && a.Action == "eng_delegate" {
			foundDel = true
			if a.Actor != "agent:none" {
				t.Fatalf("eng_delegate actor = %q, want agent:none", a.Actor)
			}
			if !strings.HasPrefix(a.Detail, "claude round=0 retry=0 ws=") {
				t.Fatalf("eng_delegate detail shape wrong: %s", a.Detail)
			}
		}
	}
	if !foundDel {
		t.Fatalf("no eng_delegate audit recorded")
	}

	// 判读命中网关:POST /v1/chat/completions + Bearer(test/review 至少各一次)
	calls := gw.snapshot()
	if len(calls) < 2 {
		t.Fatalf("judge gateway calls = %d, want >=2 (test+review)", len(calls))
	}
	for _, c := range calls {
		if c.method != http.MethodPost || c.path != "/v1/chat/completions" {
			t.Fatalf("judge call shape wrong: %s %s", c.method, c.path)
		}
		if c.auth != "Bearer sk-test-123" {
			t.Fatalf("judge call missing bearer auth: %q", c.auth)
		}
	}
	if fake.count() != 1 {
		t.Fatalf("delegator calls = %d, want 1", fake.count())
	}
}

// ---- 用例 3:判读 proto 门 ----

func TestModelCallProtoGate(t *testing.T) {
	t.Setenv("OS_ENGINE_MODE", "live")
	svc, st := newSvc(t)
	_ = st
	ctx := context.Background()

	// 非 openai 端点 → 报错含 proto=openai 指引
	anthropic := endpoint.Endpoint{Name: "legacy-native", BaseURL: "https://api.anthropic.com", Proto: "anthropic", SelectedModel: "claude-x"}
	if _, err := svc.modelCall(ctx, anthropic, "hi"); err == nil || !strings.Contains(err.Error(), "proto=openai") {
		t.Fatalf("anthropic endpoint should be rejected with proto=openai guidance, got %v", err)
	}
	if _, err := svc.modelCall(ctx, endpoint.Endpoint{Name: "auto", Proto: "auto"}, "hi"); err == nil {
		t.Fatalf("auto proto should also be rejected (explicit gateway only)")
	}

	// openai 网关 → 正常回 Content
	gw := newJudgeGateway(t)
	ep := seedEndpoint(t, svc.store, seedCompanyID(t, svc.store), gw.srv.URL)
	out, err := svc.modelCall(ctx, ep, "You are QA. hi")
	if err != nil {
		t.Fatalf("openai judge call failed: %v", err)
	}
	if !strings.Contains(out, "TEST OK") {
		t.Fatalf("judge content = %q", out)
	}
}

// ---- 用例 4:判读 length 截断(显式错误,不静默当成功) ----

func TestModelCallLengthTruncation(t *testing.T) {
	t.Setenv("OS_ENGINE_MODE", "live")
	svc, st := newSvc(t)
	_ = st
	ctx := context.Background()
	gw := newJudgeGateway(t)
	gw.respond = func(body string) (string, string) { return "long partial text", "length" }
	ep := seedEndpoint(t, svc.store, seedCompanyID(t, svc.store), gw.srv.URL)
	if _, err := svc.modelCall(ctx, ep, "review"); err == nil || !strings.Contains(err.Error(), "truncated") {
		t.Fatalf("length finish_reason should surface an explicit truncation error, got %v", err)
	}
}

// ---- 3.3.3:空 diff 不静默(delegateWriter 报错) ----

func TestDelegateWriterEmptyDiff(t *testing.T) {
	t.Setenv("OS_ENGINE_MODE", "live")
	svc, st := newSvc(t)
	ctx := context.Background()
	compID := seedCompanyID(t, st)
	ws := seedGitWorkspace(t)
	svc.delegator = &writingDelegator{} // 不写文件
	tk := seedEngineChildTask(t, svc, compID, ws, nil)
	if _, err := svc.delegateWriter(ctx, tk, engCallCtx{role: engRoleWriter, round: 0}); err == nil ||
		!strings.Contains(err.Error(), "no workspace changes") {
		t.Fatalf("empty delegation should error, got %v", err)
	}
}

// ---- delegateBaseline 判别(脏起点:任务已有委派记录 → 放行) ----

func TestDelegateBaselinePriorDelegationAllowsDirty(t *testing.T) {
	t.Setenv("OS_ENGINE_MODE", "live")
	svc, st := newSvc(t)
	ctx := context.Background()
	compID := seedCompanyID(t, st)
	ws := seedGitWorkspace(t)

	// 制造脏工作树(任务自己的残留)
	if err := os.WriteFile(filepath.Join(ws, "residue.txt"), []byte("our own prior work\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// 无委派审计 → 拒绝(认领起点须 clean)
	noPrior := task.Task{ID: uuid.NewString(), CompanyID: compID, WorkspacePath: ws}
	if err := svc.delegateBaseline(ctx, noPrior); err == nil || !strings.Contains(err.Error(), "has not delegated before") {
		t.Fatalf("dirty + no prior delegation should be refused, got %v", err)
	}

	// 落一条本任务的 eng_delegate 审计 → 放行(残留 = 自己的返工产物)
	if _, err := svc.audit(ctx, "task", noPrior.ID, "eng_delegate", "agent:none", "claude round=0 retry=0"); err != nil {
		t.Fatalf("audit: %v", err)
	}
	if err := svc.delegateBaseline(ctx, noPrior); err != nil {
		t.Fatalf("dirty + prior delegation should be allowed (rework semantics), got %v", err)
	}

	// scripted 一律跳过(即使空 workspace 也放行)
	t.Setenv("OS_ENGINE_MODE", "scripted")
	if err := svc.delegateBaseline(ctx, task.Task{ID: noPrior.ID, WorkspacePath: t.TempDir()}); err != nil {
		t.Fatalf("scripted should skip baseline, got %v", err)
	}
}
