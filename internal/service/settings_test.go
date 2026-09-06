package service

import (
	"context"
	"database/sql"
	"encoding/hex"
	"path/filepath"
	"testing"
	"time"

	"github.com/glacierzzz26/one-person-company-os/internal/endpoint"
	"github.com/glacierzzz26/one-person-company-os/internal/settings"
	"github.com/glacierzzz26/one-person-company-os/internal/storage"
	"github.com/google/uuid"
)

func sp(v string) *string { return &v }

func key32() []byte { return []byte("0123456789abcdef0123456789abcdef") }

// A1 迁移:storage.Open(tmp) 应用 0012;三表存在、默认列回填正确。
func TestA1ConfigMigrationTables(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "os-a1.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = 12`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("schema_migrations version=12 count = %d, %v; want 1", n, err)
	}
	for _, tb := range []string{"app_setting", "company_setting", "secret"} {
		var c int
		if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name = ?`, tb).Scan(&c); err != nil || c != 1 {
			t.Fatalf("table %s count = %d, %v; want 1", tb, c, err)
		}
	}
	// 默认列(app_setting:engine_mode_default 'live' / http_port 8787 / console_token_hash '')。
	def := map[string]string{}
	rows, err := db.Query(`PRAGMA table_info(app_setting)`)
	if err != nil {
		t.Fatalf("table_info: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notnull, pk int
		var name, typ string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			t.Fatalf("scan: %v", err)
		}
		def[name] = dflt.String
	}
	want := map[string]string{"engine_mode_default": "'live'", "http_port": "8787", "console_token_hash": "''", "queue_work": "0"}
	for col, d := range want {
		if def[col] != d {
			t.Fatalf("app_setting.%s default = %q, want %q", col, def[col], d)
		}
	}
}

// A2 repo/service 全链:AppSetting/CompanySetting/Secret 往返 + 隔离/删除;SetEndpointToken 写回回读。
func TestA2SettingsRoundtrip(t *testing.T) {
	svc, st := newSvc(t)
	ctx := context.Background()
	compA := seedCompanyID(t, st)
	compB := seedCompanyID(t, st)

	// 缺行 → 内置默认。
	a0, err := svc.AppSetting(ctx)
	if err != nil {
		t.Fatalf("AppSetting missing: %v", err)
	}
	if a0.EngineModeDefault != settings.DefaultEngineMode || a0.HTTPPort != settings.DefaultHTTPPort {
		t.Fatalf("default AppSetting = %+v", a0)
	}

	// Upsert → 回读。
	override := a0
	override.EngineModeDefault = "scripted"
	override.HTTPPort = 9001
	override.QueueWork = true
	if err := svc.UpsertAppSetting(ctx, override); err != nil {
		t.Fatalf("UpsertAppSetting: %v", err)
	}
	a1, err := svc.AppSetting(ctx)
	if err != nil {
		t.Fatalf("AppSetting after upsert: %v", err)
	}
	if a1.EngineModeDefault != "scripted" || a1.HTTPPort != 9001 || !a1.QueueWork || a1.UpdatedAt == 0 {
		t.Fatalf("AppSetting after upsert = %+v", a1)
	}

	// CompanySetting:缺行 ok=false。
	if _, ok, err := svc.CompanySetting(ctx, compA); err != nil || ok {
		t.Fatalf("CompanySetting missing: ok=%v, %v; want false,nil", ok, err)
	}
	// 插入覆盖行 → 回读(EngineMode 覆盖,AgentCLI 继承)。
	if err := svc.UpsertCompanySetting(ctx, settings.CompanySetting{CompanyID: compA, EngineMode: sp("scripted")}); err != nil {
		t.Fatalf("UpsertCompanySetting insert: %v", err)
	}
	cs, ok, err := svc.CompanySetting(ctx, compA)
	if err != nil || !ok || cs.EngineMode == nil || *cs.EngineMode != "scripted" || cs.AgentCLI != nil {
		t.Fatalf("CompanySetting insert = ok:%v %+v err:%v", ok, cs, err)
	}
	// 更新路径(EngineMode 回退继承,AgentCLI 设值)→ 覆盖原行。
	if err := svc.UpsertCompanySetting(ctx, settings.CompanySetting{CompanyID: compA, AgentCLI: sp("codex")}); err != nil {
		t.Fatalf("UpsertCompanySetting update: %v", err)
	}
	cs, ok, err = svc.CompanySetting(ctx, compA)
	if err != nil || !ok || cs.EngineMode != nil || cs.AgentCLI == nil || *cs.AgentCLI != "codex" {
		t.Fatalf("CompanySetting update = ok:%v %+v err:%v", ok, cs, err)
	}
	// 删行 → 回退继承。
	if err := svc.DeleteCompanySetting(ctx, compA); err != nil {
		t.Fatalf("DeleteCompanySetting: %v", err)
	}
	if _, ok, err := svc.CompanySetting(ctx, compA); err != nil || ok {
		t.Fatalf("CompanySetting after delete: ok=%v, %v; want false,nil", ok, err)
	}

	// Secret 按 company 隔离:同 id 两公司各自明文。
	key := key32()
	if err := svc.SetSecret(ctx, compA, "github_token", "token-A", key); err != nil {
		t.Fatalf("SetSecret A: %v", err)
	}
	if err := svc.SetSecret(ctx, compB, "github_token", "token-B", key); err != nil {
		t.Fatalf("SetSecret B: %v", err)
	}
	if p, ok, err := svc.OpenSecret(ctx, compA, "github_token", key); err != nil || !ok || p != "token-A" {
		t.Fatalf("OpenSecret A = ok:%v %q err:%v", ok, p, err)
	}
	if p, ok, err := svc.OpenSecret(ctx, compB, "github_token", key); err != nil || !ok || p != "token-B" {
		t.Fatalf("OpenSecret B = ok:%v %q err:%v", ok, p, err)
	}
	// 更新路径 + List 隔离。
	if err := svc.SetSecret(ctx, compA, "github_token", "token-A2", key); err != nil {
		t.Fatalf("SetSecret A update: %v", err)
	}
	secretsA, err := svc.store.ListSecrets(ctx, compA)
	if err != nil || len(secretsA) != 1 || secretsA[0].Cipher == "token-A2" {
		t.Fatalf("ListSecrets A = %+v err:%v; want 1 row, cipher not plaintext", secretsA, err)
	}
	if p, _, _ := svc.OpenSecret(ctx, compA, "github_token", key); p != "token-A2" {
		t.Fatalf("OpenSecret A after update = %q", p)
	}
	// 删除 → A 无行,B 仍在。
	if err := svc.store.DeleteSecret(ctx, compA, "github_token"); err != nil {
		t.Fatalf("DeleteSecret A: %v", err)
	}
	if _, ok, err := svc.OpenSecret(ctx, compA, "github_token", key); err != nil || ok {
		t.Fatalf("OpenSecret A after delete: ok=%v, %v", ok, err)
	}
	if p, ok, _ := svc.OpenSecret(ctx, compB, "github_token", key); !ok || p != "token-B" {
		t.Fatalf("OpenSecret B isolated = ok:%v %q", ok, p)
	}

	// SetEndpointToken 改 cipher 后回读新值。
	now := time.Now().Unix()
	ep, err := st.CreateEndpoint(ctx, endpoint.Endpoint{
		ID: uuid.NewString(), CompanyID: compA, Name: "judge", BaseURL: "http://localhost:1/v1",
		TokenEnc: "cipher-old", Proto: "openai", Vendor: "openai", Role: "pool", Tier: "standard",
		Status: "active", CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("CreateEndpoint: %v", err)
	}
	if _, err := st.SetEndpointToken(ctx, ep.ID, "cipher-new"); err != nil {
		t.Fatalf("SetEndpointToken: %v", err)
	}
	got, err := st.GetEndpoint(ctx, ep.ID)
	if err != nil || got.TokenEnc != "cipher-new" {
		t.Fatalf("GetEndpoint after token set = token_enc:%q err:%v; want cipher-new", got.TokenEnc, err)
	}
}

// C1 生效解析:company 覆盖 → global 默认 → 内置 "live"。
func TestC1EngineModeResolution(t *testing.T) {
	svc, st := newSvc(t)
	ctx := context.Background()
	comp := seedCompanyID(t, st)
	comp2 := seedCompanyID(t, st)

	if m, err := svc.EngineModeFor(ctx, comp); err != nil || m != "live" {
		t.Fatalf("no rows → EngineModeFor = %q, %v; want live", m, err)
	}

	// 全局默认改 scripted → 无覆盖公司继承。
	a := settings.DefaultAppSetting()
	a.EngineModeDefault = "scripted"
	if err := svc.UpsertAppSetting(ctx, a); err != nil {
		t.Fatalf("UpsertAppSetting: %v", err)
	}
	for _, c := range []string{comp, comp2} {
		if m, err := svc.EngineModeFor(ctx, c); err != nil || m != "scripted" {
			t.Fatalf("global scripted, company %s = %q, %v; want scripted", c, m, err)
		}
	}

	// company 覆盖 live → 优先。
	if err := svc.UpsertCompanySetting(ctx, settings.CompanySetting{CompanyID: comp, EngineMode: sp("live")}); err != nil {
		t.Fatalf("UpsertCompanySetting: %v", err)
	}
	if m, _ := svc.EngineModeFor(ctx, comp); m != "live" {
		t.Fatalf("company override live → %q; want live", m)
	}
	if m, _ := svc.EngineModeFor(ctx, comp2); m != "scripted" {
		t.Fatalf("no-override company → %q; want scripted", m)
	}

	// 覆盖行删掉 → 回落全局。
	if err := svc.DeleteCompanySetting(ctx, comp); err != nil {
		t.Fatalf("DeleteCompanySetting: %v", err)
	}
	if m, _ := svc.EngineModeFor(ctx, comp); m != "scripted" {
		t.Fatalf("after delete company → %q; want scripted", m)
	}
}

// C2 re-key:enc:v1 端点 token 从 oldKey 换到 newKey,两遍语义,失败路径零写回。
func TestC2RekeyEndpointTokens(t *testing.T) {
	oldHex := "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"
	t.Setenv("OS_ENDPOINT_KEY", oldHex)
	oldKey, err := hex.DecodeString(oldHex)
	if err != nil {
		t.Fatalf("decode old key: %v", err)
	}
	newKey := []byte("fedcba9876543210fedcba9876543210")
	ctx := context.Background()

	svc, st := newSvc(t)
	comp := seedCompanyID(t, st)
	// 带 token 端点(env key 加密)+ 空 token 端点(应被跳过)。
	epTok, err := svc.AddEndpointAs(ctx, comp, "judge", "http://localhost:1/v1", "plain-tok", "openai", "human:cli")
	if err != nil {
		t.Fatalf("AddEndpoint: %v", err)
	}
	now := time.Now().Unix()
	if _, err := st.CreateEndpoint(ctx, endpoint.Endpoint{
		ID: uuid.NewString(), CompanyID: comp, Name: "noauth", BaseURL: "http://localhost:2/v1",
		TokenEnc: "", Proto: "openai", Vendor: "openai", Role: "pool", Tier: "standard",
		Status: "active", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("CreateEndpoint noauth: %v", err)
	}

	n, err := svc.RekeyEndpointTokens(ctx, oldKey, newKey)
	if err != nil {
		t.Fatalf("RekeyEndpointTokens: %v", err)
	}
	if n != 1 {
		t.Fatalf("rekeyed count = %d, want 1 (空 token 端点跳过)", n)
	}
	got, err := st.GetEndpoint(ctx, epTok.ID)
	if err != nil {
		t.Fatalf("GetEndpoint: %v", err)
	}
	if p, err := endpoint.OpenTokenWith(newKey, got.TokenEnc); err != nil || p != "plain-tok" {
		t.Fatalf("open with new key = %q, %v; want plain-tok", p, err)
	}
	if _, err := endpoint.OpenTokenWith(oldKey, got.TokenEnc); err == nil {
		t.Fatal("open with old key should fail after re-key")
	}
}

// B2 console 令牌:SetConsoleTokenAs 哈希落库(明文不入库、保留其余字段)+ 空串拒绝 + audit。
func TestB2ConsoleTokenSet(t *testing.T) {
	svc, st := newSvc(t)
	ctx := context.Background()

	// 未初始化 → hash ''。
	if h, err := svc.ConsoleTokenHash(ctx); err != nil || h != "" {
		t.Fatalf("ConsoleTokenHash init = %q, %v; want ''", h, err)
	}
	// 空串拒绝(不落库)。
	if err := svc.SetConsoleTokenAs(ctx, "", "human:console"); err == nil {
		t.Fatal("SetConsoleTokenAs empty: want error")
	}
	if h, _ := svc.ConsoleTokenHash(ctx); h != "" {
		t.Fatalf("ConsoleTokenHash after empty reject = %q; want ''", h)
	}

	if err := svc.SetConsoleToken(ctx, "tok-abc"); err != nil {
		t.Fatalf("SetConsoleToken: %v", err)
	}
	h, err := svc.ConsoleTokenHash(ctx)
	if err != nil {
		t.Fatalf("ConsoleTokenHash: %v", err)
	}
	if want := settings.HashConsoleToken("tok-abc"); h != want {
		t.Fatalf("ConsoleTokenHash = %q, want %q", h, want)
	}
	if h == "tok-abc" {
		t.Fatal("console token stored in plaintext")
	}
	// Upsert 保留其余字段(不是只写哈希的零值行)。
	a, err := svc.AppSetting(ctx)
	if err != nil || a.EngineModeDefault != settings.DefaultEngineMode {
		t.Fatalf("AppSetting after token set = %+v err:%v; engine default should be preserved", a, err)
	}
	// audit 落一条 settings/global/set-console-token。
	ok, err := st.HasAuditAction(ctx, "settings", "global", "set-console-token")
	if err != nil || !ok {
		t.Fatalf("audit set-console-token = ok:%v err:%v; want recorded", ok, err)
	}
}

// C2 失败路径:错 old key → error 且零写回(端点 token 仍是 oldKey 可解)。
func TestC2RekeyFailureZeroWrite(t *testing.T) {
	oldHex := "112233445566778899aabbccddeeff00112233445566778899aabbccddeeff00"
	t.Setenv("OS_ENDPOINT_KEY", oldHex)
	oldKey, _ := hex.DecodeString(oldHex)
	ctx := context.Background()

	svc, st := newSvc(t)
	comp := seedCompanyID(t, st)
	ep, err := svc.AddEndpointAs(ctx, comp, "judge", "http://localhost:1/v1", "still-tok", "openai", "human:cli")
	if err != nil {
		t.Fatalf("AddEndpoint: %v", err)
	}

	wrongKey := []byte("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if _, err := svc.RekeyEndpointTokens(ctx, wrongKey, []byte("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")); err == nil {
		t.Fatal("rekey with wrong old key: want error")
	}
	got, err := st.GetEndpoint(ctx, ep.ID)
	if err != nil {
		t.Fatalf("GetEndpoint: %v", err)
	}
	if p, err := endpoint.OpenTokenWith(oldKey, got.TokenEnc); err != nil || p != "still-tok" {
		t.Fatalf("token should be untouched (zero write): open old = %q, %v", p, err)
	}
}
