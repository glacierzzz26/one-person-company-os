package server

// Phase 10 D7 HTTP 契约用例(项目响应 code_source 富化 + refresh 端点 + 登记口已删):
//   - Vsrv-1 建项目(git 带 GitHub origin)→ 201 响应 data.code_source 非空(owner/repo/repo_url/bound/has_github);
//     无 remote 项目 → code_source=null(列表 + GET 一致)
//   - Vsrv-2 refresh-code:建后补 remote → POST …/code-source/refresh 200 返回认领代码源;再 GET 项目 code_source 可见
//   - Vsrv-3 GET/POST /companies/{id}/repos 已删 → 404(登记口去掉;同步走 …/intake/sync)
//   - Vsrv-4 已初始化(设 console token)→ 无令牌刷新 → 401
// 建项目 root 用真 git(经 os/exec;server 包无 service 的 gitDirCmd)。additive:既有解到 project.Project 的用例零 body 改。

import (
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func seedGitRemote(t *testing.T, url string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) {
		args = append([]string{"-C", dir}, args...)
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q", "-b", "main")
	run("config", "user.email", "test@opos.local")
	run("config", "user.name", "opos-test")
	if url != "" {
		run("remote", "add", "origin", url)
	}
	return dir
}

func TestVsrv1ProjectCodeSourceEnrichment(t *testing.T) {
	srv, st := newTestServer(t)
	comp := seedCompany(t, st, "ACME", "")

	// 有 GitHub origin 的项目 → code_source 富化。
	dir := seedGitRemote(t, "https://github.com/acme/web.git")
	rec, env := doAPI(t, srv.Handler(), http.MethodPost, "/api/v1/companies/"+comp.ID+"/projects",
		`{"name":"web","root_path":"`+dir+`"}`)
	if rec.Code != http.StatusCreated || !env.OK {
		t.Fatalf("create remote project: code=%d ok=%v err=%+v", rec.Code, env.OK, env.Error)
	}
	var created struct {
		ID         string          `json:"id"`
		CodeSource *codeSourceView `json:"code_source"`
	}
	decodeData(t, env, &created)
	if created.CodeSource == nil {
		t.Fatalf("Vsrv-1 create response must carry code_source, got %s", env.Data)
	}
	if !created.CodeSource.HasGithub || created.CodeSource.Owner != "acme" || created.CodeSource.Repo != "web" ||
		created.CodeSource.RepoURL != "https://github.com/acme/web.git" || !created.CodeSource.Bound {
		t.Fatalf("Vsrv-1 code_source = %+v, want acme/web bound github", created.CodeSource)
	}

	// 列表 + GET 都带 code_source。
	rec, env = doAPI(t, srv.Handler(), http.MethodGet, "/api/v1/companies/"+comp.ID+"/projects", "")
	var list []struct {
		CodeSource *codeSourceView `json:"code_source"`
	}
	decodeData(t, env, &list)
	if rec.Code != http.StatusOK || len(list) != 1 || list[0].CodeSource == nil || list[0].CodeSource.Repo != "web" {
		t.Fatalf("Vsrv-1 list code_source missing: %s (code=%d)", env.Data, rec.Code)
	}
	rec, env = doAPI(t, srv.Handler(), http.MethodGet, "/api/v1/projects/"+created.ID, "")
	var one struct {
		CodeSource *codeSourceView `json:"code_source"`
	}
	decodeData(t, env, &one)
	if one.CodeSource == nil || one.CodeSource.Repo != "web" {
		t.Fatalf("Vsrv-1 GET code_source missing: %s", env.Data)
	}

	// 无 remote 项目 → code_source=null(另一个公司隔离)。
	comp2 := seedCompany(t, st, "BETA", "")
	bareDir := seedGitRemote(t, "") // init 无 origin
	rec, env = doAPI(t, srv.Handler(), http.MethodPost, "/api/v1/companies/"+comp2.ID+"/projects",
		`{"name":"bare","root_path":"`+bareDir+`"}`)
	if rec.Code != http.StatusCreated || !env.OK {
		t.Fatalf("create bare project: code=%d err=%+v", rec.Code, env.Error)
	}
	var bare struct {
		CodeSource *codeSourceView `json:"code_source"`
	}
	decodeData(t, env, &bare)
	if bare.CodeSource != nil {
		t.Fatalf("Vsrv-1 no-remote project code_source must be null, got %+v", bare.CodeSource)
	}
}

func TestVsrv2RefreshCodeSourceEndpoint(t *testing.T) {
	srv, st := newTestServer(t)
	comp := seedCompany(t, st, "ACME", "")

	// 先建无 remote 项目 → 无代码源。
	dir := seedGitRemote(t, "")
	rec, env := doAPI(t, srv.Handler(), http.MethodPost, "/api/v1/companies/"+comp.ID+"/projects",
		`{"name":"app","root_path":"`+dir+`"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d", rec.Code)
	}
	var prj struct {
		ID   string          `json:"id"`
		Code *codeSourceView `json:"code_source"`
	}
	decodeData(t, env, &prj)
	if prj.Code != nil {
		t.Fatalf("initial code_source want null, got %+v", prj.Code)
	}

	// 无 remote refresh → 400 bad_request(ErrInvalid 带指引)。
	rec, env = doAPI(t, srv.Handler(), http.MethodPost, "/api/v1/projects/"+prj.ID+"/code-source/refresh", "")
	if rec.Code != http.StatusBadRequest || env.Error == nil || env.Error.Code != "bad_request" {
		t.Fatalf("no-remote refresh: code=%d err=%+v, want 400 bad_request", rec.Code, env.Error)
	}

	// 后补 remote → refresh 200,返回认领代码源;GET 项目同步可见。
	run := func(args ...string) {
		args = append([]string{"-C", dir}, args...)
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("remote", "add", "origin", "https://github.com/acme/app.git")
	rec, env = doAPI(t, srv.Handler(), http.MethodPost, "/api/v1/projects/"+prj.ID+"/code-source/refresh", "")
	if rec.Code != http.StatusOK || !env.OK {
		t.Fatalf("refresh: code=%d err=%+v", rec.Code, env.Error)
	}
	var cs codeSourceView
	decodeData(t, env, &cs)
	if cs.Owner != "acme" || cs.Repo != "app" || cs.RepoID == "" || !cs.Bound {
		t.Fatalf("refresh returned code source = %+v, want acme/app bound", cs)
	}

	rec, env = doAPI(t, srv.Handler(), http.MethodGet, "/api/v1/projects/"+prj.ID, "")
	var got struct {
		CodeSource *codeSourceView `json:"code_source"`
	}
	decodeData(t, env, &got)
	if got.CodeSource == nil || got.CodeSource.Repo != "app" {
		t.Fatalf("project GET should now carry code_source: %s", env.Data)
	}
}

func TestVsrv3ReposRegistrationSurfaceGone(t *testing.T) {
	srv, st := newTestServer(t)
	comp := seedCompany(t, st, "ACME", "")

	// 登记端点(曾 POST/GET /companies/{id}/repos)已随「登记入口去掉」删除 → 404。
	rec, _ := doAPI(t, srv.Handler(), http.MethodGet, "/api/v1/companies/"+comp.ID+"/repos", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET /repos code=%d, want 404 (registration gone)", rec.Code)
	}
	rec, _ = doAPI(t, srv.Handler(), http.MethodPost, "/api/v1/companies/"+comp.ID+"/repos",
		`{"name":"x","repo_url":"https://github.com/acme/x.git"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("POST /repos code=%d, want 404 (registration gone)", rec.Code)
	}
}

func TestVsrv4RefreshRequiresBearerWhenInitialized(t *testing.T) {
	srv, st := newSetupServer(t)
	comp := seedCompany(t, st, "ACME", "")

	// 初始化(设 console token)→ /api/v1 全组要求 bearer。
	if rec, env := doAPI(t, srv.Handler(), http.MethodPost, "/api/v1/setup", `{"console_token":"sekret-token"}`); rec.Code != http.StatusOK || !env.OK {
		t.Fatalf("setup: code=%d err=%+v", rec.Code, env.Error)
	}
	dir := seedGitRemote(t, "https://github.com/acme/web.git")
	rec, env := doBearer(t, srv.Handler(), http.MethodPost, "/api/v1/companies/"+comp.ID+"/projects",
		"sekret-token", `{"name":"web","root_path":"`+dir+`"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create (bearer): code=%d err=%+v", rec.Code, env.Error)
	}
	var prj struct {
		ID string `json:"id"`
	}
	decodeData(t, env, &prj)

	// 无令牌 refresh → 401 unauthorized(中间件先拦,不到 handler)。
	rec, env = doAPI(t, srv.Handler(), http.MethodPost, "/api/v1/projects/"+prj.ID+"/code-source/refresh", "")
	if rec.Code != http.StatusUnauthorized || env.Error == nil || env.Error.Code != "unauthorized" {
		t.Fatalf("no-bearer refresh: code=%d err=%+v, want 401 unauthorized", rec.Code, env.Error)
	}
}
