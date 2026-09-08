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

// UpdatePipelineSchedule 更新流水线 schedule(原文落库);返回更新后整行。
func (s *Store) UpdatePipelineSchedule(ctx context.Context, id, schedule string) (pipeline.Pipeline, error) {
	row, err := s.q.UpdatePipelineSchedule(ctx, query.UpdatePipelineScheduleParams{
		Schedule: schedule, UpdatedAt: now(), ID: id,
	})
	if err != nil {
		return pipeline.Pipeline{}, err
	}
	return toPipeline(row), nil
}

// ListScheduledPipelines 返回全部 active 且 schedule 非空的流水线子集(供 server 调度轮询)。
// Phase 10.2:schedule 由 service 层校验过合法(空串 = 不调度,不落进此集)。
func (s *Store) ListScheduledPipelines(ctx context.Context) ([]pipeline.ScheduledPipeline, error) {
	rows, err := s.q.ListScheduledPipelines(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]pipeline.ScheduledPipeline, 0, len(rows))
	for _, r := range rows {
		out = append(out, pipeline.ScheduledPipeline{
			ID: r.ID, ProjectID: r.ProjectID, Name: r.Name, Kind: r.Kind, Schedule: r.Schedule,
		})
	}
	return out, nil
}

func toPipeline(r query.Pipeline) pipeline.Pipeline {
	return pipeline.Pipeline{
		ID: r.ID, ProjectID: r.ProjectID, Name: r.Name, Kind: r.Kind,
		Description: r.Description, Risk: r.Risk, Status: r.Status, Schedule: r.Schedule,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}
