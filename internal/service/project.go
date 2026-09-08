package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/glacierzzz26/one-person-company-os/internal/project"
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

// rootReady 校验并把 rootPath 就绪为 git 仓库,不写业务内容(契约 §一.1):
//   - 已存在且 git → 直接用;
//   - 不存在 → mkdir -p;空目录(无 .git)→ git init;
//   - 已存在非空但非 git → ErrInvalid(带指引)。
//
// 不 clone、不搬、不写入内容;root 在 OS 之外(委派面),OS 只做 git init / 读状态。
func rootReady(ctx context.Context, rootPath string) error {
	abs, err := filepath.Abs(rootPath)
	if err != nil {
		return fmt.Errorf("%w: bad root path %q: %v", ErrInvalid, rootPath, err)
	}
	info, err := os.Stat(abs)
	switch {
	case err == nil && !info.IsDir():
		return fmt.Errorf("%w: root path %q is not a directory", ErrInvalid, abs)
	case err == nil:
		// 已存在目录:git → 直接用;空目录 → git init;非空非 git → 报错带指引。
		if wsIsGit(ctx, abs) {
			return nil
		}
		empty, derr := dirIsEmpty(abs)
		if derr != nil {
			return derr
		}
		if !empty {
			return fmt.Errorf("%w: root path %q exists, is non-empty, and is not a git repository — "+
				"point at an existing git repo, or an empty/nonexistent path (OS will mkdir + git init)",
				ErrInvalid, abs)
		}
	case os.IsNotExist(err):
		if merr := os.MkdirAll(abs, 0o755); merr != nil {
			return fmt.Errorf("mkdir root path %q: %w", abs, merr)
		}
	default:
		return err
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

func (s *Service) CreateProject(ctx context.Context, companyID, name, rootPath, description string) (project.Project, error) {
	return s.CreateProjectAs(ctx, companyID, name, rootPath, description, "human:cli")
}

// CreateProjectAs 建项目:root 就绪(git init 见 rootReady)→ 落库 → audit。
func (s *Service) CreateProjectAs(ctx context.Context, companyID, name, rootPath, description, actor string) (project.Project, error) {
	if companyID == "" || name == "" || rootPath == "" {
		return project.Project{}, fmt.Errorf("%w: --company, --name and --root-path are required", ErrInvalid)
	}
	if err := rootReady(ctx, rootPath); err != nil {
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
	if _, err := s.store.DeleteProject(ctx, id); err != nil {
		return err
	}
	_, err = s.audit(ctx, "project", id, "delete", actor, p.Name+" "+p.RootPath)
	return err
}
