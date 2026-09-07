package service

// Phase 9.4(契约 cli-readonly.md §五 F1-F4):env 测试 seam 反向门控的产品语义。
// suite 默认经 TestMain 打开 envSeam(true);这些用例把 seam 显式关到产品态(envSeam=false),
// 断言 env 污染(OS_ENGINE_MODE/OS_AGENT_CLI/OS_ISSUE_SOURCE 全被设为与 DB 相反的值)在产品态
// 一律被忽略、纯 DB 生效。与 runtime_test.go A1-A3(seam ON,env 优先)互为正反面。
// withSeamOff 用 t.Cleanup 复位 seam true,防污染同包其它依赖 seam 的用例。

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glacierzzz26/one-person-company-os/internal/github"
	"github.com/glacierzzz26/one-person-company-os/internal/settings"
	"github.com/glacierzzz26/one-person-company-os/internal/storage"
	"github.com/glacierzzz26/one-person-company-os/internal/storage/repository"
)

// withSeamOff 把 seam 关到产品态(恒不读 env),测试结束复位 true(TestMain 开的后台默认)。
func withSeamOff(t *testing.T) {
	t.Helper()
	SetEnvSeam(false)
	t.Cleanup(func() { SetEnvSeam(true) })
}

// F1 engineScripted 产品语义:OS_ENGINE_MODE=scripted 污染也被忽略 → 纯 DB
// (company scripted→true;company live 覆盖 global scripted→false;缺省 live→false;DB 错→false)。
func TestF1EngineScriptedIgnoresEnvSeamOff(t *testing.T) {
	withSeamOff(t)
	svc, st := newSvc(t)
	ctx := context.Background()
	comp := seedCompanyID(t, st)
	comp2 := seedCompanyID(t, st)
	t.Setenv("OS_ENGINE_MODE", "scripted") // 污染:产品态必被忽略

	// 缺省(无行)→ 内置 live;env 说 scripted 也无济于事。
	if svc.engineScripted(ctx, comp) {
		t.Fatal("env scripted polluted but no DB row: want live(false) in product mode")
	}

	// company 覆盖 scripted → true(DB company 权威;只影响该公司)。
	if err := svc.UpsertCompanySetting(ctx, settings.CompanySetting{CompanyID: comp, EngineMode: sp("scripted")}); err != nil {
		t.Fatalf("UpsertCompanySetting scripted: %v", err)
	}
	if !svc.engineScripted(ctx, comp) {
		t.Fatal("company scripted override: want true")
	}
	if svc.engineScripted(ctx, comp2) {
		t.Fatal("other company no override: want live(false)")
	}

	// company live 覆盖 + global scripted:company 优先 → false(即使 env 与 global 都 scripted)。
	def := settings.DefaultAppSetting()
	def.EngineModeDefault = "scripted"
	if err := svc.UpsertAppSetting(ctx, def); err != nil {
		t.Fatalf("UpsertAppSetting global scripted: %v", err)
	}
	if err := svc.UpsertCompanySetting(ctx, settings.CompanySetting{CompanyID: comp, EngineMode: sp("live")}); err != nil {
		t.Fatalf("UpsertCompanySetting live: %v", err)
	}
	if svc.engineScripted(ctx, comp) {
		t.Fatal("company live override over global+env scripted: want false")
	}
	if !svc.engineScripted(ctx, comp2) {
		t.Fatal("global scripted inherited (no company override): want true")
	}

	// DB 解析错误 → false(live 兜底);env 污染不干扰 fail-safe。
	db, err := storage.Open(filepath.Join(t.TempDir(), "os-closed.db"))
	if err != nil {
		t.Fatalf("open closed db: %v", err)
	}
	broken := New(repository.NewStore(db))
	db.Close()
	if broken.engineScripted(ctx, comp) {
		t.Fatal("db error should resolve as live(false)")
	}
}

// F2 agentCLI 产品语义:OS_AGENT_CLI=codex 污染被忽略 → company 覆盖 → global 默认 → 内置 claude。
func TestF2AgentCLIIgnoresEnvSeamOff(t *testing.T) {
	withSeamOff(t)
	svc, st := newSvc(t)
	ctx := context.Background()
	compA := seedCompanyID(t, st)
	compB := seedCompanyID(t, st)
	t.Setenv("OS_AGENT_CLI", "codex") // 污染:产品态必被忽略

	// 无行 → 内置 claude(env codex 无效)。
	if got, err := svc.agentCLI(ctx, compA); err != nil || got != "claude" {
		t.Fatalf("no rows agentCLI = %q, %v; want claude", got, err)
	}

	// company claude + global codex → company claude 胜(env codex 污染被彻底无视的判据)。
	if err := svc.UpsertCompanySetting(ctx, settings.CompanySetting{CompanyID: compA, AgentCLI: sp("claude")}); err != nil {
		t.Fatalf("UpsertCompanySetting claude: %v", err)
	}
	def := settings.DefaultAppSetting()
	def.AgentCLIDefault = "codex"
	if err := svc.UpsertAppSetting(ctx, def); err != nil {
		t.Fatalf("UpsertAppSetting global codex: %v", err)
	}
	if got, err := svc.agentCLI(ctx, compA); err != nil || got != "claude" {
		t.Fatalf("company claude over global codex = %q, %v; want claude", got, err)
	}
	// 无 company 覆盖 → 继承 global codex。
	if got, err := svc.agentCLI(ctx, compB); err != nil || got != "codex" {
		t.Fatalf("global default codex agentCLI = %q, %v; want codex", got, err)
	}
}

// F3 issueSourceFor 产品语义:OS_ISSUE_SOURCE=fixture(+有效 OS_FIXTURE_ISSUES)污染被忽略 →
// 纯 company 配置解析(github 无 secret → fail-closed no github_token;fixture+path → LoadFixture)。
func TestF3IssueSourceForIgnoresEnvSeamOff(t *testing.T) {
	withSeamOff(t)
	svc, st := newSvc(t)
	ctx := context.Background()
	comp := seedCompanyID(t, st)
	k := key32()
	settings.UseMasterKey(k)
	t.Cleanup(func() { settings.UseMasterKey(nil) })
	fx := writeIssueFixture(t, `[]`)
	t.Setenv("OS_ISSUE_SOURCE", "fixture") // 污染:产品态必被忽略
	t.Setenv("OS_FIXTURE_ISSUES", fx)      // env fixture 有效路径也无效

	// 缺省(company 无覆盖)= github;无 github_token secret → fail-closed 报 github_token。
	// (若 env 分支被误触发 → 会返回 *github.Fixture 无错 → 本断言失败,即判据。)
	if _, err := svc.issueSourceFor(ctx, comp); err == nil || !strings.Contains(err.Error(), "github_token") {
		t.Fatalf("default github w/o token: err=%v; want fail-closed github_token (env fixture ignored)", err)
	}

	// company fixture + path → LoadFixture(*github.Fixture)(DB 权威,不靠 env)。
	if err := svc.UpsertCompanySetting(ctx, settings.CompanySetting{CompanyID: comp, IssueSource: sp("fixture"), IssueFixturePath: sp(fx)}); err != nil {
		t.Fatalf("UpsertCompanySetting fixture: %v", err)
	}
	src, err := svc.issueSourceFor(ctx, comp)
	if err != nil {
		t.Fatalf("company fixture with path: %v", err)
	}
	if _, ok := src.(*github.Fixture); !ok {
		t.Fatalf("fixture source type = %T; want *github.Fixture", src)
	}
}

// F4 agentCLIFromEnv 产品语义:envSeam 关 → OS_AGENT_CLI 被忽略,固定回落 claude。
func TestF4AgentCLIFromEnvIgnoresEnvSeamOff(t *testing.T) {
	withSeamOff(t)
	t.Setenv("OS_AGENT_CLI", "codex") // 污染
	if got := agentCLIFromEnv(); got != agentCLIClaude {
		t.Fatalf("agentCLIFromEnv (seam off) = %q; want %q", got, agentCLIClaude)
	}
}

// F4+ seam 开对照:环境变量残留不影响本文件;agentCLIFromEnv 在 seam ON 时仍照旧(与既有 A 用例一致)。
func TestF4AgentCLIFromEnvSeamOnUnchanged(t *testing.T) {
	// TestMain 已开 seam;此处不关。清掉残留后默认 claude。
	t.Setenv("OS_AGENT_CLI", "")
	if got := agentCLIFromEnv(); got != agentCLIClaude {
		t.Fatalf("agentCLIFromEnv empty env = %q; want claude", got)
	}
	t.Setenv("OS_AGENT_CLI", "codex")
	if got := agentCLIFromEnv(); got != agentCLICodex {
		t.Fatalf("agentCLIFromEnv codex env (seam on) = %q; want codex", got)
	}
}
