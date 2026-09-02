package service

import (
	"context"
	"fmt"
	"time"

	osrepo "github.com/glacierzzz26/one-person-company-os/internal/repo"
	"github.com/google/uuid"
)

// AddRepo 登记一个研发仓库。写操作落 Audit。
func (s *Service) AddRepo(ctx context.Context, companyID, name, repoURL, workspacePath string) (osrepo.Repo, error) {
	return s.AddRepoAs(ctx, companyID, name, repoURL, workspacePath, "human:cli")
}

// AddRepoAs 同 AddRepo,审计 actor 用传入值(如 human:console)。
func (s *Service) AddRepoAs(ctx context.Context, companyID, name, repoURL, workspacePath, actor string) (osrepo.Repo, error) {
	if companyID == "" || name == "" || repoURL == "" {
		return osrepo.Repo{}, fmt.Errorf("--company, --name and --repo-url are required")
	}
	r := osrepo.Repo{
		ID: uuid.NewString(), CompanyID: companyID, Name: name, RepoURL: repoURL,
		WorkspacePath: workspacePath, CreatedAt: time.Now().Unix(),
	}
	created, err := s.store.CreateRepo(ctx, r)
	if err != nil {
		return osrepo.Repo{}, err
	}
	_, err = s.audit(ctx, "repo", created.ID, "create", actor, repoURL+" "+workspacePath)
	return created, err
}

func (s *Service) ListRepos(ctx context.Context, companyID string) ([]osrepo.Repo, error) {
	if companyID == "" {
		return nil, fmt.Errorf("--company is required")
	}
	return s.store.ListRepos(ctx, companyID)
}

// ListAllRepos 全部公司仓库(server webhook 归属解析 / 无 --company 同步)。
func (s *Service) ListAllRepos(ctx context.Context) ([]osrepo.Repo, error) {
	return s.store.ListAllRepos(ctx)
}

func (s *Service) GetRepo(ctx context.Context, id string) (osrepo.Repo, error) {
	return s.store.GetRepo(ctx, id)
}
