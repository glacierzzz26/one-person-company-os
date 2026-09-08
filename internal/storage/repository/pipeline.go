package repository

import (
	"context"

	"github.com/glacierzzz26/one-person-company-os/internal/pipeline"
	"github.com/glacierzzz26/one-person-company-os/internal/storage/query"
)

func (s *Store) CreatePipeline(ctx context.Context, p pipeline.Pipeline) (pipeline.Pipeline, error) {
	row, err := s.q.CreatePipeline(ctx, query.CreatePipelineParams{
		ID: p.ID, ProjectID: p.ProjectID, Name: p.Name, Kind: p.Kind,
		Description: p.Description, Risk: p.Risk, Status: p.Status, Schedule: p.Schedule,
		CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
	})
	if err != nil {
		return pipeline.Pipeline{}, err
	}
	return toPipeline(row), nil
}

func (s *Store) GetPipeline(ctx context.Context, id string) (pipeline.Pipeline, error) {
	row, err := s.q.GetPipeline(ctx, id)
	if err != nil {
		return pipeline.Pipeline{}, err
	}
	return toPipeline(row), nil
}

func (s *Store) ListPipelinesByProject(ctx context.Context, projectID string) ([]pipeline.Pipeline, error) {
	rows, err := s.q.ListPipelinesByProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	out := make([]pipeline.Pipeline, 0, len(rows))
	for _, r := range rows {
		out = append(out, toPipeline(r))
	}
	return out, nil
}

// DeletePipeline 删除单条流水线(元数据);磁盘与历史任务不动。
func (s *Store) DeletePipeline(ctx context.Context, id string) (int64, error) {
	return s.q.DeletePipeline(ctx, id)
}

// DeletePipelinesByProject 删项目下全部流水线;删项目前置(FK ON)。
func (s *Store) DeletePipelinesByProject(ctx context.Context, projectID string) (int64, error) {
	return s.q.DeletePipelinesByProject(ctx, projectID)
}

func toPipeline(r query.Pipeline) pipeline.Pipeline {
	return pipeline.Pipeline{
		ID: r.ID, ProjectID: r.ProjectID, Name: r.Name, Kind: r.Kind,
		Description: r.Description, Risk: r.Risk, Status: r.Status, Schedule: r.Schedule,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}
