package service

// Phase 10 D7 契约用例(仓库收敛为项目的代码源绑定;方向 declarative-pipelines.md D7):
//   - D1 建项目(git 带 GitHub origin)→ 代码源自动派生落行,project_id 指回,repo_url/workspace=root
//   - D1b 无 remote 项目 → 不建 repos 行(通道 B 不适用,静默)
//   - D2 收养 legacy:先手工 legacy 行(同 owner/repo 或同路径)→ 建项目不新建行,挂 project_id
//   - D3 refresh:建后补 remote → RefreshProjectCodeSourceAs 认领;换 remote → 更新派生字段;无 remote → ErrInvalid
//   - D4 issue 接活:derived 代码源 + fixture issue direct_work → 工程任务 project_id 非空、workspace=项目根
//   - D5 归属去重:同 owner 的 legacy + derived 两行 → SyncRepos 只处理 derived 一条(不双算)
//   - D6 删项目 → repos 行 project_id 置空回 legacy,issue_sync 账本与磁盘仍在
// git remote 用真 git(同 seedGitWorkspace);issue 用 fixture + scripted(离线红线)。

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	osrepo "github.com/glacierzzz26/one-person-company-os/internal/repo"
	"github.com/glacierzzz26/one-person-company-os/internal/settings"
	"github.com/google/uuid"
)

// seedGitRemoteWorkspace 建 git 工作区并配 GitHub origin remote(通道 B 可路由)。
func seedGitRemoteWorkspace(t *testing.T, url string) string {
	t.Helper()
	dir := seedGitWorkspace(t)
	runGit(t, dir, "remote", "add", "origin", url)
	return dir
}

// seedLegacyRepo 直接落一条手工登记 legacy repos 行(project_id 为空;模拟 D7 前登记数据)。
func seedLegacyRepo(t *testing.T, svc *Service, compID, name, repoURL, ws string) osrepo.Repo {
	t.Helper()
	r, err := svc.store.CreateRepo(context.Background(), osrepo.Repo{
		ID: uuid.NewString(), CompanyID: compID, Name: name,
		RepoURL: repoURL, WorkspacePath: ws, CreatedAt: time.Now().Unix(),
	})
	if err != nil {
		t.Fatalf("seed legacy repo %s: %v", name, err)
	}
	return r
}

// seedIssueFixtureCompany 把公司配成 fixture issue 源(离线全链路;scripted 由调用方 env 兜底 —
// company 行 upsert 是全行覆盖,一次写齐 fixture 两个维度即可)。
func seedIssueFixtureCompany(t *testing.T, svc *Service, compID string, issues string) {
	t.Helper()
	fx := writeIssueFixture(t, issues)
	ctx := context.Background()
	if err := svc.UpsertCompanySetting(ctx, settings.CompanySetting{
		CompanyID: compID, IssueSource: sp("fixture"), IssueFixturePath: sp(fx),
	}); err != nil {
		t.Fatalf("UpsertCompanySetting fixture: %v", err)
	}
}

// ---- D1+D1b:建项目自动认领(有 remote 派生 / 无 remote 静默)----

func TestD1DeriveCodeSourceOnCreate(t *testing.T) {
	svc, st := newSvc(t)
	ctx := context.Background()
	compRemote := seedCompanyID(t, st)
	compBare := seedCompanyID(t, st)

	remote := "https://github.com/acme/web.git"
	dir := seedGitRemoteWorkspace(t, remote)
	p, err := svc.CreateProject(ctx, compRemote, "web", dir, "d1", "")
	if err != nil {
		t.Fatalf("CreateProject remote: %v", err)
	}

	// 派生 repos 行自动落:project_id 指回、repo_url=origin、workspace=项目根。
	repo, rerr := svc.CodeSourceFor(ctx, p.ID)
	if rerr != nil {
		t.Fatalf("CodeSourceFor: %v", rerr)
	}
	if repo == nil {
		t.Fatalf("D1 project with github origin must derive a code source")
	}
	if repo.ProjectID == nil || *repo.ProjectID != p.ID {
		t.Fatalf("D1 repo.project_id = %v, want project %s", repo.ProjectID, p.ID)
	}
	if repo.RepoURL != remote {
		t.Fatalf("D1 repo_url = %q, want origin %q", repo.RepoURL, remote)
	}
	if repo.WorkspacePath != dir {
		t.Fatalf("D1 workspace_path = %q, want project root %q", repo.WorkspacePath, dir)
	}
	// 一项目一代码源(唯一索引):建同名项目会 ErrConflict,故换公司验证;同项目只一行。
	all, err := st.ListRepos(ctx, compRemote)
	if err != nil || len(all) != 1 {
		t.Fatalf("D1 company %s repos = %d rows, want 1 (err=%v)", compRemote, len(all), err)
	}

	// D1b:无 remote 项目 → 不派生,正常建。
	pBare, err := svc.CreateProject(ctx, compBare, "bare", seedGitWorkspace(t), "d1b", "")
	if err != nil {
		t.Fatalf("CreateProject bare: %v", err)
	}
	src, serr := svc.CodeSourceFor(ctx, pBare.ID)
	if serr != nil {
		t.Fatalf("D1b CodeSourceFor: %v", serr)
	}
	if src != nil {
		t.Fatalf("D1b project without github origin must have no code source, got %+v", src)
	}
	if list, lerr := st.ListRepos(ctx, compBare); lerr != nil || len(list) != 0 {
		t.Fatalf("D1b company %s repos = %d rows, want 0 (err=%v)", compBare, len(list), lerr)
	}
}

// ---- D2:收养 legacy 行(同 owner/repo 优先;同路径在无 remote 项目也可收养)----

func TestD2AdoptLegacyRepo(t *testing.T) {
	svc, st := newSvc(t)
	ctx := context.Background()
	comp := seedCompanyID(t, st)

	// 同 owner/repo 的 legacy 行(workspace 指向别处)先存在。
	legacyOwner := seedLegacyRepo(t, svc, comp, "web-old", "https://github.com/acme/web.git", filepath.Join(t.TempDir(), "elsewhere"))

	dir := seedGitRemoteWorkspace(t, "https://github.com/acme/web.git")
	p, err := svc.CreateProject(ctx, comp, "web", dir, "d2", "")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	// 收养:不新建行(总数仍 1),legacy 行挂上 project_id。
	repos, err := st.ListRepos(ctx, comp)
	if err != nil || len(repos) != 1 {
		t.Fatalf("D2 after adopt repos = %d rows, want 1 (err=%v)", len(repos), err)
	}
	repo, rerr := svc.CodeSourceFor(ctx, p.ID)
	if rerr != nil {
		t.Fatalf("CodeSourceFor: %v", rerr)
	}
	if repo == nil || repo.ID != legacyOwner.ID {
		t.Fatalf("D2 adopted repo id = %v, want legacy %s", repo, legacyOwner.ID)
	}
	if repo.ProjectID == nil || *repo.ProjectID != p.ID {
		t.Fatalf("D2 adopted repo project_id = %v, want %s", repo.ProjectID, p.ID)
	}

	// 同路径收养(无 remote 项目):legacy 行 workspace == 项目根 → 收养。
	wsDir := seedGitWorkspace(t)
	legacyPath := seedLegacyRepo(t, svc, comp, "path-repo", "/local/path/repo.git", wsDir)
	p2, err := svc.CreateProject(ctx, comp, "path-project", wsDir, "d2b", "")
	if err != nil {
		t.Fatalf("CreateProject path-adopt: %v", err)
	}
	src2, s2err := svc.CodeSourceFor(ctx, p2.ID)
	if s2err != nil {
		t.Fatalf("D2b CodeSourceFor: %v", s2err)
	}
	if src2 == nil || src2.ID != legacyPath.ID {
		t.Fatalf("D2b same-path legacy should be adopted, got %+v", src2)
	}
}

// ---- D3:RefreshProjectCodeSourceAs(建后补 remote / 换 remote / 无 remote 报错)----

func TestD3RefreshCodeSource(t *testing.T) {
	svc, st := newSvc(t)
	ctx := context.Background()
	comp := seedCompanyID(t, st)

	// 建项目时无 remote → 无代码源。
	dir := seedGitWorkspace(t)
	p, err := svc.CreateProject(ctx, comp, "app", dir, "d3", "")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	// 无 remote refresh → ErrInvalid(带指引)。
	if _, err := svc.RefreshProjectCodeSourceAs(ctx, p.ID, "test"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("D3 no-remote refresh want ErrInvalid, got %v", err)
	}

	// 后补 remote → refresh 认领派生。
	runGit(t, dir, "remote", "add", "origin", "https://github.com/acme/app.git")
	repo, err := svc.RefreshProjectCodeSourceAs(ctx, p.ID, "test")
	if err != nil {
		t.Fatalf("D3 refresh after add remote: %v", err)
	}
	if repo.ProjectID == nil || *repo.ProjectID != p.ID || repo.RepoURL != "https://github.com/acme/app.git" {
		t.Fatalf("D3 refreshed repo = %+v, want bound acme/app", repo)
	}

	// 换 remote → refresh 更新派生字段(不新建行)。
	runGit(t, dir, "remote", "set-url", "origin", "https://github.com/acme/app2.git")
	repo2, err := svc.RefreshProjectCodeSourceAs(ctx, p.ID, "test")
	if err != nil {
		t.Fatalf("D3 refresh after set-url: %v", err)
	}
	if repo2.ID != repo.ID || repo2.RepoURL != "https://github.com/acme/app2.git" {
		t.Fatalf("D3 update-in-place failed: got %+v (want same id %s, url app2)", repo2, repo.ID)
	}
	if all, _ := st.ListRepos(ctx, comp); len(all) != 1 {
		t.Fatalf("D3 after refresh repos = %d rows, want 1", len(all))
	}
}

// ---- D4:derived 代码源 issue 接活 → 任务归属项目、用项目目录 ----

func TestD4IssueTaskBindsProject(t *testing.T) {
	t.Setenv("OS_ENGINE_MODE", "scripted") // 双保险(company 已配 scripted)
	svc, st := newSvc(t)
	ctx := context.Background()
	comp := seedCompanyID(t, st)
	seedIssueFixtureCompany(t, svc, comp,
		`[{"repo_owner":"acme","repo_name":"web","number":11,"title":"Fix nav","body":"the nav is broken","html_url":"https://github.com/acme/web/issues/11"}]`)

	dir := seedGitRemoteWorkspace(t, "https://github.com/acme/web.git")
	p, err := svc.CreateProject(ctx, comp, "web", dir, "d4", "")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	repo, rerr := svc.CodeSourceFor(ctx, p.ID)
	if rerr != nil || repo == nil {
		t.Fatalf("derive code source: repo=%v err=%v", repo, rerr)
	}

	results, err := svc.SyncRepos(ctx, comp)
	if err != nil {
		t.Fatalf("SyncRepos: %v", err)
	}
	if len(results) != 1 || len(results[0].CreatedTasks) != 1 {
		t.Fatalf("D4 expected 1 repo / 1 created task, got results=%d created=%d err=%v", len(results), len(results[0].CreatedTasks), err)
	}
	tk, gerr := svc.GetTask(ctx, results[0].CreatedTasks[0])
	if gerr != nil {
		t.Fatalf("GetTask: %v", gerr)
	}
	if tk.ProjectID == nil || *tk.ProjectID != p.ID {
		t.Fatalf("D4 issue task project_id = %v, want project %s", tk.ProjectID, p.ID)
	}
	if tk.WorkspacePath != p.RootPath {
		t.Fatalf("D4 issue task workspace = %q, want project root %q", tk.WorkspacePath, p.RootPath)
	}
	// 账本落(repo, issue)。
	if _, gerr := st.GetIssueSync(ctx, repo.ID, 11); gerr != nil {
		t.Fatalf("D4 issue ledger missing #11: %v", gerr)
	}
}

// ---- D5:归属去重(同 owner 的 legacy + derived → 只处理 derived,不双算)----

func TestD5LegacyDedupOnSync(t *testing.T) {
	t.Setenv("OS_ENGINE_MODE", "scripted")
	svc, st := newSvc(t)
	ctx := context.Background()
	comp := seedCompanyID(t, st)
	seedIssueFixtureCompany(t, svc, comp,
		`[{"repo_owner":"acme","repo_name":"web","number":21,"title":"Second bug","body":"","html_url":"https://github.com/acme/web/issues/21"}]`)

	// legacy 与 derived 同 owner/repo(legacy 先登记,workspace 别处)。
	seedLegacyRepo(t, svc, comp, "web-legacy", "https://github.com/acme/web.git", filepath.Join(t.TempDir(), "old-ws"))
	dir := seedGitRemoteWorkspace(t, "https://github.com/acme/web.git")
	p, err := svc.CreateProject(ctx, comp, "web", dir, "d5", "")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	results, err := svc.SyncRepos(ctx, comp)
	if err != nil {
		t.Fatalf("SyncRepos: %v", err)
	}
	// 只处理 derived 一条(legacy dup 被跳过 → 不双算)。
	if len(results) != 1 {
		t.Fatalf("D5 results = %d repos processed, want 1 (legacy dup skipped)", len(results))
	}
	if len(results[0].CreatedTasks) != 1 {
		t.Fatalf("D5 created tasks = %d, want 1", len(results[0].CreatedTasks))
	}
	tk, gerr := svc.GetTask(ctx, results[0].CreatedTasks[0])
	if gerr != nil {
		t.Fatalf("GetTask: %v", gerr)
	}
	if tk.ProjectID == nil || *tk.ProjectID != p.ID {
		t.Fatalf("D5 issue must be routed to the project-bound code source, got project_id %v", tk.ProjectID)
	}
	// 账本只有一条(无 legacy 双算)。
	if ledger, lerr := st.ListIssueSync(ctx, comp); lerr != nil || len(ledger) != 1 {
		t.Fatalf("D5 issue ledger = %d rows, want 1 (err=%v)", len(ledger), lerr)
	}
}

// ---- D6:删项目 → 代码源解绑回 legacy,账本/磁盘保留 ----

func TestD6DeleteProjectUnbindsRepo(t *testing.T) {
	t.Setenv("OS_ENGINE_MODE", "scripted")
	svc, st := newSvc(t)
	ctx := context.Background()
	comp := seedCompanyID(t, st)
	seedIssueFixtureCompany(t, svc, comp,
		`[{"repo_owner":"acme","repo_name":"web","number":31,"title":"Fix api","body":"","html_url":"https://github.com/acme/web/issues/31"}]`)

	dir := seedGitRemoteWorkspace(t, "https://github.com/acme/web.git")
	p, err := svc.CreateProject(ctx, comp, "web", dir, "d6", "")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	results, err := svc.SyncRepos(ctx, comp)
	if err != nil {
		t.Fatalf("SyncRepos: %v", err)
	}
	taskID := results[0].CreatedTasks[0]

	// 完成 issue 任务(删项目守卫:无活跃 run)。
	if _, err := st.CompleteTask(ctx, taskID, "ok"); err != nil {
		t.Fatalf("CompleteTask: %v", err)
	}
	if err := svc.DeleteProject(ctx, p.ID); err != nil {
		t.Fatalf("DeleteProject: %v", err)
	}

	// repos 行仍在,project_id 置空(回 legacy)。
	repos, err := st.ListRepos(ctx, comp)
	if err != nil || len(repos) != 1 {
		t.Fatalf("D6 repos after delete = %d rows, want 1 (err=%v)", len(repos), err)
	}
	if repos[0].ProjectID != nil {
		t.Fatalf("D6 repo project_id = %v, want NULL (legacy after project delete)", *repos[0].ProjectID)
	}
	// issue_sync 账本保留。
	if ledger, lerr := st.ListIssueSync(ctx, comp); lerr != nil || len(ledger) != 1 {
		t.Fatalf("D6 issue ledger must survive project delete: got %d rows err=%v", len(ledger), lerr)
	}
	// 磁盘目录仍在。
	if _, serr := os.Stat(dir); serr != nil {
		t.Fatalf("D6 disk directory must remain: %v", serr)
	}
	// 完成的任务 project_id 断引用(任务保留)。
	if tk, gerr := svc.GetTask(ctx, taskID); gerr != nil || tk.ProjectID != nil {
		t.Fatalf("D6 task project_id = %v (want nil), err=%v", tk.ProjectID, gerr)
	}
}
