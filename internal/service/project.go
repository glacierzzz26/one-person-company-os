package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/glacierzzz26/one-person-company-os/internal/github"
	"github.com/glacierzzz26/one-person-company-os/internal/project"
	osrepo "github.com/glacierzzz26/one-person-company-os/internal/repo"
	"github.com/google/uuid"
)

// Phase 10.1 sentinel(HTTP 层 handleServiceErr 扩展识别:ErrInvalid→400,其余→409)。
var (
	ErrInvalid      = errors.New("project/pipeline: invalid input")
	ErrConflict     = errors.New("project/pipeline: duplicate name")
	ErrProjectBusy  = errors.New("project: has active run")
	ErrPipelineBusy = errors.New("pipeline: project already has an active run")
)

// isUniqueViolation 判断 sqlite 唯一约束冲突(projects(company_id,name)/pipelines(project_id,name))。
func isUniqueViolation(err error) bool {
	return strings.Contains(err.Error(), "UNIQUE constraint failed")
}

// rootReady 校验并把 rootPath 就绪为 git 仓库,不写业务内容(契约 §一.1 + 自动取码方向):
//   - repoURL == "":既有就绪 —— 已存在且 git → 直接用;不存在 → mkdir -p;空目录(无 .git)→ git init;
//     已存在非空但非 git → ErrInvalid(带指引)。
//   - repoURL != "":建时克隆方向 —— 已存在且 git → 直接用(不覆盖本地已有 remote);空目录/不存在 →
//     把远端克隆成本项目代码(git clone 允许进空目录);非空非 git → ErrInvalid。clone 失败 → 建项目失败
//     (rootReady 在落库前,不留空壳半绑定)。
//
// repoURL == "" 时 OS 只做 git init / 读状态,不 clone、不搬、不写入内容;root 在 OS 之外(委派面)。
func rootReady(ctx context.Context, rootPath, repoURL string) error {
	abs, err := filepath.Abs(rootPath)
	if err != nil {
		return fmt.Errorf("%w: bad root path %q: %v", ErrInvalid, rootPath, err)
	}
	info, err := os.Stat(abs)
	switch {
	case err == nil && !info.IsDir():
		return fmt.Errorf("%w: root path %q is not a directory", ErrInvalid, abs)
	case err == nil:
		// 已存在目录:git → 直接用;空目录 → 下方 clone/init;非空非 git → 报错带指引。
		if wsIsGit(ctx, abs) {
			return nil
		}
		empty, derr := dirIsEmpty(abs)
		if derr != nil {
			return derr
		}
		if !empty {
			return fmt.Errorf("%w: root path %q exists, is non-empty, and is not a git repository — "+
				"point at an existing git repo, or an empty/nonexistent path (OS will clone --repo-url, or mkdir + git init without one)",
				ErrInvalid, abs)
		}
	case os.IsNotExist(err):
		if merr := os.MkdirAll(abs, 0o755); merr != nil {
			return fmt.Errorf("mkdir root path %q: %w", abs, merr)
		}
	default:
		return err
	}
	// abs 现为空目录(新建或原空)且非 git:给了 repo_url → 克隆代码;否则 git init。
	if strings.TrimSpace(repoURL) != "" {
		if cerr := gitCloneURL(ctx, strings.TrimSpace(repoURL), abs); cerr != nil {
			return cerr
		}
		return nil
	}
	if _, ierr := gitDirCmd(ctx, abs, "init"); ierr != nil {
		return fmt.Errorf("git init %q: %w", abs, ierr)
	}
	return nil
}

// dirIsEmpty 报告目录是否为空(不含 .git:对非 git 目录做就绪判定用)。
func dirIsEmpty(dir string) (bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false, fmt.Errorf("read dir %q: %w", dir, err)
	}
	return len(entries) == 0, nil
}

// ---- Phase 10 D7:代码源 = 项目一部分(一项目一代码仓,从 git remote 自动认领)----

// gitRemoteOrigin 返回工作区 origin 远程 URL;无 remote 或 git 出错返回空串(调用方容忍)。
func gitRemoteOrigin(ctx context.Context, ws string) string {
	out, err := gitDirCmd(ctx, ws, "remote", "get-url", "origin")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// githubOwnerName 解析 remote 的 owner/repo;非 GitHub remote / 解析不出 → has=false。
func githubOwnerName(remote string) (owner, name string, has bool) {
	return github.ParseOwnerRepo(remote)
}

// sameWorkspacePath 判定两条目录路径是否指向同一位置(realpath 优先,失败退回 abs)。
func sameWorkspacePath(a, b string) bool {
	abs := func(p string) string {
		if x, err := filepath.Abs(p); err == nil {
			return x
		}
		return p
	}
	ea, e1 := filepath.EvalSymlinks(abs(a))
	eb, e2 := filepath.EvalSymlinks(abs(b))
	if e1 == nil && e2 == nil {
		return ea == eb
	}
	return abs(a) == abs(b)
}

// repoTiesToProject 判断 legacy repos 行是否该被本项目收养:工作区同路径,或同一 GitHub owner/repo。
func repoTiesToProject(r osrepo.Repo, root string, owner, name string, hasRemote bool) bool {
	if sameWorkspacePath(r.WorkspacePath, root) {
		return true
	}
	if !hasRemote {
		return false
	}
	ro, rn, ok := githubOwnerName(r.RepoURL)
	return ok && ro == owner && rn == name
}

// ensureCodeSource 在项目落库后把它的代码源认领出来(Phase 10 D7;best-effort,失败不让建项目失败):
//
//	① 已有绑定 → 不动;② 同公司 legacy 行(同路径/同 owner)→ 收养;③ 否则 remote 可解析 → 派生建行。
//
// 无 GitHub remote 的项目 = 无代码源(通道 B 不适用),静默跳过。错误记 audit 附注但不中断。
func (s *Service) ensureCodeSource(ctx context.Context, p project.Project) {
	if _, err := s.store.GetRepoByProject(ctx, p.ID); err == nil {
		return // 已绑定
	}
	remote := gitRemoteOrigin(ctx, p.RootPath)
	owner, repoName, hasRemote := githubOwnerName(remote)

	// ② 收养 legacy 行(只在同公司内找;同路径优先于同 owner)。
	if list, lerr := s.store.ListRepos(ctx, p.CompanyID); lerr == nil {
		for _, r := range list {
			if r.ProjectID != nil {
				continue
			}
			if !repoTiesToProject(r, p.RootPath, owner, repoName, hasRemote) {
				continue
			}
			if serr := s.store.SetRepoProject(ctx, r.ID, p.ID); serr != nil {
				_, _ = s.audit(ctx, "repo", r.ID, "bind", "system", "adopt failed: "+serr.Error())
				return
			}
			_, _ = s.audit(ctx, "repo", r.ID, "bind", "system", "adopted by project "+p.Name)
			return
		}
	}

	// ③ 派生:无 GitHub remote → 无代码源。
	if !hasRemote {
		return
	}
	now := time.Now().Unix()
	pid := p.ID
	_, err := s.store.CreateRepo(ctx, osrepo.Repo{
		ID: uuid.NewString(), CompanyID: p.CompanyID, Name: p.Name, RepoURL: remote,
		WorkspacePath: p.RootPath, ProjectID: &pid, CreatedAt: now,
	})
	if err != nil {
		_, _ = s.audit(ctx, "project", p.ID, "code_source", "system", "derive failed: "+err.Error())
		return
	}
	_, _ = s.audit(ctx, "repo", p.ID, "bind", "system", remote)
}

// RefreshProjectCodeSourceAs 给已建项目(重)认领/更新代码源:remote 后补或更换后调用。
// 项目根无 GitHub origin → ErrInvalid 带指引。
func (s *Service) RefreshProjectCodeSourceAs(ctx context.Context, projectID, actor string) (osrepo.Repo, error) {
	p, err := s.store.GetProject(ctx, projectID)
	if err != nil {
		return osrepo.Repo{}, err
	}
	remote := gitRemoteOrigin(ctx, p.RootPath)
	owner, repoName, hasRemote := githubOwnerName(remote)
	if !hasRemote {
		return osrepo.Repo{}, fmt.Errorf("%w: project %q has no GitHub origin remote — add one (git remote add origin <github url>) and retry",
			ErrInvalid, p.Name)
	}

	bound, berr := s.store.GetRepoByProject(ctx, p.ID)
	if berr == nil {
		// 已有绑定:刷新派生字段(remote 换了 → 更新 repo_url;name/workspace 同步)。
		if serr := s.store.SetRepoSource(ctx, bound.ID, p.Name, remote, p.RootPath); serr != nil {
			return osrepo.Repo{}, serr
		}
		_, _ = s.audit(ctx, "repo", bound.ID, "refresh", actor, remote)
		return s.store.GetRepoByProject(ctx, p.ID)
	}
	if !errors.Is(berr, sql.ErrNoRows) {
		return osrepo.Repo{}, berr
	}

	// 无绑定:先试收养同公司 legacy 行,再派生。
	if list, lerr := s.store.ListRepos(ctx, p.CompanyID); lerr == nil {
		for _, r := range list {
			if r.ProjectID != nil {
				continue
			}
			if repoTiesToProject(r, p.RootPath, owner, repoName, true) {
				if serr := s.store.SetRepoProject(ctx, r.ID, p.ID); serr != nil {
					return osrepo.Repo{}, serr
				}
				_, _ = s.audit(ctx, "repo", r.ID, "bind", actor, "adopted by project "+p.Name)
				return s.store.GetRepoByProject(ctx, p.ID)
			}
		}
	}
	pid := p.ID
	created, cerr := s.store.CreateRepo(ctx, osrepo.Repo{
		ID: uuid.NewString(), CompanyID: p.CompanyID, Name: p.Name, RepoURL: remote,
		WorkspacePath: p.RootPath, ProjectID: &pid, CreatedAt: time.Now().Unix(),
	})
	if cerr != nil {
		return osrepo.Repo{}, cerr
	}
	_, _ = s.audit(ctx, "repo", created.ID, "bind", actor, remote)
	return created, nil
}

// CodeSourceFor 返回项目的代码源;无绑定 → (nil, nil)。
func (s *Service) CodeSourceFor(ctx context.Context, projectID string) (*osrepo.Repo, error) {
	r, err := s.store.GetRepoByProject(ctx, projectID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &r, nil
}

func (s *Service) CreateProject(ctx context.Context, companyID, name, rootPath, description, repoURL string) (project.Project, error) {
	return s.CreateProjectAs(ctx, companyID, name, rootPath, description, repoURL, "human:cli")
}

// CreateProjectAs 建项目:root 就绪(空目录/不存在 + repoURL → 自动 clone,见 rootReady)→ 落库 → audit。
// repoURL 仅驱动建时克隆,不入库:克隆后 origin 即成为代码源单一来源(ensureCodeSource 自动派生)。
func (s *Service) CreateProjectAs(ctx context.Context, companyID, name, rootPath, description, repoURL, actor string) (project.Project, error) {
	if companyID == "" || name == "" || rootPath == "" {
		return project.Project{}, fmt.Errorf("%w: --company, --name and --root-path are required", ErrInvalid)
	}
	if err := rootReady(ctx, rootPath, repoURL); err != nil {
		return project.Project{}, err
	}
	abs, _ := filepath.Abs(rootPath)
	now := time.Now().Unix()
	p := project.Project{
		ID: uuid.NewString(), CompanyID: companyID, Name: name,
		RootPath: abs, Description: description, CreatedAt: now, UpdatedAt: now,
	}
	created, err := s.store.CreateProject(ctx, p)
	if err != nil {
		if isUniqueViolation(err) {
			return project.Project{}, fmt.Errorf("%w: project %q already exists in company %s", ErrConflict, name, companyID)
		}
		return project.Project{}, err
	}
	_, err = s.audit(ctx, "project", created.ID, "create", actor, abs)
	s.ensureCodeSource(ctx, created) // D7:代码源从 git remote 自动认领(best-effort,失败不影响建项目)
	return created, err
}

func (s *Service) GetProject(ctx context.Context, id string) (project.Project, error) {
	return s.store.GetProject(ctx, id)
}

func (s *Service) ListProjects(ctx context.Context, companyID string) ([]project.Project, error) {
	return s.store.ListProjectsByCompany(ctx, companyID)
}

// DeleteProjectAs 删项目(受控 DELETE,契约 §一.5):须无活跃 run(否则 ErrProjectBusy→409);
// 历史任务先断 project 关联(project_id → NULL,任务与审计保留)→ 级联删其流水线 → 删项目行。
// 绝不触碰 root_path 磁盘目录。
// DeleteProject 删项目(CLI actor human:cli;见 DeleteProjectAs)。
func (s *Service) DeleteProject(ctx context.Context, id string) error {
	return s.DeleteProjectAs(ctx, id, "human:cli")
}

func (s *Service) DeleteProjectAs(ctx context.Context, id, actor string) error {
	p, err := s.store.GetProject(ctx, id)
	if err != nil {
		return err
	}
	cnt, err := s.store.CountActiveTasksByProject(ctx, id)
	if err != nil {
		return err
	}
	if cnt > 0 {
		return fmt.Errorf("%w: project %q has %d active run(s) — finish or cancel before deleting", ErrProjectBusy, p.Name, cnt)
	}
	if _, err := s.store.ClearTaskProject(ctx, id); err != nil {
		return err
	}
	if _, err := s.store.DeletePipelinesByProject(ctx, id); err != nil {
		return err
	}
	// 代码源解绑回 legacy(project_id → NULL):ledger/磁盘目录全保留,也避免 FK 挡删除。
	if err := s.store.ClearRepoProject(ctx, id); err != nil {
		return err
	}
	if _, err := s.store.DeleteProject(ctx, id); err != nil {
		return err
	}
	_, err = s.audit(ctx, "project", id, "delete", actor, p.Name+" "+p.RootPath)
	return err
}
