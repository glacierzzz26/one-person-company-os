package repository

import (
	"context"

	"github.com/glacierzzz26/one-person-company-os/internal/project"
	"github.com/glacierzzz26/one-person-company-os/internal/storage/query"
)

func (s *Store) CreateProject(ctx context.Context, p project.Project) (project.Project, error) {
	row, err := s.q.CreateProject(ctx, query.CreateProjectParams{
		ID: p.ID, CompanyID: p.CompanyID, Name: p.Name,
		RootPath: p.RootPath, Description: p.Description,
		CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
	})
	if err != nil {
		return project.Project{}, err
	}
	return toProject(row), nil
}

func (s *Store) GetProject(ctx context.Context, id string) (project.Project, error) {
	row, err := s.q.GetProject(ctx, id)
	if err != nil {
		return project.Project{}, err
	}
	return toProject(row), nil
}

func (s *Store) ListProjectsByCompany(ctx context.Context, companyID string) ([]project.Project, error) {
	rows, err := s.q.ListProjectsByCompany(ctx, companyID)
	if err != nil {
		return nil, err
	}
	out := make([]project.Project, 0, len(rows))
	for _, r := range rows {
		out = append(out, toProject(r))
	}
	return out, nil
}

// DeleteProject 删除项目行。调用方必须先 ClearTaskProject + DeletePipelinesByProject
// (FK ON);磁盘目录永不动(受控 DELETE 边界)。
func (s *Store) DeleteProject(ctx context.Context, id string) (int64, error) {
	return s.q.DeleteProject(ctx, id)
}

func toProject(r query.Project) project.Project {
	return project.Project{
		ID: r.ID, CompanyID: r.CompanyID, Name: r.Name,
		RootPath: r.RootPath, Description: r.Description,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}
