package service

// 项目 ⇄ GitHub 自动代码获取(方向 project-github-code.md)契约用例:
//   - GC1 建项目 + repo_url → 空目录 OS 自动 clone(git 内容落盘,origin 指向源)
//   - GC2 建项目 + repo_url + 不存在 root → mkdir + clone 成功
//   - GC3 已 git 项目 + repo_url → 复用本地(不覆盖本地 remote/内容)
//   - GC4 非空非 git + repo_url → ErrInvalid 且不落库
//   - GC5 clone 源不可达 → 建项目失败(rootReady 在落库前,不留半绑定)
//   - GC6 run 前自动拉:首个 fresh 认领(基线未钉 + clean + 有 origin)→ HEAD 前移
//   - GC7 基线已钉 / 工作区脏 / scripted → 一律不拉(HEAD 不动)
// git 用真 binary(同 seedGitWorkspace 依赖);拉取从本地路径 remote(免网络,离线可复现)。

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/glacierzzz26/one-person-company-os/internal/task"
)

// cloneProject 用 repoURL 建项目并返回项目 root 绝对路径。
func cloneProject(t *testing.T, svc *Service, compID, name, root, repoURL string) string {
	t.Helper()
	p, err := svc.CreateProject(context.Background(), compID, name, root, "gc", repoURL)
	if err != nil {
		t.Fatalf("CreateProject(%s, repoURL=%s): %v", name, repoURL, err)
	}
	return p.RootPath
}

func TestGC1GC2CloneOnCreate(t *testing.T) {
	svc, st := newSvc(t)
	compID := seedCompanyID(t, st)
	src := seedGitWorkspace(t) // 源仓库:含 base.txt 基线提交

	for _, tc := range []struct {
		name string
		root string
	}{
		{"GC1 empty-dir-clones", filepath.Join(t.TempDir(), "root")},
		{"GC2 nonexistent-path-clones", filepath.Join(t.TempDir(), "does-not-exist", "root")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.name == "GC1 empty-dir-clones" {
				if err := os.MkdirAll(tc.root, 0o755); err != nil {
					t.Fatalf("mkdir root: %v", err)
				}
			}
			got := cloneProject(t, svc, compID, tc.name, tc.root, src)
			// clone 的代码已落盘(源基线文件在)+ 是 git 仓库 + origin 指向源
			if _, err := os.Stat(filepath.Join(got, "base.txt")); err != nil {
				t.Fatalf("clone did not materialize code into %s: %v", got, err)
			}
			if !wsIsGit(context.Background(), got) {
				t.Fatalf("%s is not a git repo after clone", got)
			}
			if origin := gitRemoteOrigin(context.Background(), got); origin != src {
				t.Fatalf("origin = %q, want %q", origin, src)
			}
		})
	}
}

func TestGC3ExistingGitReused(t *testing.T) {
	svc, st := newSvc(t)
	compID := seedCompanyID(t, st)
	// 本地已是一个带 GitHub origin 的 git 工作区(内容由用户自备)。
	dir := seedGitRemoteWorkspace(t, "https://github.com/acme/web.git")
	before := gitRemoteOrigin(context.Background(), dir)

	got := cloneProject(t, svc, compID, "existing", dir, "https://github.com/acme/other.git")
	if origin := gitRemoteOrigin(context.Background(), got); origin != before {
		t.Fatalf("existing git origin overwritten: %q → %q", before, origin)
	}
	if _, err := os.Stat(filepath.Join(got, "base.txt")); err != nil {
		t.Fatalf("existing content lost: %v", err)
	}
}

func TestGC4NonEmptyNonGitRootErr(t *testing.T) {
	svc, st := newSvc(t)
	compID := seedCompanyID(t, st)
	src := seedGitWorkspace(t)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "junk.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write junk: %v", err)
	}
	_, err := svc.CreateProject(context.Background(), compID, "bad", root, "", src)
	if err == nil || !errors.Is(err, ErrInvalid) {
		t.Fatalf("want ErrInvalid for non-empty non-git + repoURL, got %v", err)
	}
	// 未落库
	list, lerr := svc.ListProjects(context.Background(), compID)
	if lerr != nil {
		t.Fatal(lerr)
	}
	if len(list) != 0 {
		t.Fatalf("project persisted despite rootReady failure: %+v", list)
	}
}

func TestGC5CloneUnreachableFailsCreate(t *testing.T) {
	svc, st := newSvc(t)
	compID := seedCompanyID(t, st)
	root := t.TempDir()
	_, err := svc.CreateProject(context.Background(), compID, "missing", root, "", filepath.Join(t.TempDir(), "no-such-repo"))
	if err == nil {
		t.Fatal("want error cloning unreachable source")
	}
	list, lerr := svc.ListProjects(context.Background(), compID)
	if lerr != nil {
		t.Fatal(lerr)
	}
	if len(list) != 0 {
		t.Fatalf("project persisted despite clone failure: %+v", list)
	}
}

// cloneWorktree 从 src 克隆出一个干净工作区(带 origin 追踪),跑拉取语义用。
func cloneWorktree(t *testing.T, src string) string {
	t.Helper()
	ws := t.TempDir()
	runGit(t, src, "clone", "-q", src, ws)
	return ws
}

// advanceRemote 在源仓库加一个新提交(含 marker 文件),供被测工作区拉取。
func advanceRemote(t *testing.T, src, marker string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(src, marker), []byte("v2\n"), 0o644); err != nil {
		t.Fatalf("write marker: %v", err)
	}
	runGit(t, src, "add", "-A")
	runGit(t, src, "commit", "-q", "-m", "advance "+marker)
}

func hasFile(t *testing.T, dir, name string) bool {
	t.Helper()
	_, err := os.Stat(filepath.Join(dir, name))
	return err == nil
}

func newPullTask(compID, id, ws string) task.Task {
	return task.Task{ID: id, CompanyID: compID, WorkspacePath: ws}
}

// GC6:首个 fresh 认领(基线未钉 + clean + 有 origin)→ 自动 pull,HEAD 前移拿到新提交。
func TestGC6PullRunFreshAdvancesHead(t *testing.T) {
	svc, st := newSvc(t)
	compID := seedCompanyID(t, st)
	src := seedGitWorkspace(t)
	ws := cloneWorktree(t, src)
	advanceRemote(t, src, "feature.txt")

	svc.pullRunFresh(context.Background(), newPullTask(compID, "gc6-fresh", ws))
	if !hasFile(t, ws, "feature.txt") {
		t.Fatal("fresh run did not pull upstream commit before start")
	}
}

// GC7:基线已钉 / 脏 / scripted / 无 origin → 一律不拉(HEAD 不动)。
func TestGC7PullRunFreshSkips(t *testing.T) {
	newFixture := func(t *testing.T) (*Service, string, string, string) {
		svc, st := newSvc(t)
		compID := seedCompanyID(t, st)
		src := seedGitWorkspace(t)
		return svc, compID, src, cloneWorktree(t, src)
	}

	t.Run("baseline-pinned-skips", func(t *testing.T) {
		svc, compID, src, ws := newFixture(t)
		runGit(t, ws, "update-ref", "refs/os/tasks/gc7-pinned", "HEAD") // 已钉 = 本 run 已开始
		advanceRemote(t, src, "feature-pinned.txt")
		svc.pullRunFresh(context.Background(), newPullTask(compID, "gc7-pinned", ws))
		if hasFile(t, ws, "feature-pinned.txt") {
			t.Fatal("pulled despite baseline pinned (would pollute task net diff)")
		}
	})

	t.Run("dirty-skips", func(t *testing.T) {
		svc, compID, src, ws := newFixture(t)
		if err := os.WriteFile(filepath.Join(ws, "dirty.txt"), []byte("wip\n"), 0o644); err != nil {
			t.Fatalf("dirty file: %v", err)
		}
		advanceRemote(t, src, "feature-dirty.txt")
		svc.pullRunFresh(context.Background(), newPullTask(compID, "gc7-dirty", ws))
		if hasFile(t, ws, "feature-dirty.txt") {
			t.Fatal("pulled into a dirty worktree (unrelated local changes must be left alone)")
		}
		if !hasFile(t, ws, "dirty.txt") {
			t.Fatal("local dirty file disappeared")
		}
	})

	t.Run("scripted-skips", func(t *testing.T) {
		t.Setenv("OS_ENGINE_MODE", "scripted") // 离线冒烟红线:scripted 不碰网络 git
		svc, compID, src, ws := newFixture(t)
		advanceRemote(t, src, "feature-scripted.txt")
		svc.pullRunFresh(context.Background(), newPullTask(compID, "gc7-scripted", ws))
		if hasFile(t, ws, "feature-scripted.txt") {
			t.Fatal("pulled in scripted (offline) mode")
		}
	})

	t.Run("no-origin-skips", func(t *testing.T) {
		svc, compID, _, _ := newFixture(t)
		ws := seedGitWorkspace(t)                                                    // 本地-only git,无 remote
		svc.pullRunFresh(context.Background(), newPullTask(compID, "gc7-local", ws)) // 不应 panic/拉取
	})
}
