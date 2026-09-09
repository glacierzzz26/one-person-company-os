package server

// 项目 ⇄ GitHub 自动代码获取 HTTP 契约用例(project-github-code.md §四.C/§五,Gsrv-*):
//   - Gsrv-1 POST /companies/{id}/projects 带 repo_url(nonexistent root)→ 201 + OS 自动 clone:
//     返回 root_path 内容已落盘(git 源基线文件在)+ 项目 code_source 认领(origin=src,Bound,
//     非 GitHub remote → has_github=false);repo_url 可空(既有 Vsrv-1/makeProjViaAPI 已盖空语义)。
// 建 root 与 src 用真 git(os/exec;server 包无 service gitDirCmd);clone 从本地 src 路径(离线可复现)。

import (
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// seedGitSrc 在临时目录建一个带基线提交的 git 源仓库(供被测 root 克隆)。
func seedGitSrc(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "src")
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
	if err := os.WriteFile(filepath.Join(dir, "base.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "-A")
	run("commit", "-q", "-m", "base")
	return dir
}

func TestGsrv1CreateProjectClonesRepoURL(t *testing.T) {
	srv, st := newTestServer(t)
	comp := seedCompany(t, st, "ACME", "")
	src := seedGitSrc(t)

	// nonexitent root + repo_url → 201;服务端 OS 自动 clone 代码进来。
	root := filepath.Join(t.TempDir(), "bound", "repo")
	rec, env := doAPI(t, srv.Handler(), http.MethodPost, "/api/v1/companies/"+comp.ID+"/projects",
		`{"name":"bound","root_path":"`+root+`","repo_url":"`+src+`"}`)
	if rec.Code != http.StatusCreated || !env.OK {
		t.Fatalf("Gsrv-1 create+clone: code=%d ok=%v err=%+v body=%s", rec.Code, env.OK, env.Error, rec.Body.String())
	}
	var created struct {
		ID         string          `json:"id"`
		RootPath   string          `json:"root_path"`
		CodeSource *codeSourceView `json:"code_source"`
	}
	decodeData(t, env, &created)

	// (1) 代码已落盘:返回 root 目录里有 src 的基线文件。
	if created.RootPath == "" {
		t.Fatal("Gsrv-1 create response missing root_path")
	}
	if _, err := os.Stat(filepath.Join(created.RootPath, "base.txt")); err != nil {
		t.Fatalf("Gsrv-1 clone did not materialize code into %s: %v", created.RootPath, err)
	}
	// (2) 克隆后是 git 仓库且 origin = 源(代码源单一来源)。
	if out, err := exec.Command("git", "-C", created.RootPath, "remote", "get-url", "origin").CombinedOutput(); err != nil || strings.TrimSpace(string(out)) != src {
		t.Fatalf("Gsrv-1 origin = %q err=%v, want %q", strings.TrimSpace(string(out)), err, src)
	}
	// (3) code_source:本地路径 remote 非 GitHub → null(ensureCodeSource 只认 GitHub 可解析 origin;
	//     GitHub origin 的绑定富化由 Vsrv-1(已 git + GitHub origin)盖;两半合起来 = 真实 GitHub clone 的
	//     clone 机械 + 绑定形状,离线各证其半)。
	if created.CodeSource != nil {
		t.Fatalf("Gsrv-1 code_source = %+v, want null for non-GitHub origin", created.CodeSource)
	}
}
