package service

import (
	"context"
	"time"

	"github.com/glacierzzz26/one-person-company-os/internal/audit"
	"github.com/glacierzzz26/one-person-company-os/internal/company"
	"github.com/glacierzzz26/one-person-company-os/internal/github"
	"github.com/glacierzzz26/one-person-company-os/internal/storage/repository"
	"github.com/google/uuid"
)

type Service struct {
	store *repository.Store
	// delegator 执行委派工具(Phase 8.2 修订 B:writer live = 委派集成 agent CLI)。
	// 默认 claudeDelegator;测试注入 fake(记录 spec、向 workspace 落文件、返回 canned report)。
	delegator Delegator
	// prPub 项目收尾 PR 发布器(Phase 10.5,契约 github-roundtrip-pr.md §四 D)。nil →
	// publishRunPR 按项目写 token 实时构建真实 github.Client(PR 创建 + default_branch 兜底);
	// 测试注入 fake(记录 CreatePull 参,不触网)。git push 不经它,走 gitAuthEnv 宿主侧。
	prPub github.PullPublisher
}

func New(store *repository.Store) *Service {
	return &Service{store: store, delegator: &claudeDelegator{}}
}

// (9.3 起不再有进程级 notify 单例:通知按任务归属公司机密 feishu_webhook 解析,见 notifyCompany;
// SetNotifier/NotifyEnabled 已删除,通知源不再读 env OS_FEISHU_*。)

// audit 记录一次写操作。所有写操作经由 service 层,自动落 Audit。
func (s *Service) audit(ctx context.Context, entityType, entityID, action, actor, detail string) (audit.Audit, error) {
	return s.store.CreateAudit(ctx, audit.Audit{
		ID: uuid.NewString(), EntityType: entityType, EntityID: entityID,
		Action: action, Actor: actor, Detail: detail, CreatedAt: time.Now().Unix(),
	})
}

func (s *Service) CreateCompany(ctx context.Context, name, vision string) (company.Company, error) {
	return s.CreateCompanyAs(ctx, name, vision, "human:cli")
}

// CreateCompanyAs 同 CreateCompany,审计 actor 用传入值(如 human:console)。
func (s *Service) CreateCompanyAs(ctx context.Context, name, vision, actor string) (company.Company, error) {
	now := time.Now().Unix()
	c := company.Company{ID: uuid.NewString(), Name: name, Vision: vision, CreatedAt: now, UpdatedAt: now}
	created, err := s.store.CreateCompany(ctx, c)
	if err != nil {
		return company.Company{}, err
	}
	_, err = s.audit(ctx, "company", created.ID, "create", actor, "")
	return created, err
}

func (s *Service) ListCompanies(ctx context.Context) ([]company.Company, error) {
	return s.store.ListCompanies(ctx)
}

func (s *Service) GetCompany(ctx context.Context, id string) (company.Company, error) {
	return s.store.GetCompany(ctx, id)
}
