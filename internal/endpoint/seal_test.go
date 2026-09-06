package endpoint

import (
	"encoding/hex"
	"strings"
	"testing"
)

// B2 seal 注入:SealTokenWith/OpenTokenWith 往返;错 key 失败;env SealToken/OpenToken 语义不变(测试 seam)。
func TestSealWithRoundtrip(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	enc, err := SealTokenWith(key, "tok-123")
	if err != nil {
		t.Fatalf("SealTokenWith: %v", err)
	}
	if !strings.HasPrefix(enc, "enc:v1:") {
		t.Fatalf("cipher = %q, want enc:v1: prefix", enc)
	}
	plain, err := OpenTokenWith(key, enc)
	if err != nil || plain != "tok-123" {
		t.Fatalf("OpenTokenWith roundtrip = %q, %v", plain, err)
	}

	if _, err := OpenTokenWith([]byte("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), enc); err == nil {
		t.Fatal("OpenTokenWith wrong key: want error")
	}
	if _, err := OpenTokenWith(key, "plain"); err == nil {
		t.Fatal("OpenTokenWith bad prefix: want error")
	}
	// 空串直通(无鉴权端点)。
	if e, err := SealTokenWith(key, ""); err != nil || e != "" {
		t.Fatalf("SealTokenWith('') = %q, %v", e, err)
	}
	if p, err := OpenTokenWith(key, ""); err != nil || p != "" {
		t.Fatalf("OpenTokenWith('') = %q, %v", p, err)
	}
}

// B2 env 语义不变:OS_ENDPOINT_KEY 存在时 SealToken/OpenToken 照旧工作,且产物可被 With 变体解。
func TestEnvSealSemanticsUnchanged(t *testing.T) {
	oldHex := "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"
	t.Setenv("OS_ENDPOINT_KEY", oldHex)
	enc, err := SealToken("env-token")
	if err != nil {
		t.Fatalf("SealToken: %v", err)
	}
	plain, err := OpenToken(enc)
	if err != nil || plain != "env-token" {
		t.Fatalf("OpenToken roundtrip = %q, %v", plain, err)
	}
	key, _ := hex.DecodeString(oldHex)
	if p, err := OpenTokenWith(key, enc); err != nil || p != "env-token" {
		t.Fatalf("OpenTokenWith(env-key) = %q, %v", p, err)
	}
}

// B1 holder:UseMasterKey 注入后 SealToken/OpenToken 用主密钥(不理 env);清除回 env;env seam 语义不变。
func TestUseMasterKeyHolder(t *testing.T) {
	t.Cleanup(func() { UseMasterKey(nil) }) // 复位进程级 holder,防污染包内依赖 env 的其它测试
	envHex := "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"
	envKey, _ := hex.DecodeString(envHex)
	t.Setenv("OS_ENDPOINT_KEY", envHex)

	// 未注入 master → env seam;env 密封产物可用显式 env key 解。
	if src := CurrentKeySource(); src != "env" {
		t.Fatalf("CurrentKeySource (no master) = %q, want env", src)
	}
	encEnv, err := SealToken("tok-env")
	if err != nil {
		t.Fatalf("SealToken env: %v", err)
	}
	if p, err := OpenToken(encEnv); err != nil || p != "tok-env" {
		t.Fatalf("OpenToken env roundtrip = %q, %v", p, err)
	}

	// 注入 master → 走 master;env 密封产物不再被 OpenToken 解(需 re-key),显式 env key 仍可解(With 层)。
	master := []byte("0123456789abcdef0123456789abcdef")
	UseMasterKey(master)
	if src := CurrentKeySource(); src != "master-key" {
		t.Fatalf("CurrentKeySource (master) = %q, want master-key", src)
	}
	encM, err := SealToken("tok-m")
	if err != nil {
		t.Fatalf("SealToken master: %v", err)
	}
	if p, err := OpenToken(encM); err != nil || p != "tok-m" {
		t.Fatalf("OpenToken master roundtrip = %q, %v", p, err)
	}
	if _, err := OpenTokenWith(envKey, encM); err == nil {
		t.Fatal("env key must not open master-sealed token")
	}
	if _, err := OpenToken(encEnv); err == nil {
		t.Fatal("OpenToken after master injection must not fall back to env (fail-closed till re-key)")
	}
	if p, err := OpenTokenWith(envKey, encEnv); err != nil || p != "tok-env" {
		t.Fatalf("OpenTokenWith(envKey, env-sealed) = %q, %v", p, err)
	}

	// 清除 master → 回落 env seam(encEnv 重新可经 OpenToken 解)。
	UseMasterKey(nil)
	if src := CurrentKeySource(); src != "env" {
		t.Fatalf("CurrentKeySource (cleared) = %q, want env", src)
	}
	if p, err := OpenToken(encEnv); err != nil || p != "tok-env" {
		t.Fatalf("OpenToken after clear = %q, %v", p, err)
	}

	// env 未设 & 无 master → Seal 失败(语义不变,fail-closed)。
	t.Setenv("OS_ENDPOINT_KEY", "")
	if _, err := SealToken("x"); err == nil {
		t.Fatal("SealToken with no master and no env: want error")
	}
}
