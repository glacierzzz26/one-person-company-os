package service

import (
	"context"
	"time"

	"github.com/glacierzzz26/one-person-company-os/internal/memory"
	"github.com/google/uuid"
)

func (s *Service) CreateMemory(ctx context.Context, companyID, mtype, title, content, source, tags string) (memory.Memory, error) {
	now := time.Now().Unix()
	m := memory.Memory{
		ID: uuid.NewString(), CompanyID: companyID, Type: mtype, Title: title,
		Content: content, Source: source, Tags: tags, CreatedAt: now, UpdatedAt: now,
	}
	created, err := s.store.CreateMemory(ctx, m)
	if err != nil {
		return memory.Memory{}, err
	}
	_, err = s.audit(ctx, "memory", created.ID, "create", "human:cli", mtype)
	return created, err
}

func (s *Service) ListMemories(ctx context.Context, companyID, mtype string) ([]memory.Memory, error) {
	return s.store.ListMemories(ctx, companyID, mtype)
}

func (s *Service) SearchMemories(ctx context.Context, companyID, q string) ([]memory.Memory, error) {
	return s.store.SearchMemories(ctx, companyID, q)
}
