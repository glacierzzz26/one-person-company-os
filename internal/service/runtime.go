package service

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/glacierzzz26/one-person-company-os/internal/github"
)

// 运行时旋钮 DB 化(Phase 9.3,契约 runtime-knobs-web.md §3.1;9.4 收口 cli-readonly.md §3.1)。
// 11 个 env 旋钮(方向 config-governance.md §3.1)逐行收编:engine_mode / agent_cli / issue_source
// (+github_token 机密)统一改走 company 覆盖 → global 默认 → 内置默认的生效解析。
// env 只作测试 seam(9.4 决策①反向门控:envSeam 默认关 = 产品恒不读 env,纯 DB;仅 go test 经
// SetEnvSeam(true) 打开)。调用点替换见契约 §三 3.1 表。

// engineScripted 生效 engine 模式判定。env==scripted 仅在测试 seam 打开时生效(envSeam 门,9.4:
// 产品恒关 → 纯 DB);否则 DB 生效值(EngineModeFor:company 覆盖 → global 默认 → 内置 live)。
// 解析失败按 live(与既存 fail-closed 端点语义一致,不动调用点)。Web 只写 live(契约决策③),
// scripted 只可能来自测试 seam/手工 DB。
func (s *Service) engineScripted(ctx context.Context, companyID string) bool {
	if envSeam && strings.EqualFold(os.Getenv("OS_ENGINE_MODE"), "scripted") {
		return true
	}
	m, err := s.EngineModeFor(ctx, companyID)
	if err != nil {
		return false
	}
	return m == "scripted"
}

// agentCLI 生效委派工具族。env seam(OS_AGENT_CLI,与 agentCLIFromEnv 同源)仅在测试 seam 打开时
// 生效(envSeam 门,9.4:产品恒关);否则 company 覆盖 → global 默认 → 内置 claude。
// 合法性留 delegatorFor 判(报错文案见 delegate.go,不泄 env 名)。
func (s *Service) agentCLI(ctx context.Context, companyID string) (string, error) {
	if envSeam {
		if v := strings.TrimSpace(os.Getenv("OS_AGENT_CLI")); v != "" {
			return strings.ToLower(v), nil
		}
	}
	if cs, ok, err := s.CompanySetting(ctx, companyID); err != nil {
		return "", err
	} else if ok && cs.AgentCLI != nil && *cs.AgentCLI != "" {
		return strings.ToLower(strings.TrimSpace(*cs.AgentCLI)), nil
	}
	app, err := s.AppSetting(ctx)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(app.AgentCLIDefault) == "" {
		return agentCLIClaude, nil
	}
	return strings.ToLower(strings.TrimSpace(app.AgentCLIDefault)), nil
}

// issueSourceFor 按公司解析通道 B issue 源(契约 §3.1):company 覆盖 issue_source
// (fixture | github,缺省 github)→ github = 需该公司 secret github_token(fail-closed);
// fixture = 需 company 覆盖 issue_fixture_path。env seam:仅测试 seam 打开时(envSeam 门,9.4:
// 产品恒关)OS_ISSUE_SOURCE 非空 → 原 env 分支(OS_FIXTURE_ISSUES / OS_GITHUB_TOKEN)。
// Phase 10.5 起经 issueSourceForProject 共享解析(company 级同步 = projectID 空)。
func (s *Service) issueSourceFor(ctx context.Context, companyID string) (github.Source, error) {
	return s.issueSourceForProject(ctx, companyID, "")
}

// issueSourceForProject 同 issueSourceFor,但 github 模式读端 token 按项目解析(githubTokenFor:
// 项目级 github_token → 公司回退)。projectID 空 = 纯公司语义(与既有 issueSourceFor 逐字一致);
// SyncRepos 逐 repo / SyncProject 用项目绑仓时传 projectID(契约 github-roundtrip-pr.md §四 C)。
func (s *Service) issueSourceForProject(ctx context.Context, companyID, projectID string) (github.Source, error) {
	if envSeam {
		if v := strings.TrimSpace(os.Getenv("OS_ISSUE_SOURCE")); v != "" {
			return issueSourceFromEnv()
		}
	}
	mode := "github"
	path := ""
	if cs, ok, err := s.CompanySetting(ctx, companyID); err != nil {
		return nil, err
	} else if ok {
		if cs.IssueSource != nil && *cs.IssueSource != "" {
			mode = strings.ToLower(strings.TrimSpace(*cs.IssueSource))
		}
		if cs.IssueFixturePath != nil {
			path = strings.TrimSpace(*cs.IssueFixturePath)
		}
	}
	switch mode {
	case "fixture":
		if path == "" {
			return nil, fmt.Errorf("company %s: issue_source=fixture requires issue_fixture_path", short8(companyID))
		}
		return github.LoadFixture(path)
	case "", "github":
		tok, ok, err := s.githubTokenFor(ctx, companyID, projectID)
		if err != nil {
			return nil, err
		}
		if !ok || tok == "" {
			return nil, fmt.Errorf("company %s: no github_token secret set (add via Web settings /companies/%s/secrets/github_token)", short8(companyID), short8(companyID))
		}
		return github.NewClient(tok), nil
	default:
		return nil, fmt.Errorf("company %s: issue_source must be github|fixture (got %q)", short8(companyID), mode)
	}
}

// githubTokenFor 解析通道 B GitHub 读端 token(Phase 10.5 双层,契约 §一.2):projectID 非空且该
// project_secret 已设 → 项目 token;否则回退公司 secret github_token(公司 token 只读,永不用于 push/PR)。
// 均无 → ok=false(调用方 fail-closed / 报错指引)。主密钥未注入 → 报错(与 OpenSecretCurrent 同)。
func (s *Service) githubTokenFor(ctx context.Context, companyID, projectID string) (string, bool, error) {
	if projectID != "" {
		if tok, ok, err := s.OpenProjectSecretCurrent(ctx, projectID, ProjectSecretGitHubToken); err != nil {
			return "", false, err
		} else if ok && tok != "" {
			return tok, true, nil
		}
	}
	return s.OpenSecretCurrent(ctx, companyID, SecretGitHubToken)
}

// issueSourceFromEnv 按 env 解析 issue 源(原 issueSource();9.4 起仅测试 seam 打开时被调用,产品不可达)。
func issueSourceFromEnv() (github.Source, error) {
	switch mode := strings.ToLower(strings.TrimSpace(os.Getenv("OS_ISSUE_SOURCE"))); mode {
	case "fixture":
		p := os.Getenv("OS_FIXTURE_ISSUES")
		if p == "" {
			return nil, fmt.Errorf("OS_ISSUE_SOURCE=fixture requires OS_FIXTURE_ISSUES=<json path>")
		}
		return github.LoadFixture(p)
	case "", "github":
		tok := os.Getenv("OS_GITHUB_TOKEN")
		if tok == "" {
			return nil, fmt.Errorf("OS_GITHUB_TOKEN is required for GitHub issue sync (offline smoke: OS_ISSUE_SOURCE=fixture + OS_FIXTURE_ISSUES)")
		}
		return github.NewClient(tok), nil
	default:
		return nil, fmt.Errorf("OS_ISSUE_SOURCE must be github|fixture (got %q)", mode)
	}
}
