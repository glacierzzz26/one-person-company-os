package service

import (
	"context"
	"time"

	"github.com/glacierzzz26/one-person-company-os/internal/audit"
	"github.com/glacierzzz26/one-person-company-os/internal/company"
	"github.com/glacierzzz26/one-person-company-os/internal/notify"
	"github.com/glacierzzz26/one-person-company-os/internal/storage/repository"
	"github.com/google/uuid"
)

type Service struct {
	store  *repository.Store
	notify *notify.Notifier // 可选:飞书通知(Phase 6.5);nil = 禁用
}

func New(store *repository.Store) *Service {
	return &Service{store: store}
}

// SetNotifier 注入飞书通知器(env OS_FEISHU_WEBHOOK 为空时传入 nil = 禁用)。
func (s *Service) SetNotifier(n *notify.Notifier) { s.notify = n }

// NotifyEnabled 是否已配置通知。nil 安全。
func (s *Service) NotifyEnabled() bool { return s.notify != nil && s.notify.Enabled() }

// audit 记录一次写操作。所有写操作经由 service 层,自动落 Audit。
func (s *Service) audit(ctx context.Context, entityType, entityID, action, actor, detail string) (audit.Audit, error) {
	return s.store.CreateAudit(ctx, audit.Audit{
		ID: uuid.NewString(), EntityType: entityType, EntityID: entityID,
		Action: action, Actor: actor, Detail: detail, CreatedAt: time.Now().Unix(),
	})
}

func (s *Service) CreateCompany(ctx context.Context, name, vision string) (company.Company, error) {
	now := time.Now().Unix()
	c := company.Company{ID: uuid.NewString(), Name: name, Vision: vision, CreatedAt: now, UpdatedAt: now}
	created, err := s.store.CreateCompany(ctx, c)
	if err != nil {
		return company.Company{}, err
	}
	_, err = s.audit(ctx, "company", created.ID, "create", "human:cli", "")
	return created, err
}

func (s *Service) ListCompanies(ctx context.Context) ([]company.Company, error) {
	return s.store.ListCompanies(ctx)
}

func (s *Service) GetCompany(ctx context.Context, id string) (company.Company, error) {
	return s.store.GetCompany(ctx, id)
}
