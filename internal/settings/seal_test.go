package settings

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// B1 密钥文件:GenerateKey=64 hex;Save 落 0600;Load 往返;KeyPath(db)=db+".key";空/缺失拒绝。
func TestKeyfileB1(t *testing.T) {
	if got := KeyPath("/data/os.db"); got != "/data/os.db.key" {
		t.Fatalf("KeyPath = %q, want /data/os.db.key", got)
	}

	k := GenerateKey()
	if len(k) != 64 {
		t.Fatalf("GenerateKey len = %d, want 64", len(k))
	}
	if b, err := hex.DecodeString(k); err != nil || len(b) != 32 {
		t.Fatalf("GenerateKey not 32 bytes hex: %v", err)
	}

	dir := t.TempDir()
	p := filepath.Join(dir, "os-test.db.key")
	if err := SaveKey(p, k); err != nil {
		t.Fatalf("SaveKey: %v", err)
	}
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatalf("stat keyfile: %v", err)
	}
	if fi.Mode().Perm() != KeyFilePerm {
		t.Fatalf("keyfile perm = %v, want %v", fi.Mode().Perm(), KeyFilePerm)
	}
	got, err := LoadKey(p)
	if err != nil {
		t.Fatalf("LoadKey: %v", err)
	}
	if got != k {
		t.Fatalf("LoadKey roundtrip = %q, want %q", got, k)
	}

	if _, err := LoadKey(filepath.Join(dir, "missing.key")); err == nil {
		t.Fatal("LoadKey on missing file: want error")
	}
	if err := SaveKey(filepath.Join(dir, "empty.key"), "  \n"); err == nil {
		t.Fatal("SaveKey empty: want error")
	}
}

// B2 settings seal:SealSecret/OpenSecret 往返;错 key 失败;空串直通;非 enc:v2 前缀报错。
func TestSealSecretB2(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef") // 32B
	if enc, err := SealSecret(key, ""); err != nil || enc != "" {
		t.Fatalf("SealSecret('') = %q, %v; want '', nil", enc, err)
	}
	if plain, err := OpenSecret(key, ""); err != nil || plain != "" {
		t.Fatalf("OpenSecret('') = %q, %v; want '', nil", plain, err)
	}

	enc, err := SealSecret(key, "ghp_token_secret")
	if err != nil {
		t.Fatalf("SealSecret: %v", err)
	}
	if !strings.HasPrefix(enc, SecretPrefix) {
		t.Fatalf("cipher = %q, want %s prefix", enc, SecretPrefix)
	}
	plain, err := OpenSecret(key, enc)
	if err != nil || plain != "ghp_token_secret" {
		t.Fatalf("OpenSecret roundtrip = %q, %v", plain, err)
	}

	wrong := []byte("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if _, err := OpenSecret(wrong, enc); err == nil {
		t.Fatal("OpenSecret wrong key: want error")
	}
	if _, err := OpenSecret(key, "not-encrypted"); err == nil {
		t.Fatal("OpenSecret bad prefix: want error")
	}
}
