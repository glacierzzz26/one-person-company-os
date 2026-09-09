package server

// Phase 10.5 回环 HTTP 契约用例(github-roundtrip-pr.md §五;全部离线,不触外网):
//   - R1 PUT /projects/{id} 已有项目编辑:name/desc 更新 + code_source 仍绑定;name 缺省保留现有;
//     未知项目 → 404;重名 → 409 conflict(Edit 面)
//   - R2 项目机密 PUT/GET/DELETE(github_token):list 只出掩码、明文永不回显;set/delete 幂等往返;
//     未知 secret id → 400
//   - R3 POST /projects/{id}/intake/sync 项目级通道 B 同步:DB 种子 scripted + fixture(server 包无
//     env seam,产品恒 live → 直插 company 覆盖行)离线确定性 direct_work 建 task + 账本 + audit
//     human:console;二次 sync 幂等 already=1
//   - R4 POST /tasks/{id}/publish-pr 门错误映射(不发网络):未完成 → 400;完成但项目无 github_token
//     → 400(消息含 github_token)
//
// git 工作区用真 git(经 os/exec,server 包无 service gitDirCmd);GitHub origin 只挂不推(parse-only)。

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	osrepo "github.com/glacierzzz26/one-person-company-os/internal/repo"
	"github.com/glacierzzz26/one-person-company-os/internal/settings"
	"github.com/glacierzzz26/one-person-company-os/internal/task"
	"github.com/google/uuid"
)

// seedFixtureFile 写一条 acme/web 的 fixture issue JSON(供 issue_source=fixture 离线路由)。
func seedFixtureFile(t *testing.T, title string, number int64) string {
	t.Helper()
	n := strconv.FormatInt(number, 10)
	body := `[{"repo_owner":"acme","repo_name":"web","number":` + n +
		`,"title":"` + title + `","body":"navigation broken","html_url":"https://github.com/acme/web/issues/` + n + `"}]`
	p := filepath.Join(t.TempDir(), "issues.json")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// createRemoteProject 建一个挂 GitHub origin(parse-only)的项目,返回 id + root。
// token 空 = 未初始化(开放)服务器;非空 = 已初始化(带 Bearer console 令牌)。
func createRemoteProject(t *testing.T, h http.Handler, compID, name, token string) (string, string) {
	t.Helper()
	dir := seedGitRemote(t, "https://github.com/acme/web.git")
	rec, env := doBearer(t, h, http.MethodPost, "/api/v1/companies/"+compID+"/projects",
		token, `{"name":"`+name+`","root_path":"`+dir+`"}`)
	if rec.Code != http.StatusCreated || !env.OK {
		t.Fatalf("create remote project: code=%d err=%+v body=%s", rec.Code, env.Error, rec.Body.String())
	}
	var created struct {
		ID         string          `json:"id"`
		RootPath   string          `json:"root_path"`
		CodeSource *codeSourceView `json:"code_source"`
	}
	decodeData(t, env, &created)
	if created.CodeSource == nil || created.CodeSource.Repo != "web" || !created.CodeSource.HasGithub {
		t.Fatalf("create remote project code_source = %+v, want bound github acme/web", created.CodeSource)
	}
	return created.ID, created.RootPath
}

// R1 PUT /projects/{id} 编辑:元数据更新、name 缺省保留、code_source 保持绑定;404 / 409 映射。
func TestR1UpdateProjectEdit(t *testing.T) {
	srv, st := newTestServer(t)
	comp := seedCompany(t, st, "ACME", "")
	h := srv.Handler()
	pID, _ := createRemoteProject(t, h, comp.ID, "web", "")

	// name + desc 一起改 → 200;code_source 仍绑定 acme/web(bound=true,非本地路径)。
	rec, env := doAPI(t, h, http.MethodPut, "/api/v1/projects/"+pID, `{"name":"web renamed","description":"d2"}`)
	if rec.Code != http.StatusOK || !env.OK {
		t.Fatalf("R1 update name/desc: code=%d err=%+v body=%s", rec.Code, env.Error, rec.Body.String())
	}
	var upd struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		CodeSource  *codeSourceView `json:"code_source"`
	}
	decodeData(t, env, &upd)
	if upd.Name != "web renamed" || upd.Description != "d2" {
		t.Fatalf("R1 update result = %+v, want name+desc changed", upd)
	}
	if upd.CodeSource == nil || upd.CodeSource.Repo != "web" || !upd.CodeSource.HasGithub {
		t.Fatalf("R1 code_source lost after metadata update: %+v", upd.CodeSource)
	}

	// name 缺省(只改 desc)→ name 保留现有(编辑语义,不进空名校验)。
	rec, env = doAPI(t, h, http.MethodPut, "/api/v1/projects/"+pID, `{"description":"only"}`)
	if rec.Code != http.StatusOK || !env.OK {
		t.Fatalf("R1 update desc-only: code=%d err=%+v", rec.Code, env.Error)
	}
	decodeData(t, env, &upd)
	if upd.Name != "web renamed" || upd.Description != "only" {
		t.Fatalf("R1 desc-only result = %+v, want name kept", upd)
	}

	// 未知项目 → 404 not_found。
	rec, env = doAPI(t, h, http.MethodPut, "/api/v1/projects/"+uuid.NewString(), `{"name":"nobody"}`)
	if rec.Code != http.StatusNotFound || env.Error == nil || env.Error.Code != "not_found" {
		t.Fatalf("R1 update unknown project: code=%d err=%+v, want 404 not_found", rec.Code, env.Error)
	}

	// 重名(同公司已有 project)→ 409 conflict。
	dir2 := seedGitRemote(t, "")
	rec, env = doAPI(t, h, http.MethodPost, "/api/v1/companies/"+comp.ID+"/projects",
		`{"name":"second","root_path":"`+dir2+`"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("R1 create second project: code=%d", rec.Code)
	}
	rec, env = doAPI(t, h, http.MethodPut, "/api/v1/projects/"+pID, `{"name":"second"}`)
	if rec.Code != http.StatusConflict || env.Error == nil || env.Error.Code != "conflict" {
		t.Fatalf("R1 rename to duplicate: code=%d err=%+v, want 409 conflict", rec.Code, env.Error)
	}
}

// R2 项目机密 github_token 设/换/删:list 只出掩码;PUT/DELETE 往返;明文永不回显;未知 id → 400。
func TestR2ProjectSecretsMasked(t *testing.T) {
	srv, st := newSetupServer(t)
	t.Cleanup(func() { settings.UseMasterKey(nil) })
	h := srv.Handler()
	tok := setupConsole(t, h) // 注入主密钥 holder(seal 用)+ console 令牌
	comp := seedCompany(t, st, "ACME", "")

	// 建普通项目(本地 git,无 remote;secret 与代码源无关)。
	dir := seedGitRemote(t, "")
	rec, env := doBearer(t, h, http.MethodPost, "/api/v1/companies/"+comp.ID+"/projects",
		tok, `{"name":"web","root_path":"`+dir+`"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("R2 create project: code=%d err=%+v", rec.Code, env.Error)
	}
	var prj struct {
		ID string `json:"id"`
	}
	decodeData(t, env, &prj)

	list := func() []secretMetaDTO {
		t.Helper()
		rec, env := doBearer(t, h, http.MethodGet, "/api/v1/projects/"+prj.ID+"/secrets", tok, "")
		if rec.Code != http.StatusOK || !env.OK {
			t.Fatalf("R2 list secrets: code=%d err=%+v", rec.Code, env.Error)
		}
		var out []secretMetaDTO
		decodeData(t, env, &out)
		return out
	}

	// 初始:白名单仅 github_token,set=false。
	got := list()
	if len(got) != 1 || got[0].ID != "github_token" || got[0].Set {
		t.Fatalf("R2 initial project secret list = %+v, want single github_token unset", got)
	}

	// 未知 id set → 400。
	if rec, env := doBearer(t, h, http.MethodPut, "/api/v1/projects/"+prj.ID+"/secrets/not_a_secret", tok, `{"value":"x"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("R2 set unknown secret: code=%d err=%+v, want 400", rec.Code, env.Error)
	}

	// PUT github_token → set true;任何响应体绝不含明文。
	const plain = "ghp_proj_plain_roundtrip"
	rec, env = doBearer(t, h, http.MethodPut, "/api/v1/projects/"+prj.ID+"/secrets/github_token", tok, `{"value":"`+plain+`"}`)
	if rec.Code != http.StatusOK || !env.OK {
		t.Fatalf("R2 set github_token: code=%d err=%+v", rec.Code, env.Error)
	}
	if strings.Contains(rec.Body.String(), plain) {
		t.Fatalf("R2 set response leaked plaintext:\n%s", rec.Body.String())
	}
	rec, env = doBearer(t, h, http.MethodGet, "/api/v1/projects/"+prj.ID+"/secrets", tok, "")
	if strings.Contains(rec.Body.String(), plain) {
		t.Fatalf("R2 list response leaked plaintext:\n%s", rec.Body.String())
	}
	got = list()
	if len(got) != 1 || !got[0].Set {
		t.Fatalf("R2 secret list after set = %+v, want github_token set", got)
	}

	// DELETE → set=false(幂等往返)。
	rec, env = doBearer(t, h, http.MethodDelete, "/api/v1/projects/"+prj.ID+"/secrets/github_token", tok, "")
	if rec.Code != http.StatusOK || !env.OK {
		t.Fatalf("R2 delete github_token: code=%d err=%+v", rec.Code, env.Error)
	}
	got = list()
	if len(got) != 1 || got[0].Set {
		t.Fatalf("R2 secret list after delete = %+v, want github_token unset", got)
	}
}

// R3 POST /projects/{id}/intake/sync:DB 种子 scripted + fixture → 离线确定性建 engineering task;
// 任务归属项目(project_id)+ 工作区 = 项目根 + 账本回链 + audit human:console;重跑幂等。
func TestR3SyncProjectIssues(t *testing.T) {
	srv, st := newTestServer(t)
	ctx := context.Background()
	comp := seedCompany(t, st, "ACME", "")
	seedCapability(t, st, comp.ID, "engineering") // createIssueTask 尽力找 engineering 归属
	h := srv.Handler()
	pID, root := createRemoteProject(t, h, comp.ID, "web", "")

	// DB 种子公司覆盖:engine=scripted(server 无 env seam → 直插)+ issue_source=fixture + 路径。
	fx := seedFixtureFile(t, "Fix nav crash", 41)
	scripted, fixture := "scripted", "fixture"
	if err := st.UpsertCompanySetting(ctx, settings.CompanySetting{
		CompanyID: comp.ID, EngineMode: &scripted, IssueSource: &fixture,
		IssueFixturePath: &fx, UpdatedAt: time.Now().Unix(),
	}); err != nil {
		t.Fatalf("R3 seed company scripted+fixture: %v", err)
	}

	rec, env := doAPI(t, h, http.MethodPost, "/api/v1/projects/"+pID+"/intake/sync", "")
	if rec.Code != http.StatusOK || !env.OK {
		t.Fatalf("R3 sync project: code=%d err=%+v body=%s", rec.Code, env.Error, rec.Body.String())
	}
	var res struct {
		Repo         string         `json:"repo"`
		IssuesSeen   int            `json:"issues_seen"`
		Already      int            `json:"already"`
		ByDisp       map[string]int `json:"by_disp"`
		CreatedTasks []string       `json:"created_tasks"`
	}
	decodeData(t, env, &res)
	if res.Repo != "web" || res.IssuesSeen != 1 || res.Already != 0 || len(res.CreatedTasks) != 1 || res.ByDisp["direct_work"] != 1 {
		t.Fatalf("R3 sync result = %+v, want web issue seen → 1 direct_work task", res)
	}

	// 建出的任务归属项目 + 工作区 = 项目根(闭环工程在项目目录干)。
	tsk, err := st.GetTask(ctx, res.CreatedTasks[0])
	if err != nil {
		t.Fatalf("R3 get created task: %v", err)
	}
	if tsk.ProjectID == nil || *tsk.ProjectID != pID || tsk.WorkspacePath != root || tsk.ToolName != "engineering" {
		t.Fatalf("R3 created task = %+v, want project-scoped engineering task in project root", tsk)
	}

	// 账本回链(task_id 指向该任务,后续 publish-pr / prTarget 用)。
	syncs, err := st.ListIssueSync(ctx, comp.ID)
	if err != nil || len(syncs) != 1 || syncs[0].Disposition != "direct_work" || syncs[0].TaskID == nil || *syncs[0].TaskID != tsk.ID {
		t.Fatalf("R3 issue ledger = %+v err=%v, want single direct_work row back-linked to task", syncs, err)
	}

	// audit:sync_issues 落 actor=human:console(项目级同步走 console 动作)。
	audits, err := srv.svc.ListAudits(ctx, "project")
	if err != nil {
		t.Fatalf("R3 list project audits: %v", err)
	}
	var sawSync bool
	for _, a := range audits {
		if a.EntityID == pID && a.Action == "sync_issues" {
			sawSync = true
			if a.Actor != "human:console" {
				t.Fatalf("R3 sync audit actor = %q, want human:console", a.Actor)
			}
		}
	}
	if !sawSync {
		t.Fatalf("R3 no sync_issues audit for project %s in %+v", pID, audits)
	}

	// 二次 sync:账本已存在 → already=1、不重复建任务(幂等)。
	rec, env = doAPI(t, h, http.MethodPost, "/api/v1/projects/"+pID+"/intake/sync", "")
	if rec.Code != http.StatusOK || !env.OK {
		t.Fatalf("R3 re-sync: code=%d err=%+v", rec.Code, env.Error)
	}
	var res2 struct {
		Already      int            `json:"already"`
		IssuesSeen   int            `json:"issues_seen"`
		CreatedTasks []string       `json:"created_tasks"`
		ByDisp       map[string]int `json:"by_disp"`
	}
	decodeData(t, env, &res2)
	if res2.IssuesSeen != 1 || res2.Already != 1 || len(res2.CreatedTasks) != 0 || len(res2.ByDisp) != 0 {
		t.Fatalf("R3 re-sync = %+v, want already=1 no new task", res2)
	}
}

// R4 POST /tasks/{id}/publish-pr 门错误映射(契约 §四 E;不发网络):
// 未完成 → 400;完成但项目无 github_token → 400(消息指引在项目代码源卡设 token)。
func TestR4PublishPRGateErrors(t *testing.T) {
	srv, st := newSetupServer(t)
	t.Cleanup(func() { settings.UseMasterKey(nil) })
	ctx := context.Background()
	h := srv.Handler()
	tok := setupConsole(t, h)
	comp := seedCompany(t, st, "ACME", "")
	pID, _ := createRemoteProject(t, h, comp.ID, "web", tok)

	// 未完成(pending)任务 → 400 bad_request,不发网络。
	pending := seedTask(t, st, comp.ID, func(tk *task.Task) { tk.Title = "fix nav crash" })
	rec, env := doBearer(t, h, http.MethodPost, "/api/v1/tasks/"+pending.ID+"/publish-pr", tok, "")
	if rec.Code != http.StatusBadRequest || env.Error == nil || env.Error.Code != "bad_request" || !strings.Contains(env.Error.Message, "not completed") {
		t.Fatalf("R4 publish pending: code=%d err=%+v, want 400 bad_request not-completed", rec.Code, env.Error)
	}

	// 完成态 + prTarget 命中(项目 GitHub 代码源 + issue 账本回链)+ 项目无 github_token → 400 指引。
	repo, err := srv.svc.CodeSourceFor(ctx, pID)
	if err != nil || repo == nil {
		t.Fatalf("R4 code source for project: %+v err=%v", repo, err)
	}
	done := seedTask(t, st, comp.ID, func(tk *task.Task) {
		tk.Title = "fix nav crash"
		tk.Status = "completed"
		tk.QStatus = "completed"
		tk.ProjectID = &pID
		tk.WorkspacePath = repo.WorkspacePath
	})
	tid := done.ID
	if _, err := st.UpsertIssueSync(ctx, osrepo.IssueSync{
		ID: uuid.NewString(), CompanyID: comp.ID, RepoID: repo.ID, IssueNumber: 41,
		Title: "fix nav crash", Disposition: "direct_work", TaskID: &tid,
		Note: "fixture", CreatedAt: time.Now().Unix(),
	}); err != nil {
		t.Fatalf("R4 seed issue ledger: %v", err)
	}
	rec, env = doBearer(t, h, http.MethodPost, "/api/v1/tasks/"+done.ID+"/publish-pr", tok, "")
	if rec.Code != http.StatusBadRequest || env.Error == nil || env.Error.Code != "bad_request" || !strings.Contains(env.Error.Message, "github_token") {
		t.Fatalf("R4 publish completed-no-token: code=%d err=%+v, want 400 bad_request github_token guidance", rec.Code, env.Error)
	}
}
