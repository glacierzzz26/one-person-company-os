package server

// Phase 9.3 — 全量设置 API(契约 runtime-knobs-web.md §五 E1-E4)。
// 走真 svc + /setup 初始化(注入主密钥 holder)→ Bearer console 令牌请求。
// E5(已有 auth 回归:空 hash 开放 / bearer 401+200)由 setup_test.go 覆盖,本文件不重复。

import (
	"context"
	"encoding/hex"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/glacierzzz26/one-person-company-os/internal/endpoint"
	"github.com/glacierzzz26/one-person-company-os/internal/service"
	"github.com/glacierzzz26/one-person-company-os/internal/settings"
)

// setupConsole 完成 /setup 首启并返回可用 console 令牌。
func setupConsole(t *testing.T, h http.Handler) string {
	t.Helper()
	if rec, env := doAPI(t, h, http.MethodPost, "/api/v1/setup", `{"console_token":"sekret-token"}`); rec.Code != http.StatusOK || !env.OK {
		t.Fatalf("setup: code=%d env=%+v body=%s", rec.Code, env, rec.Body.String())
	}
	return "sekret-token"
}

// E1 GET/PUT /settings:GET 默认生效行;PUT 部分更新即生效且**保留 console_token_hash**(guard);
// engine_mode_default=scripted → 400;空 patch → 400;digest 格式错 → 400。
func TestE1GetPutGlobalSettings(t *testing.T) {
	srv, _ := newSetupServer(t)
	t.Cleanup(func() { settings.UseMasterKey(nil) })
	h := srv.Handler()
	tok := setupConsole(t, h)

	// GET 默认(缺行 → 内置默认;console_token_set=true,无 hash 字段)。
	rec, env := doBearer(t, h, http.MethodGet, "/api/v1/settings", tok, "")
	if rec.Code != http.StatusOK || !env.OK {
		t.Fatalf("GET settings: code=%d env=%+v", rec.Code, env)
	}
	if strings.Contains(rec.Body.String(), "console_token_hash") {
		t.Fatalf("GET settings must never return console_token_hash:\n%s", rec.Body.String())
	}
	var gs globalSettingsDTO
	decodeData(t, env, &gs)
	if gs.EngineModeDefault != "live" || gs.AgentCLIDefault != "claude" || gs.DigestTime != "09:00" ||
		gs.HTTPPort != 8787 || gs.PollMin != 5 || gs.QueueWork || gs.QueueIntervalSec != 10 ||
		gs.SchedulePollSec != 0 || !gs.ConsoleTokenSet {
		t.Fatalf("GET settings default = %+v", gs)
	}

	// PUT 部分更新(请求不带 console 相关字段)→ 生效 + 令牌仍有效(hash 保留,红线 guard)。
	// 10.2:schedule_poll_sec 一并收 + 回包回显(Web 设置行读它;DGO 防只写不回)。
	rec, env = doBearer(t, h, http.MethodPut, "/api/v1/settings", tok,
		`{"agent_cli_default":"codex","http_port":8899,"queue_work":true,"digest_time":"off","schedule_poll_sec":60}`)
	if rec.Code != http.StatusOK || !env.OK {
		t.Fatalf("PUT settings: code=%d env=%+v body=%s", rec.Code, env, rec.Body.String())
	}
	decodeData(t, env, &gs)
	if gs.AgentCLIDefault != "codex" || gs.HTTPPort != 8899 || !gs.QueueWork || gs.DigestTime != "" ||
		gs.EngineModeDefault != "live" || gs.SchedulePollSec != 60 || !gs.ConsoleTokenSet {
		t.Fatalf("PUT settings result = %+v", gs)
	}
	if rec, _ := doBearer(t, h, http.MethodGet, "/api/v1/companies", tok, ""); rec.Code != http.StatusOK {
		t.Fatalf("console token after PUT /settings: code=%d, want 200 (console_token_hash preserved)", rec.Code)
	}

	// 引擎只收 live:scripted → 400(决策③)。
	rec, env = doBearer(t, h, http.MethodPut, "/api/v1/settings", tok, `{"engine_mode_default":"scripted"}`)
	if rec.Code != http.StatusBadRequest || env.Error == nil || env.Error.Code != "bad_request" {
		t.Fatalf("PUT engine_mode scripted: code=%d env=%+v, want 400 bad_request", rec.Code, env)
	}
	// 空 patch → 400。
	if rec, env := doBearer(t, h, http.MethodPut, "/api/v1/settings", tok, `{}`); rec.Code != http.StatusBadRequest || env.OK {
		t.Fatalf("PUT empty patch: code=%d env=%+v, want 400", rec.Code, env)
	}
	// digest 非法格式 → 400。
	if rec, env := doBearer(t, h, http.MethodPut, "/api/v1/settings", tok, `{"digest_time":"25:99"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("PUT bad digest: code=%d env=%+v, want 400", rec.Code, env)
	}
}

type companySettingsDTO struct {
	CompanyID        string  `json:"company_id"`
	EngineMode       *string `json:"engine_mode"`
	AgentCLI         *string `json:"agent_cli"`
	IssueSource      *string `json:"issue_source"`
	IssueFixturePath *string `json:"issue_fixture_path"`
}

// E2 company settings:GET 无行全 null(整行继承);PUT 覆盖 + 空串清维度;fixture 无 path → 400;
// 空 patch → 400;非法 issue_source → 400;DELETE 重置回全 null。
func TestE2CompanySettingsOverrideReset(t *testing.T) {
	srv, st := newSetupServer(t)
	t.Cleanup(func() { settings.UseMasterKey(nil) })
	h := srv.Handler()
	tok := setupConsole(t, h)
	comp := seedCompany(t, st, "ACME", "")
	cid := comp.ID

	get := func() companySettingsDTO {
		t.Helper()
		rec, env := doBearer(t, h, http.MethodGet, "/api/v1/companies/"+cid+"/settings", tok, "")
		if rec.Code != http.StatusOK || !env.OK {
			t.Fatalf("GET company settings: code=%d env=%+v", rec.Code, env)
		}
		var cs companySettingsDTO
		decodeData(t, env, &cs)
		return cs
	}

	// 无覆盖行 → 全 null。
	cs := get()
	if cs.CompanyID != cid || cs.EngineMode != nil || cs.AgentCLI != nil || cs.IssueSource != nil || cs.IssueFixturePath != nil {
		t.Fatalf("GET no-row company settings = %+v; want all null", cs)
	}

	// PUT agent_cli=codex → 生效;显式空串 = 清该维度(回退继承)。
	rec, env := doBearer(t, h, http.MethodPut, "/api/v1/companies/"+cid+"/settings", tok, `{"agent_cli":"codex"}`)
	if rec.Code != http.StatusOK || !env.OK {
		t.Fatalf("PUT company agent_cli: code=%d env=%+v", rec.Code, env)
	}
	if cs = get(); cs.AgentCLI == nil || *cs.AgentCLI != "codex" {
		t.Fatalf("company agent_cli after PUT = %+v; want codex", cs)
	}
	rec, env = doBearer(t, h, http.MethodPut, "/api/v1/companies/"+cid+"/settings", tok, `{"agent_cli":""}`)
	if rec.Code != http.StatusOK || !env.OK {
		t.Fatalf("PUT company clear agent_cli: code=%d env=%+v", rec.Code, env)
	}
	if cs = get(); cs.AgentCLI != nil {
		t.Fatalf("company agent_cli after clear = %+v; want nil(继承)", cs)
	}

	// fixture 无 path → 400;非法 source → 400。
	if rec, env := doBearer(t, h, http.MethodPut, "/api/v1/companies/"+cid+"/settings", tok, `{"issue_source":"fixture"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("PUT fixture w/o path: code=%d env=%+v, want 400", rec.Code, env)
	}
	if rec, env := doBearer(t, h, http.MethodPut, "/api/v1/companies/"+cid+"/settings", tok, `{"issue_source":"gitea"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("PUT issue_source=gitea: code=%d env=%+v, want 400", rec.Code, env)
	}
	// fixture + path 同批 → 200。
	rec, env = doBearer(t, h, http.MethodPut, "/api/v1/companies/"+cid+"/settings", tok,
		`{"issue_source":"fixture","issue_fixture_path":"/tmp/issues.json"}`)
	if rec.Code != http.StatusOK || !env.OK {
		t.Fatalf("PUT fixture+path: code=%d env=%+v", rec.Code, env)
	}
	if cs = get(); cs.IssueSource == nil || *cs.IssueSource != "fixture" || cs.IssueFixturePath == nil || *cs.IssueFixturePath != "/tmp/issues.json" {
		t.Fatalf("company fixture settings after PUT = %+v", cs)
	}
	// 空 patch → 400。
	if rec, env := doBearer(t, h, http.MethodPut, "/api/v1/companies/"+cid+"/settings", tok, `{}`); rec.Code != http.StatusBadRequest || env.OK {
		t.Fatalf("PUT company empty patch: code=%d env=%+v, want 400", rec.Code, env)
	}
	// engine_mode=scripted → 400(只收 live)。
	if rec, env := doBearer(t, h, http.MethodPut, "/api/v1/companies/"+cid+"/settings", tok, `{"engine_mode":"scripted"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("PUT company engine_mode scripted: code=%d env=%+v, want 400", rec.Code, env)
	}

	// DELETE 重置整行 → 全 null。
	rec, env = doBearer(t, h, http.MethodDelete, "/api/v1/companies/"+cid+"/settings", tok, "")
	if rec.Code != http.StatusOK || !env.OK {
		t.Fatalf("DELETE company settings: code=%d env=%+v", rec.Code, env)
	}
	if cs = get(); cs.EngineMode != nil || cs.AgentCLI != nil || cs.IssueSource != nil || cs.IssueFixturePath != nil {
		t.Fatalf("company settings after DELETE reset = %+v; want all null", cs)
	}
}

type secretMetaDTO struct {
	ID        string `json:"id"`
	Set       bool   `json:"set"`
	UpdatedAt int64  `json:"updated_at"`
}

// E3 company secrets:list 白名单顺序 + 只出掩码(明文永不回显);PUT set / DELETE 复位;
// 未知 secretID 400。
func TestE3CompanySecretsMasked(t *testing.T) {
	srv, st := newSetupServer(t)
	t.Cleanup(func() { settings.UseMasterKey(nil) })
	h := srv.Handler()
	tok := setupConsole(t, h)
	comp := seedCompany(t, st, "ACME", "")
	cid := comp.ID

	list := func() []secretMetaDTO {
		t.Helper()
		rec, env := doBearer(t, h, http.MethodGet, "/api/v1/companies/"+cid+"/secrets", tok, "")
		if rec.Code != http.StatusOK || !env.OK {
			t.Fatalf("GET secrets: code=%d env=%+v", rec.Code, env)
		}
		var out []secretMetaDTO
		decodeData(t, env, &out)
		return out
	}

	// 初始:白名单三行按确定性顺序全 set=false。
	got := list()
	if len(got) != 3 || got[0].ID != "github_token" || got[1].ID != "feishu_webhook" || got[2].ID != "feishu_secret" {
		t.Fatalf("initial secret list = %+v; want whitelist order 3 rows unset", got)
	}
	for _, m := range got {
		if m.Set {
			t.Fatalf("initial secret %s should be unset", m.ID)
		}
	}

	// 未知 id set/delete → 400。
	if rec, env := doBearer(t, h, http.MethodPut, "/api/v1/companies/"+cid+"/secrets/not_a_secret", tok, `{"value":"x"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("PUT unknown secret: code=%d env=%+v, want 400", rec.Code, env)
	}
	if rec, env := doBearer(t, h, http.MethodDelete, "/api/v1/companies/"+cid+"/secrets/not_a_secret", tok, ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("DELETE unknown secret: code=%d env=%+v, want 400", rec.Code, env)
	}

	// PUT github_token → set true;GET list 响应体绝不含明文。
	const plain = "ghp_live_secret"
	rec, env := doBearer(t, h, http.MethodPut, "/api/v1/companies/"+cid+"/secrets/github_token", tok, `{"value":"`+plain+`"}`)
	if rec.Code != http.StatusOK || !env.OK {
		t.Fatalf("PUT github_token: code=%d env=%+v body=%s", rec.Code, env, rec.Body.String())
	}
	rec, env = doBearer(t, h, http.MethodGet, "/api/v1/companies/"+cid+"/secrets", tok, "")
	if strings.Contains(rec.Body.String(), plain) {
		t.Fatalf("GET secrets response leaked plaintext:\n%s", rec.Body.String())
	}
	var metas []secretMetaDTO
	decodeData(t, env, &metas)
	if len(metas) != 3 || !metas[0].Set {
		t.Fatalf("secret list after set = %+v; want github_token set", metas)
	}

	// DELETE feishu_secret(未设置,幂等)→ 200 deleted;回读 github_token set=true,其余 false。
	rec, env = doBearer(t, h, http.MethodDelete, "/api/v1/companies/"+cid+"/secrets/feishu_secret", tok, "")
	if rec.Code != http.StatusOK || !env.OK {
		t.Fatalf("DELETE feishu_secret: code=%d env=%+v", rec.Code, env)
	}
	got = list()
	if !got[0].Set || got[1].Set || got[2].Set {
		t.Fatalf("secret list after delete = %+v; want github_token set, others unset", got)
	}
	// 明文仍不可读(secret 表只存密文,enc:v2)。
	secs, err := st.ListSecrets(context.Background(), cid)
	if err != nil || len(secs) != 1 || strings.HasPrefix(secs[0].Cipher, plain) || !strings.HasPrefix(secs[0].Cipher, "enc:v2:") {
		t.Fatalf("secret rows = %+v err:%v; want single enc:v2 cipher-only row", secs, err)
	}
}

// E4 rotate-master-key:temp masterKeyPath + 注入 old holder → 端点 token + 公司机密双 re-key;
// 新 key 落盘 0600 且 = 响应;新 key 可解、旧 key 不可解;holder 换新。
func TestE4RotateMasterKey(t *testing.T) {
	srv, st := newSetupServer(t)
	t.Cleanup(func() {
		settings.UseMasterKey(nil)
		endpoint.UseMasterKey(nil)
	})
	ctx := context.Background()
	h := srv.Handler()
	tok := setupConsole(t, h)
	comp := seedCompany(t, st, "ACME", "")
	cid := comp.ID

	// 以当前注入 holder(=/setup 主密钥)落一条端点 token + 一条 secret。
	ep, err := srv.svc.AddEndpointAs(ctx, cid, "judge", "http://127.0.0.1:1/v1", "ep-plain-99", "openai", consoleActor)
	if err != nil {
		t.Fatalf("AddEndpointAs: %v", err)
	}
	if err := srv.svc.SetCompanySecretAs(ctx, cid, service.SecretGitHubToken, "ghp_sec_plain", consoleActor); err != nil {
		t.Fatalf("SetCompanySecretAs: %v", err)
	}
	oldKey, ok := settings.MasterKey()
	if !ok {
		t.Fatal("master key not injected after setup")
	}

	// POST rotate → 200:新 key 仅此一次 + 计数。
	rec, env := doBearer(t, h, http.MethodPost, "/api/v1/settings/rotate-master-key", tok, "")
	if rec.Code != http.StatusOK || !env.OK {
		t.Fatalf("rotate: code=%d env=%+v body=%s", rec.Code, env, rec.Body.String())
	}
	var rot struct {
		MasterKey       string `json:"master_key"`
		EndpointsRekeyd int    `json:"endpoints_rekeyed"`
		SecretsRekeyd   int    `json:"secrets_rekeyed"`
	}
	decodeData(t, env, &rot)
	if len(rot.MasterKey) != 64 || rot.EndpointsRekeyd != 1 || rot.SecretsRekeyd != 1 {
		t.Fatalf("rotate result = %+v; want 64-hex key, 1/1 rekeyed", rot)
	}
	newKey, err := hex.DecodeString(rot.MasterKey)
	if err != nil {
		t.Fatalf("decode new master key: %v", err)
	}

	// key 文件 0600 覆盖,内容 = 本次响应(master_key 是唯一取回通道,必须与落盘一致)。
	kb, err := os.ReadFile(srv.masterKeyPath)
	if err != nil {
		t.Fatalf("read keyfile: %v", err)
	}
	if strings.TrimSpace(string(kb)) != rot.MasterKey {
		t.Fatalf("keyfile != returned master_key:\n%s\nvs %s", strings.TrimSpace(string(kb)), rot.MasterKey)
	}
	if fi, err := os.Stat(srv.masterKeyPath); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("keyfile perm after rotate = %v err:%v, want 0600", fi.Mode().Perm(), err)
	}

	// 新 key 可解端点 token + secret;旧 key 不可解。
	got, err := st.GetEndpoint(ctx, ep.ID)
	if err != nil {
		t.Fatalf("GetEndpoint: %v", err)
	}
	if p, err := endpoint.OpenTokenWith(newKey, got.TokenEnc); err != nil || p != "ep-plain-99" {
		t.Fatalf("endpoint open new = %q, %v; want ep-plain-99", p, err)
	}
	if _, err := endpoint.OpenTokenWith(oldKey, got.TokenEnc); err == nil {
		t.Fatal("endpoint should not open with old key after rotate")
	}
	if p, ok, err := srv.svc.OpenSecret(ctx, cid, service.SecretGitHubToken, newKey); err != nil || !ok || p != "ghp_sec_plain" {
		t.Fatalf("secret open new = ok:%v %q err:%v; want ghp_sec_plain", ok, p, err)
	}
	if _, ok, err := srv.svc.OpenSecret(ctx, cid, service.SecretGitHubToken, oldKey); err == nil || ok {
		t.Fatalf("secret should not open with old key after rotate: ok=%v err=%v", ok, err)
	}

	// 进程 holder 已换新(免重启生效)。
	if s := settings.CurrentKeySource(); s != "master-key" {
		t.Fatalf("settings.CurrentKeySource = %q, want master-key", s)
	}
	if s := endpoint.CurrentKeySource(); s != "master-key" {
		t.Fatalf("endpoint.CurrentKeySource = %q, want master-key", s)
	}
	// 换新后写一条 secret → 新 holder 可立即 seal/reopen。
	if err := srv.svc.SetCompanySecretAs(ctx, cid, service.SecretFeishuSecret, "post-rotate", consoleActor); err != nil {
		t.Fatalf("SetCompanySecretAs after rotate: %v", err)
	}
	if p, ok, err := srv.svc.OpenSecretCurrent(ctx, cid, service.SecretFeishuSecret); err != nil || !ok || p != "post-rotate" {
		t.Fatalf("OpenSecretCurrent after rotate = ok:%v %q err:%v", ok, p, err)
	}
}

// E4 前置守卫:未初始化 rotate → 400 not_initialized;masterKeyPath 空(已初始化)→ 500。
func TestE4RotateGuards(t *testing.T) {
	// 未初始化 → 400 not_initialized(先 /setup)。
	srv, _ := newSetupServer(t)
	h := srv.Handler()
	if rec, env := doAPI(t, h, http.MethodPost, "/api/v1/settings/rotate-master-key", ""); rec.Code != http.StatusBadRequest ||
		env.Error == nil || env.Error.Code != "not_initialized" {
		t.Fatalf("rotate before init: code=%d env=%+v, want 400 not_initialized", rec.Code, env)
	}

	// 已初始化但 server 无 masterKeyPath → 500(防 re-key 已提交却无法落盘)。
	srv2, _ := newTestServer(t) // 不调 SetMasterKeyPath
	ctx := context.Background()
	if err := srv2.svc.SetConsoleToken(ctx, "rotate-guard-tok"); err != nil {
		t.Fatalf("SetConsoleToken: %v", err)
	}
	if rec, env := doBearer(t, srv2.Handler(), http.MethodPost, "/api/v1/settings/rotate-master-key", "rotate-guard-tok", ""); rec.Code != http.StatusInternalServerError {
		t.Fatalf("rotate without masterKeyPath: code=%d env=%+v, want 500", rec.Code, env)
	}
}
