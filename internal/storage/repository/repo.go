package repository

import (
	"context"

	osrepo "github.com/glacierzzz26/one-person-company-os/internal/repo"
	"github.com/glacierzzz26/one-person-company-os/internal/storage/query"
)

func (s *Store) CreateRepo(ctx context.Context, r osrepo.Repo) (osrepo.Repo, error) {
	row, err := s.q.CreateRepo(ctx, query.CreateRepoParams{
		ID: r.ID, CompanyID: r.CompanyID, Name: r.Name, RepoUrl: r.RepoURL,
		WorkspacePath: r.WorkspacePath, CreatedAt: r.CreatedAt,
	})
	if err != nil {
		return osrepo.Repo{}, err
	}
	return toRepo(row), nil
}

func (s *Store) GetRepo(ctx context.Context, id string) (osrepo.Repo, error) {
	row, err := s.q.GetRepo(ctx, id)
	if err != nil {
		return osrepo.Repo{}, err
	}
	return toRepo(row), nil
}

func (s *Store) ListRepos(ctx context.Context, companyID string) ([]osrepo.Repo, error) {
	rows, err := s.q.ListRepos(ctx, companyID)
	if err != nil {
		return nil, err
	}
	out := make([]osrepo.Repo, 0, len(rows))
	for _, r := range rows {
		out = append(out, toRepo(r))
	}
	return out, nil
}

// ListAllRepos 返回全部公司登记的仓库(server 轮询 / os intake sync 不带 --company)。
func (s *Store) ListAllRepos(ctx context.Context) ([]osrepo.Repo, error) {
	rows, err := s.q.ListAllRepos(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]osrepo.Repo, 0, len(rows))
	for _, r := range rows {
		out = append(out, toRepo(r))
	}
	return out, nil
}

// UpsertIssueSync 记一条 issue 处置;同 (repo_id, issue_number) 冲突时覆盖处置结果(幂等)。
func (s *Store) UpsertIssueSync(ctx context.Context, rec osrepo.IssueSync) (osrepo.IssueSync, error) {
	row, err := s.q.UpsertIssueSync(ctx, query.UpsertIssueSyncParams{
		ID: rec.ID, CompanyID: rec.CompanyID, RepoID: rec.RepoID, IssueNumber: rec.IssueNumber,
		Title: rec.Title, Disposition: rec.Disposition, TaskID: ptrToNull(rec.TaskID),
		Note: rec.Note, CreatedAt: rec.CreatedAt,
	})
	if err != nil {
		return osrepo.IssueSync{}, err
	}
	return toIssueSync(row), nil
}

func (s *Store) GetIssueSync(ctx context.Context, repoID string, issueNumber int64) (osrepo.IssueSync, error) {
	row, err := s.q.GetIssueSync(ctx, query.GetIssueSyncParams{
		RepoID: repoID, IssueNumber: issueNumber,
	})
	if err != nil {
		return osrepo.IssueSync{}, err
	}
	return toIssueSync(row), nil
}

func (s *Store) ListIssueSync(ctx context.Context, companyID string) ([]osrepo.IssueSync, error) {
	rows, err := s.q.ListIssueSync(ctx, companyID)
	if err != nil {
		return nil, err
	}
	out := make([]osrepo.IssueSync, 0, len(rows))
	for _, r := range rows {
		out = append(out, toIssueSync(r))
	}
	return out, nil
}

func toRepo(r query.Repo) osrepo.Repo {
	return osrepo.Repo{
		ID: r.ID, CompanyID: r.CompanyID, Name: r.Name, RepoURL: r.RepoUrl,
		WorkspacePath: r.WorkspacePath, CreatedAt: r.CreatedAt,
	}
}

func toIssueSync(r query.IssueSync) osrepo.IssueSync {
	return osrepo.IssueSync{
		ID: r.ID, CompanyID: r.CompanyID, RepoID: r.RepoID, IssueNumber: r.IssueNumber,
		Title: r.Title, Disposition: r.Disposition, TaskID: nullToPtr(r.TaskID),
		Note: r.Note, CreatedAt: r.CreatedAt,
	}
}
