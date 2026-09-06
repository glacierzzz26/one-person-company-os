package server

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/glacierzzz26/one-person-company-os/internal/endpoint"
	"github.com/glacierzzz26/one-person-company-os/internal/service"
	"github.com/glacierzzz26/one-person-company-os/internal/storage"
	"github.com/glacierzzz26/one-person-company-os/internal/storage/repository"
	"github.com/google/uuid"
)

// Phase 9.2 /setup 首启 + console token 哈希鉴权 + re-key + 轮换(契约 console-access.md §五 A1-A3)。

// newSetupServer 同 newTestServer + 注入 masterKeyPath(<tmp>.db.key,CLI 真实路径形态)。
func newSetupServer(t *testing.T) (*Server, *repository.Store) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "os-test.db")
	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	st := repository.NewStore(db)
	srv := New(service.New(st), 0)
	srv.SetMasterKeyPath(filepath.Join(dir, "os-test.db.key"))
	t.Cleanup(func() { endpoint.UseMasterKey(nil) }) // 复位进程级 holder,防污染其它 server 测试
	return srv, st
}

func doBearer(t *testing.T, h http.Handler, method, path, token, body string) (*httptest.ResponseRecorder, apiEnvelope) {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var env apiEnvelope
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	return rec, env
}

// A1 首启初始化闭环:未初始化开放 → POST /setup → master_key 64hex + 落盘 0600 + 已初始化 → 鉴权生效。
func TestA1SetupFirstBoot(t *testing.T) {
	srv, st := newSetupServer(t)
	ctx := context.Background()

	// 未初始化:status false + /api/v1 开放。
	rec, env := doAPI(t, srv.Handler(), http.MethodGet, "/api/v1/setup/status", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("setup/status: code=%d", rec.Code)
	}
	var s0 struct {
		Initialized bool `json:"initialized"`
	}
	decodeData(t, env, &s0)
	if s0.Initialized {
		t.Fatal("fresh db should be uninitialized")
	}
	if rec, _ := doAPI(t, srv.Handler(), http.MethodGet, "/api/v1/companies", ""); rec.Code != http.StatusOK {
		t.Fatalf("pre-setup /api/v1 should be open: code=%d", rec.Code)
	}

	// POST /setup → 200 + master_key。
	rec, env = doAPI(t, srv.Handler(), http.MethodPost, "/api/v1/setup", `{"console_token":"sekret-token"}`)
	if rec.Code != http.StatusOK || !env.OK {
		t.Fatalf("setup: code=%d ok=%v err=%+v body=%s", rec.Code, env.OK, env.Error, rec.Body.String())
	}
	var sr struct {
		Initialized bool   `json:"initialized"`
		MasterKey   string `json:"master_key"`
	}
	decodeData(t, env, &sr)
	if !sr.Initialized || len(sr.MasterKey) != 64 {
		t.Fatalf("setup result = %+v; want initialized + 64-hex master_key", sr)
	}
	if _, err := hex.DecodeString(sr.MasterKey); err != nil {
		t.Fatalf("master_key not hex: %v", err)
	}

	// 主密钥落盘 0600(路径 = db.key)。
	fi, err := os.Stat(srv.masterKeyPath)
	if err != nil {
		t.Fatalf("stat keyfile: %v", err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("keyfile perm = %v, want 0600", fi.Mode().Perm())
	}
	// 进程内已注入主密钥(免重启)。
	if got := endpoint.CurrentKeySource(); got != "master-key" {
		t.Fatalf("CurrentKeySource after setup = %q, want master-key", got)
	}

	// 已初始化:status true + /api/v1 无头 401、带头 200、错 token 401。
	rec, env = doAPI(t, srv.Handler(), http.MethodGet, "/api/v1/setup/status", "")
	decodeData(t, env, &s0)
	if !s0.Initialized {
		t.Fatal("setup/status should be initialized after setup")
	}
	if rec, _ := doAPI(t, srv.Handler(), http.MethodGet, "/api/v1/companies", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("no token after init: code=%d, want 401", rec.Code)
	}
	if rec, _ := doBearer(t, srv.Handler(), http.MethodGet, "/api/v1/companies", "sekret-token", ""); rec.Code != http.StatusOK {
		t.Fatalf("with token: code=%d, want 200", rec.Code)
	}
	if rec, _ := doBearer(t, srv.Handler(), http.MethodGet, "/api/v1/companies", "wrong-token", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong token: code=%d, want 401", rec.Code)
	}

	// 重复 setup → 409(不覆盖)。
	rec, env = doAPI(t, srv.Handler(), http.MethodPost, "/api/v1/setup", `{"console_token":"another-token"}`)
	if rec.Code != http.StatusConflict || env.Error == nil || env.Error.Code != "already_initialized" {
		t.Fatalf("double setup: code=%d env=%+v, want 409 already_initialized", rec.Code, env)
	}
	// 旧令牌仍有效(未被第二次 setup 覆盖)。
	if rec, _ := doBearer(t, srv.Handler(), http.MethodGet, "/api/v1/companies", "sekret-token", ""); rec.Code != http.StatusOK {
		t.Fatalf("original token after double-setup: code=%d, want 200", rec.Code)
	}

	// audit:setup 的 set-console-token 落 actor=human:console。
	ok, err := st.HasAuditAction(ctx, "settings", "global", "set-console-token")
	if err != nil || !ok {
		t.Fatalf("setup audit = ok:%v err:%v; want recorded", ok, err)
	}
}

// A2 setup re-key:存量 enc:v1 端点 token 从旧 key 换到新主密钥;错 old key → 400 且零写回、仍未初始化。
func TestA2SetupRekey(t *testing.T) {
	srv, st := newSetupServer(t)
	ctx := context.Background()

	comp := seedCompany(t, st, "ACME", "")
	// 用显式旧 key(模拟 OS_ENDPOINT_KEY 时代)seal 端点 token,直插 repo。
	oldKeyHex := "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"
	oldKey, _ := hex.DecodeString(oldKeyHex)
	tok, err := endpoint.SealTokenWith(oldKey, "ghp_old_live")
	if err != nil {
		t.Fatalf("seal old token: %v", err)
	}
	now := time.Now().Unix()
	ep, err := st.CreateEndpoint(ctx, endpoint.Endpoint{
		ID: uuid.NewString(), CompanyID: comp.ID, Name: "judge", BaseURL: "http://localhost:1/v1",
		TokenEnc: tok, Proto: "openai", Vendor: "openai", Role: "pool", Tier: "standard",
		Status: "active", CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("CreateEndpoint: %v", err)
	}

	// POST /setup 带 old_endpoint_key → 200;返回 master_key 能解该端点 token,旧 key 失效。
	body := `{"console_token":"sekret-token","old_endpoint_key":"` + oldKeyHex + `"}`
	rec, env := doAPI(t, srv.Handler(), http.MethodPost, "/api/v1/setup", body)
	if rec.Code != http.StatusOK || !env.OK {
		t.Fatalf("setup rekey: code=%d env=%+v body=%s", rec.Code, env, rec.Body.String())
	}
	var sr struct {
		MasterKey string `json:"master_key"`
	}
	decodeData(t, env, &sr)
	newKey, _ := hex.DecodeString(sr.MasterKey)

	got, err := st.GetEndpoint(ctx, ep.ID)
	if err != nil {
		t.Fatalf("GetEndpoint after rekey: %v", err)
	}
	if p, err := endpoint.OpenTokenWith(newKey, got.TokenEnc); err != nil || p != "ghp_old_live" {
		t.Fatalf("open with new master key = %q, %v; want ghp_old_live", p, err)
	}
	if _, err := endpoint.OpenTokenWith(oldKey, got.TokenEnc); err == nil {
		t.Fatal("old env key should not open token after re-key")
	}
	if got := endpoint.CurrentKeySource(); got != "master-key" {
		t.Fatalf("CurrentKeySource = %q, want master-key", got)
	}
}

// A2 原子性:错 old key → 400 rekey_failed 且零写回(端点 token 未动、哈希未设、key 文件未落、仍可重试)。
func TestA2SetupRekeyFailureZeroWrite(t *testing.T) {
	srv, st := newSetupServer(t)
	ctx := context.Background()

	comp := seedCompany(t, st, "ACME", "")
	oldKeyHex := "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"
	oldKey, _ := hex.DecodeString(oldKeyHex)
	tok, _ := endpoint.SealTokenWith(oldKey, "ghp_still_mine")
	now := time.Now().Unix()
	ep, err := st.CreateEndpoint(ctx, endpoint.Endpoint{
		ID: uuid.NewString(), CompanyID: comp.ID, Name: "judge", BaseURL: "http://localhost:1/v1",
		TokenEnc: tok, Proto: "openai", Vendor: "openai", Role: "pool", Tier: "standard",
		Status: "active", CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("CreateEndpoint: %v", err)
	}

	wrongKeyHex := "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210"
	body := `{"console_token":"sekret-token","old_endpoint_key":"` + wrongKeyHex + `"}`
	rec, env := doAPI(t, srv.Handler(), http.MethodPost, "/api/v1/setup", body)
	if rec.Code != http.StatusBadRequest || env.Error == nil || env.Error.Code != "rekey_failed" {
		t.Fatalf("setup wrong old key: code=%d env=%+v, want 400 rekey_failed", rec.Code, env)
	}
	// 零写回:token 原 key 仍可解;哈希未设(未初始化);key 文件未落;可重试成功。
	got, err := st.GetEndpoint(ctx, ep.ID)
	if err != nil {
		t.Fatalf("GetEndpoint: %v", err)
	}
	if p, err := endpoint.OpenTokenWith(oldKey, got.TokenEnc); err != nil || p != "ghp_still_mine" {
		t.Fatalf("token should be untouched: open old = %q, %v", p, err)
	}
	if h, _ := srv.svc.ConsoleTokenHash(ctx); h != "" {
		t.Fatalf("console_token_hash after failed setup = %q; want ''", h)
	}
	if _, err := os.Stat(srv.masterKeyPath); !os.IsNotExist(err) {
		t.Fatalf("keyfile should not be written after failed setup")
	}
	// 重试(正确 old key)→ 成功,证明仍停在未初始化态。
	bodyOK := `{"console_token":"sekret-token","old_endpoint_key":"` + oldKeyHex + `"}`
	if rec, env := doAPI(t, srv.Handler(), http.MethodPost, "/api/v1/setup", bodyOK); rec.Code != http.StatusOK || !env.OK {
		t.Fatalf("setup retry: code=%d env=%+v", rec.Code, env)
	}
}

// A3 轮换 PUT /settings/console-token:旧 token 即失效、新 token 生效;未初始化 → 409。
func TestA3RotateConsoleToken(t *testing.T) {
	srv, _ := newSetupServer(t)
	h := srv.Handler()

	// 未初始化:PUT 轮换 → 409(无令牌可轮换)。
	if rec, _ := doBearer(t, h, http.MethodPut, "/api/v1/settings/console-token", "", `{"new_token":"fresh-token"}`); rec.Code != http.StatusConflict {
		t.Fatalf("rotate before init: code=%d, want 409", rec.Code)
	}

	if rec, env := doAPI(t, h, http.MethodPost, "/api/v1/setup", `{"console_token":"old-token-1"}`); rec.Code != http.StatusOK || !env.OK {
		t.Fatalf("setup: code=%d env=%+v", rec.Code, env)
	}

	// 带旧 token 轮换 → 200。
	rec, env := doBearer(t, h, http.MethodPut, "/api/v1/settings/console-token", "old-token-1", `{"new_token":"new-token-2"}`)
	if rec.Code != http.StatusOK || !env.OK {
		t.Fatalf("rotate: code=%d env=%+v body=%s", rec.Code, env, rec.Body.String())
	}
	// 旧 token 401、新 token 200。
	if rec, _ := doBearer(t, h, http.MethodGet, "/api/v1/companies", "old-token-1", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("old token after rotate: code=%d, want 401", rec.Code)
	}
	if rec, _ := doBearer(t, h, http.MethodGet, "/api/v1/companies", "new-token-2", ""); rec.Code != http.StatusOK {
		t.Fatalf("new token after rotate: code=%d, want 200", rec.Code)
	}
	// 空新 token → 400。
	if rec, env := doBearer(t, h, http.MethodPut, "/api/v1/settings/console-token", "new-token-2", `{"new_token":""}`); rec.Code != http.StatusBadRequest || env.OK {
		t.Fatalf("rotate empty: code=%d env=%+v, want 400", rec.Code, env)
	}
}

// 无 masterKeyPath 的 server → /setup 500(防「re-key 已提交但主密钥无路径可落」)。
func TestSetupWithoutKeyPath(t *testing.T) {
	srv, _ := newTestServer(t) // 不调 SetMasterKeyPath
	rec, env := doAPI(t, srv.Handler(), http.MethodPost, "/api/v1/setup", `{"console_token":"sekret-token"}`)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("setup without key path: code=%d env=%+v, want 500", rec.Code, env)
	}
	if h, _ := srv.svc.ConsoleTokenHash(context.Background()); h != "" {
		t.Fatal("hash should stay '' when setup fails")
	}
}
