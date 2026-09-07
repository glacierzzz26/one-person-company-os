package service

// Phase 9.3 — 机密 Current 族(主密钥 holder)/ RekeyAll / 白名单 + 审计(契约 runtime-knobs-web.md §五 C1-C3)。
// settings 与 endpoint 主密钥 holder 均为进程级:注入测试一律 t.Cleanup 复位。

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/glacierzzz26/one-person-company-os/internal/endpoint"
	"github.com/glacierzzz26/one-person-company-os/internal/settings"
	"github.com/google/uuid"
)

// C1 secret Current 族:settings.UseMasterKey 注入后 SetSecretCurrent/OpenSecretCurrent roundtrip
// (明文只进 seal,库内只存 enc:v2);未注入主密钥 → 明确报错。
func TestC1SecretCurrentRoundtrip(t *testing.T) {
	svc, st := newSvc(t)
	ctx := context.Background()
	comp := seedCompanyID(t, st)
	k := key32()

	// 未注入主密钥 → OpenSecretCurrent 报错(不 panic)。
	if _, _, err := svc.OpenSecretCurrent(ctx, comp, SecretGitHubToken); err == nil {
		t.Fatal("OpenSecretCurrent without injected key: want error")
	}
	if err := svc.SetSecretCurrent(ctx, comp, SecretGitHubToken, "ghp_plain"); err == nil {
		t.Fatal("SetSecretCurrent without injected key: want error")
	}

	settings.UseMasterKey(k)
	t.Cleanup(func() { settings.UseMasterKey(nil) })
	if err := svc.SetSecretCurrent(ctx, comp, SecretGitHubToken, "ghp_plain"); err != nil {
		t.Fatalf("SetSecretCurrent: %v", err)
	}
	p, ok, err := svc.OpenSecretCurrent(ctx, comp, SecretGitHubToken)
	if err != nil || !ok || p != "ghp_plain" {
		t.Fatalf("OpenSecretCurrent = ok:%v %q err:%v; want ghp_plain", ok, p, err)
	}
	// 库内只存密文(enc:v2: 前缀,明文不出 service/store)。
	secs, err := st.ListSecrets(ctx, comp)
	if err != nil || len(secs) != 1 {
		t.Fatalf("ListSecrets = %+v err:%v; want 1 row", secs, err)
	}
	if !strings.HasPrefix(secs[0].Cipher, "enc:v2:") || strings.Contains(secs[0].Cipher, "ghp_plain") {
		t.Fatalf("secret cipher leaked plaintext = %q; want enc:v2: only", secs[0].Cipher)
	}
}

// C2 RekeyAll(端点 token + 公司机密双表):oldKey 全解成功 → 换新后新解成功、旧解失败、计数正确;
// 错 oldKey → error 且零写(两端点 token 与 secret 均仍可被旧 key 解、不可被错 key 解)。
func TestC2RekeyAllBothTables(t *testing.T) {
	svc, st := newSvc(t)
	ctx := context.Background()
	comp := seedCompanyID(t, st)
	oldKey := key32()
	newKey := []byte("abcdefghijklmnopqrstuvwxyz123456") // 32 字节
	now := time.Now().Unix()

	// 端点 token + 公司机密各一条,均用 oldKey seal(直插 repo / 显式 key 路径,不依赖 holder)。
	tok, err := endpoint.SealTokenWith(oldKey, "ep-plain")
	if err != nil {
		t.Fatalf("seal endpoint token: %v", err)
	}
	ep, err := st.CreateEndpoint(ctx, endpoint.Endpoint{
		ID: uuid.NewString(), CompanyID: comp, Name: "judge", BaseURL: "http://localhost:1/v1",
		TokenEnc: tok, Proto: "openai", Vendor: "openai", Role: "pool", Tier: "standard",
		Status: "active", CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("CreateEndpoint: %v", err)
	}
	if err := svc.SetSecret(ctx, comp, SecretGitHubToken, "sec-plain", oldKey); err != nil {
		t.Fatalf("SetSecret oldKey: %v", err)
	}

	epN, secN, err := svc.RekeyAll(ctx, oldKey, newKey)
	if err != nil {
		t.Fatalf("RekeyAll: %v", err)
	}
	if epN != 1 || secN != 1 {
		t.Fatalf("RekeyAll counts = %d endpoints, %d secrets; want 1/1", epN, secN)
	}
	// 端点 token 新 key 可解、旧 key 失败。
	got, err := st.GetEndpoint(ctx, ep.ID)
	if err != nil {
		t.Fatalf("GetEndpoint after rekey: %v", err)
	}
	if p, err := endpoint.OpenTokenWith(newKey, got.TokenEnc); err != nil || p != "ep-plain" {
		t.Fatalf("endpoint open new = %q, %v; want ep-plain", p, err)
	}
	if _, err := endpoint.OpenTokenWith(oldKey, got.TokenEnc); err == nil {
		t.Fatal("endpoint old key should fail after rekey")
	}
	// secret 新 key 可解、旧 key 失败。
	if p, ok, err := svc.OpenSecret(ctx, comp, SecretGitHubToken, newKey); err != nil || !ok || p != "sec-plain" {
		t.Fatalf("secret open new = ok:%v %q err:%v; want sec-plain", ok, p, err)
	}
	if _, ok, err := svc.OpenSecret(ctx, comp, SecretGitHubToken, oldKey); err == nil || ok {
		t.Fatalf("secret old key should fail after rekey: ok=%v err=%v", ok, err)
	}
}

func TestC2RekeyAllWrongOldZeroWrite(t *testing.T) {
	svc, st := newSvc(t)
	ctx := context.Background()
	comp := seedCompanyID(t, st)
	oldKey := key32()
	now := time.Now().Unix()

	tok, err := endpoint.SealTokenWith(oldKey, "still-ep")
	if err != nil {
		t.Fatalf("seal endpoint token: %v", err)
	}
	ep, err := st.CreateEndpoint(ctx, endpoint.Endpoint{
		ID: uuid.NewString(), CompanyID: comp, Name: "judge", BaseURL: "http://localhost:1/v1",
		TokenEnc: tok, Proto: "openai", Vendor: "openai", Role: "pool", Tier: "standard",
		Status: "active", CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("CreateEndpoint: %v", err)
	}
	if err := svc.SetSecret(ctx, comp, SecretGitHubToken, "still-sec", oldKey); err != nil {
		t.Fatalf("SetSecret oldKey: %v", err)
	}

	wrongKey := []byte("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if _, _, err := svc.RekeyAll(ctx, wrongKey, []byte("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")); err == nil {
		t.Fatal("RekeyAll wrong old key: want error")
	}
	// 零写:端点 token 与 secret 都仍可被 oldKey 解。
	got, err := st.GetEndpoint(ctx, ep.ID)
	if err != nil {
		t.Fatalf("GetEndpoint: %v", err)
	}
	if p, err := endpoint.OpenTokenWith(oldKey, got.TokenEnc); err != nil || p != "still-ep" {
		t.Fatalf("endpoint should be untouched: open old = %q, %v", p, err)
	}
	if p, ok, err := svc.OpenSecret(ctx, comp, SecretGitHubToken, oldKey); err != nil || !ok || p != "still-sec" {
		t.Fatalf("secret should be untouched: open old = ok:%v %q err:%v", ok, p, err)
	}
}

// C3 白名单:set/delete 未知 secretID 拒绝(ErrSettingsBadRequest);合法 set 审计 detail 不含明文。
func TestC3SecretWhitelistAndAudit(t *testing.T) {
	svc, st := newSvc(t)
	ctx := context.Background()
	comp := seedCompanyID(t, st)
	settings.UseMasterKey(key32())
	t.Cleanup(func() { settings.UseMasterKey(nil) })

	// 未知 id:set/delete 均拒(400 sentinel)。
	if err := svc.SetCompanySecretAs(ctx, comp, "api_key", "v", "human:console"); !errors.Is(err, ErrSettingsBadRequest) {
		t.Fatalf("SetCompanySecretAs unknown id: err=%v; want ErrSettingsBadRequest", err)
	}
	if err := svc.DeleteCompanySecretAs(ctx, comp, "api_key", "human:console"); !errors.Is(err, ErrSettingsBadRequest) {
		t.Fatalf("DeleteCompanySecretAs unknown id: err=%v; want ErrSettingsBadRequest", err)
	}

	// 合法 set:落一条;空 value 拒。
	if err := svc.SetCompanySecretAs(ctx, comp, SecretFeishuSecret, "feishu-sign-secret-xyz", "human:console"); err != nil {
		t.Fatalf("SetCompanySecretAs feishu_secret: %v", err)
	}
	if err := svc.SetCompanySecretAs(ctx, comp, SecretFeishuWebhook, "   ", "human:console"); !errors.Is(err, ErrSettingsBadRequest) {
		t.Fatalf("empty value: err=%v; want ErrSettingsBadRequest", err)
	}

	// 审计 detail 不得含明文(id/company 掩码除外)。
	audits, err := st.ListAudits(ctx, "secret")
	if err != nil {
		t.Fatalf("ListAudits secret: %v", err)
	}
	if len(audits) == 0 {
		t.Fatal("no secret audit rows recorded")
	}
	for _, a := range audits {
		if strings.Contains(a.Detail, "feishu-sign-secret-xyz") || strings.Contains(a.Detail, "github_token value") {
			t.Fatalf("audit detail leaked secret plaintext: %q", a.Detail)
		}
	}
}
