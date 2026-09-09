package repository

import (
	"context"
	"database/sql"

	osrepo "github.com/glacierzzz26/one-person-company-os/internal/repo"
	"github.com/glacierzzz26/one-person-company-os/internal/storage/query"
)

func (s *Store) CreateRepo(ctx context.Context, r osrepo.Repo) (osrepo.Repo, error) {
	row, err := s.q.CreateRepo(ctx, query.CreateRepoParams{
		ID: r.ID, CompanyID: r.CompanyID, ProjectID: ptrToNull(r.ProjectID),
		Name: r.Name, RepoUrl: r.RepoURL, WorkspacePath: r.WorkspacePath, CreatedAt: r.CreatedAt,
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

// GetRepoByProject 返回某项目的代码源(无绑定 → sql.ErrNoRows)。
func (s *Store) GetRepoByProject(ctx context.Context, projectID string) (osrepo.Repo, error) {
	row, err := s.q.GetRepoByProject(ctx, nullStr(projectID))
	if err != nil {
		return osrepo.Repo{}, err
	}
	return toRepo(row), nil
}

// SetRepoProject 把一条 repos 行挂到项目(收养 legacy 行用;两参数均非空)。
func (s *Store) SetRepoProject(ctx context.Context, repoID, projectID string) error {
	_, err := s.q.SetRepoProject(ctx, query.SetRepoProjectParams{
		ProjectID: nullStr(projectID), ID: repoID,
	})
	return err
}

// SetRepoSource 刷新一条代码源行的派生字段(remote 后补/更换后 refresh 用)。
func (s *Store) SetRepoSource(ctx context.Context, repoID, name, repoURL, workspacePath string) error {
	_, err := s.q.SetRepoSource(ctx, query.SetRepoSourceParams{
		Name: name, RepoUrl: repoURL, WorkspacePath: workspacePath, ID: repoID,
	})
	return err
}

// ClearRepoProject 把某项目的代码源解绑回 legacy(project_id → NULL;删项目前置,防 FK/丢账本)。
func (s *Store) ClearRepoProject(ctx context.Context, projectID string) error {
	_, err := s.q.ClearRepoProject(ctx, nullStr(projectID))
	return err
}

// nullStr 把非空字符串构造成 sql.NullString(调用方保证非空,如 project id)。
func nullStr(s string) sql.NullString {
	return sql.NullString{String: s, Valid: s != ""}
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

// ListAllRepos 返回全部公司登记的代码源(项目 derived + legacy;server 轮询 / os intake sync 不带 --company)。
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

// GetIssueSyncByTask 返回来源 issue 账本(经 issue_sync.task_id 回链)。Phase 10.5 PR 圈定:
// 任务须是某 GitHub issue 的 direct_work/merge 产物才有行;非 issue 来源 → sql.ErrNoRows。
func (s *Store) GetIssueSyncByTask(ctx context.Context, taskID string) (osrepo.IssueSync, error) {
	row, err := s.q.GetIssueSyncByTask(ctx, sql.NullString{String: taskID, Valid: taskID != ""})
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
		WorkspacePath: r.WorkspacePath, ProjectID: nullToPtr(r.ProjectID), CreatedAt: r.CreatedAt,
	}
}

func toIssueSync(r query.IssueSync) osrepo.IssueSync {
	return osrepo.IssueSync{
		ID: r.ID, CompanyID: r.CompanyID, RepoID: r.RepoID, IssueNumber: r.IssueNumber,
		Title: r.Title, Disposition: r.Disposition, TaskID: nullToPtr(r.TaskID),
		Note: r.Note, CreatedAt: r.CreatedAt,
	}
}
