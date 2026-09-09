package service

// Phase 10.5 契约用例(github-roundtrip-pr.md §五,service 层;离线,免网络):
//   - P1 项目机密 CRUD + 解析双层:项目 token 覆盖公司回退;项目无 → 公司 github_token;均无 → ok=false
//   - P2 分支名 / prefix / slug 纯函数单测(确定性)
//   - P3 gitAuthEnv 纯函数:https-github origin 注入 Basic x-access-token,token 只进 env;其余 origin nil
//   - P4 originDefaultBranch:克隆仓带 origin/HEAD → 分支名;无 origin/HEAD → ""
//   - P5 ensureIssueBranch 幂等:切换 → 已在 → 再来 no-op;非 git no-op
//   - P6 发 PR 端到端(finishRun):origin=fetch URL 是 github 字面量(OS 解析 prTarget/GetRepoByProject 可见),
//     push 经 remote.origin.pushurl 落本地 bare → push 分支成功 → CreatePull(owner/repo/title/head/base/
//     body Resolves #N)被 fake 记录 → pull_request_url/number 落库 → audit pr_opened → 人工 PublishTaskPR 幂等不重复建
//   - P7 静默 skip:publishRunPR scripted / 无项目 token(pr_skip)/ 无 prTarget(无 ledger/无 project)
//   - P8 PublishTaskPR 门:未完成 / scripted / 无 prTarget / 无项目 token → ErrInvalid(不发网络)
//   - P9 SyncProject(fixture)issue→task(项目级同步只处理绑仓)
//   - P10 UpdateProjectAs 编辑:meta 改名/同目标不动盘/空名 ErrInvalid/重名 ErrConflict/占位仓 clone 认领/
//       带历史改挂 ErrInvalid
// git 用真 binary;离线法:OS 读 origin 用 `git remote get-url`(fetch URL),因此 fetch URL 保持 github 字面量
// 才能派生代码源 / prTarget 命中;push 要离线不能靠 url.insteadOf(它反向改写 get-url 输出 → OS 解析不出),
// 改走 remote.origin.pushurl = 本地 bare。clone 类用例用本地路径 repoURL(同 D7/GC 约定)。

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/glacierzzz26/one-person-company-os/internal/github"
	"github.com/glacierzzz26/one-person-company-os/internal/project"
	osrepo "github.com/glacierzzz26/one-person-company-os/internal/repo"
	"github.com/glacierzzz26/one-person-company-os/internal/settings"
	"github.com/glacierzzz26/one-person-company-os/internal/storage/repository"
	"github.com/glacierzzz26/one-person-company-os/internal/task"
	"github.com/google/uuid"
)

// ---- fake PullPublisher(记录 CreatePull 参,不触网)----

type pullCall struct {
	owner string
	repo  string
	p     github.PullParams
}

type fakePulls struct{ calls []pullCall }

func (f *fakePulls) CreatePull(_ context.Context, owner, repo string, p github.PullParams) (github.Pull, error) {
	f.calls = append(f.calls, pullCall{owner: owner, repo: repo, p: p})
	return github.Pull{Number: 77, HTMLURL: "https://github.com/acme/web/pull/77"}, nil
}

func (f *fakePulls) last() pullCall { return f.calls[len(f.calls)-1] }

// ---- 离线 GitHub 工作区:origin = github 字面量 + pushurl 落本地 bare ----

// newGitHubWorkspace 从本地 bare 克隆工作区:fetch URL(= git remote get-url origin,OS 解析 origin 用)
// 指到 github 字面量(派生代码源 / prTarget 命中),push 经 remote.origin.pushurl 落本地 bare(离线)。
// origin/HEAD 在改 URL 前先对本地 bare 建好(离线),保证 originDefaultBranch 可解析出 base。
func newGitHubWorkspace(t *testing.T, bare, originURL string) string {
	t.Helper()
	ws := cloneWorktree(t, bare)
	runGit(t, ws, "remote", "set-head", "origin", "-a") // 本地 ls-remote(bare),先建 refs/remotes/origin/HEAD
	runGit(t, ws, "remote", "set-url", "origin", originURL)
	runGit(t, ws, "config", "remote.origin.pushurl", bare)
	return ws
}

// newBareRepo 建一个独立本地 bare(带 main 提交),供 push 落点。
func newBareRepo(t *testing.T) string {
	t.Helper()
	src := seedGitWorkspace(t)
	bare := filepath.Join(t.TempDir(), "web.git")
	runGit(t, src, "clone", "-q", "--bare", src, bare)
	return bare
}

// prHarness 是「来源 GitHub issue 的工程任务」夹具:项目(绑 acme/web 代码源)+ 工程任务 + issue_sync
// 回链。引擎默认 live(不开 scripted seam);主密钥已注入(项目机密加密封存需要)。测试按需补 token / 完成态。
type prHarness struct {
	svc    *Service
	st     *repository.Store
	comp   string
	p      project.Project
	ws     string
	bare   string
	taskID string
}

func newPRHarness(t *testing.T, title string, issue int64) *prHarness {
	t.Helper()
	svc, st := newSvc(t)
	ctx := context.Background()
	comp := seedCompanyID(t, st)
	settings.UseMasterKey(key32())
	t.Cleanup(func() { settings.UseMasterKey(nil) })

	base := newBareRepo(t)
	const originURL = "https://github.com/acme/web.git"
	ws := newGitHubWorkspace(t, base, originURL)
	p, err := svc.CreateProject(ctx, comp, "web", ws, "roundtrip", "")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	repo, rerr := svc.CodeSourceFor(ctx, p.ID)
	if rerr != nil || repo == nil {
		t.Fatalf("derive code source: repo=%v err=%v", repo, rerr)
	}

	tid := uuid.NewString()
	now := time.Now().Unix()
	if _, err := st.CreateTask(ctx, task.Task{
		ID: tid, CompanyID: comp, Title: title, ToolName: "engineering",
		Status: "pending", QStatus: "ready", Risk: "medium", MaxAttempts: 1, TimeoutSec: 60,
		WorkspacePath: p.RootPath, ProjectID: &p.ID, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if _, err := st.UpsertIssueSync(ctx, osrepo.IssueSync{
		ID: uuid.NewString(), CompanyID: comp, RepoID: repo.ID, IssueNumber: issue,
		Title: title, Disposition: "direct_work", TaskID: &tid, CreatedAt: now,
	}); err != nil {
		t.Fatalf("UpsertIssueSync: %v", err)
	}
	return &prHarness{svc: svc, st: st, comp: comp, p: p, ws: ws, bare: base, taskID: tid}
}

func (h *prHarness) get(t *testing.T) task.Task {
	t.Helper()
	tk, err := h.svc.GetTask(context.Background(), h.taskID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	return tk
}

func (h *prHarness) auditActions(t *testing.T, entity string) []string {
	t.Helper()
	rows, err := h.st.ListAudits(context.Background(), entity)
	if err != nil {
		t.Fatalf("ListAudits(%s): %v", entity, err)
	}
	acts := make([]string, 0, len(rows))
	for _, a := range rows {
		acts = append(acts, a.Action)
	}
	return acts
}

func containsStr(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

func currentBranch(t *testing.T, ws string) string {
	t.Helper()
	out, err := gitDirCmd(context.Background(), ws, "branch", "--show-current")
	if err != nil {
		t.Fatalf("git branch --show-current: %v", err)
	}
	return strings.TrimSpace(out)
}

// ---- P1 项目机密 CRUD + 解析双层 ----

func TestP1ProjectSecretCRUDAndResolution(t *testing.T) {
	svc, st := newSvc(t)
	ctx := context.Background()
	comp := seedCompanyID(t, st)
	settings.UseMasterKey(key32())
	t.Cleanup(func() { settings.UseMasterKey(nil) })
	ws := seedGitWorkspace(t)
	p, err := svc.CreateProject(ctx, comp, "app", ws, "", "")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	// 白名单 + 空值拒。
	if err := svc.SetProjectSecretAs(ctx, p.ID, "api_key", "x", "human:console"); !errors.Is(err, ErrSettingsBadRequest) {
		t.Fatalf("unknown project secret id: err=%v, want ErrSettingsBadRequest", err)
	}
	if err := svc.SetProjectSecretAs(ctx, p.ID, ProjectSecretGitHubToken, "   ", "human:console"); !errors.Is(err, ErrSettingsBadRequest) {
		t.Fatalf("empty value: err=%v, want ErrSettingsBadRequest", err)
	}
	// 未注入前(设了后)… 直接设项目 token。
	if err := svc.SetProjectSecretAs(ctx, p.ID, ProjectSecretGitHubToken, "ghp_proj", "human:console"); err != nil {
		t.Fatalf("SetProjectSecretAs: %v", err)
	}
	// roundtrip。
	plain, ok, err := svc.OpenProjectSecretCurrent(ctx, p.ID, ProjectSecretGitHubToken)
	if err != nil || !ok || plain != "ghp_proj" {
		t.Fatalf("OpenProjectSecretCurrent = ok:%v %q err:%v; want ghp_proj", ok, plain, err)
	}
	// meta 白名单顺序 + 只出掩码;库内只存密文。
	metas, err := svc.ProjectSecretMeta(ctx, p.ID)
	if err != nil || len(metas) != 1 || metas[0].ID != ProjectSecretGitHubToken || !metas[0].Set {
		t.Fatalf("ProjectSecretMeta = %+v err:%v; want one set github_token", metas, err)
	}
	rows, err := st.ListProjectSecrets(ctx, p.ID)
	if err != nil || len(rows) != 1 {
		t.Fatalf("ListProjectSecrets = %+v err:%v; want 1 row", rows, err)
	}
	if !strings.HasPrefix(rows[0].Cipher, "enc:v2:") || strings.Contains(rows[0].Cipher, "ghp_proj") {
		t.Fatalf("project secret cipher leaked plaintext = %q; want enc:v2: only", rows[0].Cipher)
	}

	// 解析双层 ①:项目 token 优先(公司也配了 ghp_comp 仍取项目)。
	if err := svc.SetCompanySecretAs(ctx, comp, SecretGitHubToken, "ghp_comp", "human:console"); err != nil {
		t.Fatalf("SetCompanySecretAs: %v", err)
	}
	if tok, ok, err := svc.githubTokenFor(ctx, comp, p.ID); err != nil || !ok || tok != "ghp_proj" {
		t.Fatalf("project token must win: tok=%q ok=%v err=%v", tok, ok, err)
	}
	// ②:删项目 token → 回退公司。
	if err := svc.DeleteProjectSecretAs(ctx, p.ID, ProjectSecretGitHubToken, "human:console"); err != nil {
		t.Fatalf("DeleteProjectSecretAs: %v", err)
	}
	if tok, ok, err := svc.githubTokenFor(ctx, comp, p.ID); err != nil || !ok || tok != "ghp_comp" {
		t.Fatalf("company fallback: tok=%q ok=%v err=%v", tok, ok, err)
	}
	// ③:公司也没有 → ok=false。
	if err := svc.DeleteSecret(ctx, comp, SecretGitHubToken); err != nil {
		t.Fatalf("DeleteSecret: %v", err)
	}
	if tok, ok, err := svc.githubTokenFor(ctx, comp, p.ID); err != nil || ok || tok != "" {
		t.Fatalf("no token anywhere: tok=%q ok=%v err=%v; want ok=false", tok, ok, err)
	}
}

// ---- P2 分支名 / prefix / slug 纯函数 ----

func TestP2IssueBranchName(t *testing.T) {
	long := "Add " + strings.Repeat("x", 60)   // slug 整体(含前缀词)超 48 → 截断
	slug44 := "add-" + strings.Repeat("x", 44) // "add " 词首进 slug,48 上限里词占 4 格
	cases := []struct {
		title  string
		number int64
		want   string
	}{
		{"Fix nav crash", 7, "bugfix/7-fix-nav-crash"},
		{"Bug in checkout", 5, "bugfix/5-bug-in-checkout"},
		{"Defect: crash handler missing", 9, "bugfix/9-defect-crash-handler-missing"},
		{"修复登录错误", 7, "bugfix/7-issue"},
		{"Add user auth", 12, "feature/12-add-user-auth"},
		{"Add User AUTH", 12, "feature/12-add-user-auth"},
		{"UI: hide sidebar", 4, "feature/4-ui-hide-sidebar"},
		{"增加新功能", 2, "feature/2-issue"},
		{"", 1, "feature/1-issue"},
		{long, 1, "feature/1-" + slug44},
	}
	for _, tc := range cases {
		if got := issueBranchName(tc.number, tc.title); got != tc.want {
			t.Errorf("issueBranchName(%d, %q) = %q; want %q", tc.number, tc.title, got, tc.want)
		}
	}
	// prefix 归类(命中 → bugfix;否则 feature)。
	for _, hit := range []string{"fix", "bug", "修复", "崩溃", "defect", "Crash", "错误"} {
		if prPrefix("has "+hit) != "bugfix" {
			t.Errorf("prPrefix(%q) should be bugfix", hit)
		}
	}
	if prPrefix("feat: land new api") != "feature" {
		t.Error("prPrefix non-bug title should be feature")
	}
}

// ---- P3 gitAuthEnv ----

func TestP3GitAuthEnv(t *testing.T) {
	env := gitAuthEnv("https://github.com/acme/web.git", "sekrit-token")
	if len(env) != 3 {
		t.Fatalf("gitAuthEnv len = %d, want 3", len(env))
	}
	wantAuth := "Authorization: Basic " + base64.StdEncoding.EncodeToString([]byte("x-access-token:sekrit-token"))
	got := map[string]string{}
	for _, kv := range env {
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) == 2 {
			got[parts[0]] = parts[1]
		}
	}
	if got["GIT_CONFIG_COUNT"] != "1" || got["GIT_CONFIG_KEY_0"] != "http.https://github.com/.extraheader" || got["GIT_CONFIG_VALUE_0"] != wantAuth {
		t.Fatalf("gitAuthEnv = %v; want count=1 + extraheader Basic x-access-token", env)
	}
	for _, kv := range env {
		if strings.Contains(kv, "sekrit-token") {
			t.Fatalf("token plaintext leaked into env: %q", kv)
		}
	}
	// 非 https-github origin → nil(明文推,不注入)。
	for _, origin := range []string{"git@github.com:acme/web.git", "https://gitlab.com/a/b.git", "/local/repo.git", ""} {
		if env := gitAuthEnv(origin, "t"); env != nil {
			t.Fatalf("gitAuthEnv(%q) = %v; want nil", origin, env)
		}
	}
}

// ---- P4 originDefaultBranch ----

func TestP4OriginDefaultBranch(t *testing.T) {
	ctx := context.Background()
	src := seedGitWorkspace(t) // main 分支,克隆后 origin/HEAD → main
	ws := cloneWorktree(t, src)
	if got := originDefaultBranch(ctx, ws); got != "main" {
		t.Fatalf("originDefaultBranch(clone) = %q; want main", got)
	}
	// 删 origin/HEAD 符号引用 → ""。
	runGit(t, ws, "remote", "set-head", "origin", "-d")
	if got := originDefaultBranch(ctx, ws); got != "" {
		t.Fatalf("originDefaultBranch(no HEAD symref) = %q; want empty", got)
	}
	// 无 origin 工作区 → ""。
	if got := originDefaultBranch(ctx, seedGitWorkspace(t)); got != "" {
		t.Fatalf("originDefaultBranch(no remote) = %q; want empty", got)
	}
}

// ---- P5 ensureIssueBranch 幂等 ----

func TestP5EnsureIssueBranchIdempotent(t *testing.T) {
	ctx := context.Background()
	ws := seedGitWorkspace(t)
	const branch = "feature/7-add-user-auth"
	if err := ensureIssueBranch(ctx, ws, branch); err != nil {
		t.Fatalf("ensureIssueBranch (create): %v", err)
	}
	if cur := currentBranch(t, ws); cur != branch {
		t.Fatalf("after checkout -b current = %q; want %q", cur, branch)
	}
	// 已在目标分支 → no-op 不报错。
	if err := ensureIssueBranch(ctx, ws, branch); err != nil {
		t.Fatalf("ensureIssueBranch (already there): %v", err)
	}
	// 切走后再 ensure → checkout 存在分支,仍在目标分支。
	runGit(t, ws, "checkout", "main")
	if err := ensureIssueBranch(ctx, ws, branch); err != nil {
		t.Fatalf("ensureIssueBranch (existing): %v", err)
	}
	if cur := currentBranch(t, ws); cur != branch {
		t.Fatalf("re-checkout current = %q; want %q", cur, branch)
	}
	// 非 git / 空目录 → no-op。
	if err := ensureIssueBranch(ctx, t.TempDir(), branch); err != nil {
		t.Fatalf("ensureIssueBranch on non-git dir: %v", err)
	}
}

// ---- P6 发 PR 端到端(finishRun → push → CreatePull → 落库 + audit + 人工幂等)----

func TestP6PublishPREndToEnd(t *testing.T) {
	h := newPRHarness(t, "Fix nav crash", 7)
	ctx := context.Background()
	const tok = "ghp_e2e_token"
	if err := h.svc.SetProjectSecretAs(ctx, h.p.ID, ProjectSecretGitHubToken, tok, "test"); err != nil {
		t.Fatalf("SetProjectSecretAs: %v", err)
	}
	fake := &fakePulls{}
	h.svc.prPub = fake

	tsk := h.get(t)
	if err := h.svc.finishRun(ctx, tsk, "roundtrip ok"); err != nil {
		t.Fatalf("finishRun: %v", err)
	}
	// 完成态照常;任务落 PR 账本。
	got := h.get(t)
	if got.Status != "completed" {
		t.Fatalf("task status = %q; want completed", got.Status)
	}
	if got.PullRequestURL == "" || got.PullRequestNumber == nil || *got.PullRequestNumber != 77 {
		t.Fatalf("task PR ledger = url:%q number:%v; want pull/77", got.PullRequestURL, got.PullRequestNumber)
	}
	// 分支被创建并推到本地 bare。
	if cur := currentBranch(t, h.ws); cur != "bugfix/7-fix-nav-crash" {
		t.Fatalf("workspace current branch = %q; want bugfix/7-fix-nav-crash", cur)
	}
	if _, err := gitDirCmd(ctx, h.bare, "rev-parse", "--verify", "--quiet", "refs/heads/bugfix/7-fix-nav-crash"); err != nil {
		t.Fatalf("bare remote missing pushed issue branch: %v", err)
	}
	// CreatePull 参数:owner/repo/title/head/base + body Resolves #7。
	if len(fake.calls) != 1 {
		t.Fatalf("CreatePull calls = %d; want 1", len(fake.calls))
	}
	c := fake.last()
	if c.owner != "acme" || c.repo != "web" || c.p.Title != "Fix nav crash" || c.p.Head != "bugfix/7-fix-nav-crash" || c.p.Base != "main" {
		t.Fatalf("CreatePull(%s/%s) = %+v; want acme/web title/head/base main", c.owner, c.repo, c.p)
	}
	if !strings.Contains(c.p.Body, "Resolves #7") {
		t.Fatalf("PR body missing Resolves #7: %q", c.p.Body)
	}
	if !containsStr(h.auditActions(t, "task"), "pr_opened") {
		t.Fatalf("audit missing pr_opened, got %v", h.auditActions(t, "task"))
	}
	// 人工 publish-pr 幂等:已置 url → 返回既有,不重复建。
	again, err := h.svc.PublishTaskPR(ctx, h.taskID, "human:console")
	if err != nil {
		t.Fatalf("PublishTaskPR (idempotent): %v", err)
	}
	if again.PullRequestURL != got.PullRequestURL {
		t.Fatalf("idempotent re-publish url = %q; want %q", again.PullRequestURL, got.PullRequestURL)
	}
	if len(fake.calls) != 1 {
		t.Fatalf("idempotent re-publish must not call CreatePull again: calls = %d", len(fake.calls))
	}
}

// ---- P7 publishRunPR 静默 skip(scripted / 无 token / 无 target)----

func TestP7PublishRunPRSilentSkips(t *testing.T) {
	t.Run("scripted-no-op", func(t *testing.T) {
		h := newPRHarness(t, "Fix nav crash", 7)
		t.Setenv("OS_ENGINE_MODE", "scripted") // 离线红线:scripted 不建分支不发 PR
		tsk := h.get(t)
		if err := h.svc.publishRunPR(context.Background(), tsk, "test"); err != nil {
			t.Fatalf("scripted publishRunPR: %v", err)
		}
		if got := h.get(t); got.PullRequestURL != "" {
			t.Fatalf("scripted must not open PR, got url %q", got.PullRequestURL)
		}
		if cur := currentBranch(t, h.ws); cur != "main" {
			t.Fatalf("scripted must not branch, current = %q", cur)
		}
		for _, act := range h.auditActions(t, "task") {
			if strings.HasPrefix(act, "pr_") {
				t.Fatalf("scripted skip should not audit %q", act)
			}
		}
	})

	t.Run("no-project-token-pr-skip", func(t *testing.T) {
		h := newPRHarness(t, "Fix nav crash", 7) // 主密钥已注入,但无项目 secret
		tsk := h.get(t)
		if err := h.svc.publishRunPR(context.Background(), tsk, "test"); err != nil {
			t.Fatalf("no-token publishRunPR: %v", err)
		}
		if got := h.get(t); got.PullRequestURL != "" {
			t.Fatalf("no token must not open PR, got url %q", got.PullRequestURL)
		}
		if !containsStr(h.auditActions(t, "task"), "pr_skip") {
			t.Fatalf("no-token should audit pr_skip, got %v", h.auditActions(t, "task"))
		}
	})

	t.Run("no-pr-target-silent", func(t *testing.T) {
		h := newPRHarness(t, "Fix nav crash", 7)
		tsk := h.get(t)
		tsk.ProjectID = nil // 非 issue 载体任务 → prTarget 不命中
		if err := h.svc.publishRunPR(context.Background(), tsk, "test"); err != nil {
			t.Fatalf("no-target publishRunPR: %v", err)
		}
		if len(h.auditActions(t, "task")) != 0 {
			t.Fatalf("no-target should be silent, got audits %v", h.auditActions(t, "task"))
		}
	})
}

// ---- P8 PublishTaskPR 门(不发网络)----

func TestP8PublishTaskPRGates(t *testing.T) {
	ctx := context.Background()
	t.Run("not-completed", func(t *testing.T) {
		h := newPRHarness(t, "Fix nav crash", 7)
		_, err := h.svc.PublishTaskPR(ctx, h.taskID, "human:console")
		if !errors.Is(err, ErrInvalid) {
			t.Fatalf("pending task publish err = %v; want ErrInvalid", err)
		}
	})
	t.Run("scripted-engine", func(t *testing.T) {
		h := newPRHarness(t, "Fix nav crash", 7)
		if _, err := h.st.CompleteTask(ctx, h.taskID, "ok"); err != nil {
			t.Fatalf("CompleteTask: %v", err)
		}
		t.Setenv("OS_ENGINE_MODE", "scripted")
		_, err := h.svc.PublishTaskPR(ctx, h.taskID, "human:console")
		if !errors.Is(err, ErrInvalid) {
			t.Fatalf("scripted publish err = %v; want ErrInvalid", err)
		}
	})
	t.Run("no-pr-target", func(t *testing.T) {
		h := newPRHarness(t, "Fix nav crash", 7)
		tid2 := uuid.NewString()
		now := time.Now().Unix()
		if _, err := h.st.CreateTask(ctx, task.Task{
			ID: tid2, CompanyID: h.comp, Title: "plain task", ToolName: "engineering",
			Status: "pending", QStatus: "ready", Risk: "low", CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatalf("CreateTask: %v", err)
		}
		if _, err := h.st.CompleteTask(ctx, tid2, "ok"); err != nil {
			t.Fatalf("CompleteTask: %v", err)
		}
		_, err := h.svc.PublishTaskPR(ctx, tid2, "human:console")
		if !errors.Is(err, ErrInvalid) {
			t.Fatalf("no-target publish err = %v; want ErrInvalid", err)
		}
	})
	t.Run("no-project-token", func(t *testing.T) {
		h := newPRHarness(t, "Fix nav crash", 7)
		if _, err := h.st.CompleteTask(ctx, h.taskID, "ok"); err != nil {
			t.Fatalf("CompleteTask: %v", err)
		}
		_, err := h.svc.PublishTaskPR(ctx, h.taskID, "human:console")
		if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "github_token") {
			t.Fatalf("no-token publish err = %v; want ErrInvalid hinting github_token", err)
		}
	})
}

// ---- P9 SyncProject(fixture)issue→task(项目级同步只处理绑仓)----

func TestP9SyncProjectFixtureIssueToTask(t *testing.T) {
	t.Setenv("OS_ENGINE_MODE", "scripted") // fixture 确定性 triage(离线红线)
	svc, st := newSvc(t)
	ctx := context.Background()
	comp := seedCompanyID(t, st)
	seedIssueFixtureCompany(t, svc, comp,
		`[{"repo_owner":"acme","repo_name":"web","number":31,"title":"Crash on login","body":"","html_url":"https://github.com/acme/web/issues/31"}]`)
	ws := seedGitRemoteWorkspace(t, "https://github.com/acme/web.git")
	p, err := svc.CreateProject(ctx, comp, "web", ws, "p9", "")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	res, err := svc.SyncProjectAs(ctx, p.ID, "human:console")
	if err != nil {
		t.Fatalf("SyncProject: %v", err)
	}
	if res == nil || len(res.CreatedTasks) != 1 || res.IssuesSeen != 1 {
		t.Fatalf("SyncProject = %+v; want 1 seen / 1 created", res)
	}
	tk, gerr := svc.GetTask(ctx, res.CreatedTasks[0])
	if gerr != nil {
		t.Fatalf("GetTask: %v", gerr)
	}
	if tk.ProjectID == nil || *tk.ProjectID != p.ID || tk.WorkspacePath != p.RootPath {
		t.Fatalf("issue task must bind project: project=%v workspace=%q", tk.ProjectID, tk.WorkspacePath)
	}
	// issue_sync 回链(Title/issue number 进账本 → prTarget 可命中)。
	rec, lerr := st.GetIssueSyncByTask(ctx, tk.ID)
	if lerr != nil {
		t.Fatalf("GetIssueSyncByTask: %v", lerr)
	}
	if rec.IssueNumber != 31 || rec.Title != "Crash on login" {
		t.Fatalf("issue ledger = %+v; want #31 Crash on login", rec)
	}
	// 审计 actor = console(项目级同步动作)。
	rows, err := st.ListAudits(ctx, "project")
	if err != nil {
		t.Fatalf("ListAudits: %v", err)
	}
	seen := false
	for _, a := range rows {
		if a.Action == "sync_issues" && a.EntityID == p.ID && a.Actor == "human:console" {
			seen = true
		}
	}
	if !seen {
		t.Fatalf("missing project sync_issues audit (console), got %+v", rows)
	}
}

// ---- P10 UpdateProjectAs 编辑(meta / 同目标 / 占位仓 clone 认领 / 带历史拒绝 / 重名)----

func TestP10UpdateProjectMetadataAndRepoint(t *testing.T) {
	svc, st := newSvc(t)
	ctx := context.Background()
	comp := seedCompanyID(t, st)
	const webURL = "https://github.com/acme/web.git"
	const otherURL = "https://github.com/acme/other.git"

	// ① + ③/④ 的 github-origin 工作区:origin 只需被解析(D1 同款),不触网,无需 pushurl。
	wsA := seedGitRemoteWorkspace(t, webURL)
	p, err := svc.CreateProject(ctx, comp, "web", wsA, "old-desc", "")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	// ① meta 改名/描述;repoURL 同目标(去 .git 写法)→ 同 owner/repo 判定命中,不动盘,代码源仍在。
	up, err := svc.UpdateProject(ctx, p.ID, "web-renamed", "new-desc", "https://github.com/acme/web")
	if err != nil {
		t.Fatalf("UpdateProject meta+same-target: %v", err)
	}
	if up.Name != "web-renamed" || up.Description != "new-desc" {
		t.Fatalf("updated project = %+v; want renamed/new-desc", up)
	}
	if origin := gitRemoteOrigin(ctx, up.RootPath); origin != webURL {
		t.Fatalf("same-target must not touch disk origin: got %q want %q", origin, webURL)
	}
	if src, serr := svc.CodeSourceFor(ctx, p.ID); serr != nil || src == nil || src.RepoURL != webURL {
		t.Fatalf("code source after meta edit = %+v err:%v; want still web", src, serr)
	}
	// 空 name → ErrInvalid。
	if _, err := svc.UpdateProject(ctx, p.ID, "   ", "", ""); !errors.Is(err, ErrInvalid) {
		t.Fatalf("empty name err = %v; want ErrInvalid", err)
	}

	// ② 占位仓(零提交、无 origin)改挂 → 删 .git + clone 认领真实代码。clone 用本地路径源(GC/D7 同款
	// 离线约定):真实 GitHub 目标 clone 语义同 CreateProject repoURL,见 GC1-GC5;repoint 后非 GitHub
	// origin 的代码源重认领是 best-effort(失败仅 audit,编辑成功)——GitHub origin 重认领见 D3/P6。
	empty := filepath.Join(t.TempDir(), "placeholder")
	if err := os.MkdirAll(empty, 0o755); err != nil {
		t.Fatalf("mkdir placeholder: %v", err)
	}
	pp, err := svc.CreateProject(ctx, comp, "placeholder", empty, "", "")
	if err != nil {
		t.Fatalf("CreateProject placeholder: %v", err)
	}
	otherSrc := seedGitWorkspace(t) // 带 main 提交的本地源,占位仓 repoint 的 clone 目标
	up2, err := svc.UpdateProject(ctx, pp.ID, "placeholder", "", otherSrc)
	if err != nil {
		t.Fatalf("UpdateProject repoint placeholder: %v", err)
	}
	if _, err := os.Stat(filepath.Join(up2.RootPath, "base.txt")); err != nil {
		t.Fatalf("placeholder repoint did not clone real code: %v", err)
	}
	if origin := gitRemoteOrigin(ctx, up2.RootPath); origin != otherSrc {
		t.Fatalf("placeholder origin after repoint = %q; want %q", origin, otherSrc)
	}

	// ③ 带本地 git 历史(有提交 + github origin)的项目改挂异仓库 → ErrInvalid(不迁移、不发网络)。
	wsB := seedGitRemoteWorkspace(t, webURL)
	pB, err := svc.CreateProject(ctx, comp, "web-b", wsB, "", "")
	if err != nil {
		t.Fatalf("CreateProject web-b: %v", err)
	}
	if _, err := svc.UpdateProject(ctx, pB.ID, "web-b", "", otherURL); !errors.Is(err, ErrInvalid) {
		t.Fatalf("repoint legacy-history err = %v; want ErrInvalid", err)
	}

	// ④ 同公司重名 → ErrConflict(与 ① 的 web-renamed 撞)。
	if _, err := svc.UpdateProject(ctx, pB.ID, "web-renamed", "", ""); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate project name err = %v; want ErrConflict", err)
	}
}
