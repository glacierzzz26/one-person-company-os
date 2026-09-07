package service

// Phase 9.3 — 运行时旋钮 DB 化(契约 runtime-knobs-web.md §五 A1-A3)。
// 方法与 env seam 均 unexported/同包直测;env 经 t.Setenv 显式控制(空串=清),防宿主残留。
// settings 主密钥 holder 是进程级状态:注入后用 t.Cleanup 复位,防污染同包其它测试。

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glacierzzz26/one-person-company-os/internal/github"
	"github.com/glacierzzz26/one-person-company-os/internal/settings"
	"github.com/glacierzzz26/one-person-company-os/internal/storage"
	"github.com/glacierzzz26/one-person-company-os/internal/storage/repository"
)

// A1 engineScripted:D3 env seam 优先(==scripted→true);否则 DB 生效值
// (company 覆盖 → global 默认 → 内置 live → false);DB 解析错误按 live(false)。
func TestA1EngineScripted(t *testing.T) {
	svc, st := newSvc(t)
	ctx := context.Background()
	comp := seedCompanyID(t, st)
	comp2 := seedCompanyID(t, st)
	t.Setenv("OS_ENGINE_MODE", "") // 清 seam,走 DB 解析

	// 缺省(无 company/global 行)→ 内置 live → false。
	if svc.engineScripted(ctx, comp) {
		t.Fatal("no rows: want live(false)")
	}

	// env seam:OS_ENGINE_MODE=scripted → true,无视 DB。
	t.Setenv("OS_ENGINE_MODE", "scripted")
	if !svc.engineScripted(ctx, comp) {
		t.Fatal("env scripted seam: want true")
	}

	// 清 env,company 覆盖 scripted → true(只影响该公司)。
	t.Setenv("OS_ENGINE_MODE", "")
	if err := svc.UpsertCompanySetting(ctx, settings.CompanySetting{CompanyID: comp, EngineMode: sp("scripted")}); err != nil {
		t.Fatalf("UpsertCompanySetting scripted: %v", err)
	}
	if !svc.engineScripted(ctx, comp) {
		t.Fatal("company override scripted: want true")
	}
	if svc.engineScripted(ctx, comp2) {
		t.Fatal("other company no override: want live(false)")
	}

	// company 覆盖 live + global scripted → company 优先 false。
	def := settings.DefaultAppSetting()
	def.EngineModeDefault = "scripted"
	if err := svc.UpsertAppSetting(ctx, def); err != nil {
		t.Fatalf("UpsertAppSetting global scripted: %v", err)
	}
	if err := svc.UpsertCompanySetting(ctx, settings.CompanySetting{CompanyID: comp, EngineMode: sp("live")}); err != nil {
		t.Fatalf("UpsertCompanySetting live: %v", err)
	}
	if svc.engineScripted(ctx, comp) {
		t.Fatal("company live override over global scripted: want false")
	}
	if !svc.engineScripted(ctx, comp2) {
		t.Fatal("global scripted inherited (no override): want true")
	}

	// DB 解析错误(store 打不开)→ false(live 兜底,不 panic)。
	db, err := storage.Open(filepath.Join(t.TempDir(), "os-closed.db"))
	if err != nil {
		t.Fatalf("open closed db: %v", err)
	}
	broken := New(repository.NewStore(db))
	db.Close() // 关库后 GetCompanySetting 必错
	t.Setenv("OS_ENGINE_MODE", "")
	if broken.engineScripted(ctx, comp) {
		t.Fatal("db error should resolve as live(false)")
	}
}

// A2 agentCLI:env seam 优先 → company 覆盖 → global 默认 → 内置 claude。
func TestA2AgentCLI(t *testing.T) {
	svc, st := newSvc(t)
	ctx := context.Background()
	compA := seedCompanyID(t, st)
	compB := seedCompanyID(t, st)
	t.Setenv("OS_AGENT_CLI", "") // 清 seam

	// 无任何行 → 内置 claude。
	if got, err := svc.agentCLI(ctx, compA); err != nil || got != "claude" {
		t.Fatalf("no rows agentCLI = %q, %v; want claude", got, err)
	}

	// company 覆盖 codex → 该公司 codex,别家仍 claude。
	if err := svc.UpsertCompanySetting(ctx, settings.CompanySetting{CompanyID: compA, AgentCLI: sp("codex")}); err != nil {
		t.Fatalf("UpsertCompanySetting codex: %v", err)
	}
	if got, _ := svc.agentCLI(ctx, compA); got != "codex" {
		t.Fatalf("company codex agentCLI = %q; want codex", got)
	}
	if got, _ := svc.agentCLI(ctx, compB); got != "claude" {
		t.Fatalf("other company agentCLI = %q; want claude", got)
	}

	// global 默认 codex → 无覆盖公司继承 codex。
	def := settings.DefaultAppSetting()
	def.AgentCLIDefault = "codex"
	if err := svc.UpsertAppSetting(ctx, def); err != nil {
		t.Fatalf("UpsertAppSetting codex: %v", err)
	}
	if got, _ := svc.agentCLI(ctx, compB); got != "codex" {
		t.Fatalf("global default codex agentCLI = %q; want codex", got)
	}

	// env seam:OS_AGENT_CLI=codex 覆盖全部 DB(company claude 也被盖)。
	if err := svc.UpsertCompanySetting(ctx, settings.CompanySetting{CompanyID: compA, AgentCLI: sp("claude")}); err != nil {
		t.Fatalf("UpsertCompanySetting claude: %v", err)
	}
	t.Setenv("OS_AGENT_CLI", "codex")
	if got, _ := svc.agentCLI(ctx, compA); got != "codex" {
		t.Fatalf("env codex seam agentCLI = %q; want codex", got)
	}
}

// writeIssueFixture 落一个 issue fixture json(空数组即可满足 LoadFixture 形状)。
func writeIssueFixture(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "issues.json")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write issue fixture: %v", err)
	}
	return p
}

// A3 issueSourceFor:company github+fail-closed 无 token 报错;fixture 需路径 → LoadFixture;
// github 配 secret → 构造成功;env seam(OS_ISSUE_SOURCE)保持旧行为。
func TestA3IssueSourceFor(t *testing.T) {
	svc, st := newSvc(t)
	ctx := context.Background()
	comp := seedCompanyID(t, st)
	k := key32()
	settings.UseMasterKey(k)
	t.Cleanup(func() { settings.UseMasterKey(nil) })
	t.Setenv("OS_ISSUE_SOURCE", "") // 清 seam,走 DB 解析

	// 缺省(无 company 覆盖)= github;无 github_token secret → fail-closed 报错。
	if _, err := svc.issueSourceFor(ctx, comp); err == nil || !strings.Contains(err.Error(), "github_token") {
		t.Fatalf("default github w/o token: err=%v; want fail-closed github_token 提示", err)
	}

	// company 覆盖 github + 配了 github_token secret → 构造成功(*github.Client)。
	if err := svc.SetCompanySecretAs(ctx, comp, SecretGitHubToken, "ghp_dummy", "human:console"); err != nil {
		t.Fatalf("SetCompanySecretAs github_token: %v", err)
	}
	if err := svc.UpsertCompanySetting(ctx, settings.CompanySetting{CompanyID: comp, IssueSource: sp("github")}); err != nil {
		t.Fatalf("UpsertCompanySetting github: %v", err)
	}
	src, err := svc.issueSourceFor(ctx, comp)
	if err != nil {
		t.Fatalf("github with token: %v", err)
	}
	if _, ok := src.(*github.Client); !ok {
		t.Fatalf("github source type = %T; want *github.Client", src)
	}

	// fixture + 无路径 → 报错要求 path。
	if err := svc.UpsertCompanySetting(ctx, settings.CompanySetting{CompanyID: comp, IssueSource: sp("fixture")}); err != nil {
		t.Fatalf("UpsertCompanySetting fixture(无路径): %v", err)
	}
	if _, err := svc.issueSourceFor(ctx, comp); err == nil || !strings.Contains(err.Error(), "issue_fixture_path") {
		t.Fatalf("fixture w/o path: err=%v; want requires issue_fixture_path", err)
	}

	// fixture + 路径 → LoadFixture 成功(*github.Fixture)。
	fx := writeIssueFixture(t, `[]`)
	if err := svc.UpsertCompanySetting(ctx, settings.CompanySetting{CompanyID: comp, IssueSource: sp("fixture"), IssueFixturePath: sp(fx)}); err != nil {
		t.Fatalf("UpsertCompanySetting fixture(带路径): %v", err)
	}
	src, err = svc.issueSourceFor(ctx, comp)
	if err != nil {
		t.Fatalf("fixture with path: %v", err)
	}
	if _, ok := src.(*github.Fixture); !ok {
		t.Fatalf("fixture source type = %T; want *github.Fixture", src)
	}

	// env seam:OS_ISSUE_SOURCE=fixture + OS_FIXTURE_ISSUES → 无 company 行也保持旧行为。
	t.Setenv("OS_ISSUE_SOURCE", "fixture")
	t.Setenv("OS_FIXTURE_ISSUES", fx)
	if src, err := svc.issueSourceFor(ctx, comp); err != nil {
		t.Fatalf("env fixture seam: %v", err)
	} else if _, ok := src.(*github.Fixture); !ok {
		t.Fatalf("env fixture source type = %T; want *github.Fixture", src)
	}
}
