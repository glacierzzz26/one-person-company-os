package service

import (
	"context"

	osrepo "github.com/glacierzzz26/one-person-company-os/internal/repo"
)

// 代码源(Phase 6.3 研发仓库 → Phase 10 D7 收敛为项目 code 源绑定)。
// 手工登记口(AddRepo/AddRepoAs/ListRepos)已随「登记入口去掉」移除:
// 代码源由建项目自动认领(ensureCodeSource/RefreshProjectCodeSourceAs),sync 遍历 = 项目 derived + legacy。

// ListAllRepos 全部公司代码源(server 轮询 / webhook 归属解析 / os intake sync 不带 --company)。
func (s *Service) ListAllRepos(ctx context.Context) ([]osrepo.Repo, error) {
	return s.store.ListAllRepos(ctx)
}
