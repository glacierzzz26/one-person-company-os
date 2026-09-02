package endpoint

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
)

// Token 加密/解密。密钥来自环境变量 OS_ENDPOINT_KEY(32 字节 hex)。
// 安全策略:含 token 的写入在无密钥时失败关闭,不留明文;token 为空则跳过(无鉴权端点)。

const encPrefix = "enc:v1:"

func envKey() ([]byte, error) {
	raw := os.Getenv("OS_ENDPOINT_KEY")
	if raw == "" {
		return nil, fmt.Errorf("OS_ENDPOINT_KEY not set: required to encrypt endpoint token")
	}
	key, err := hex.DecodeString(raw)
	if err != nil || len(key) != 32 {
		return nil, fmt.Errorf("OS_ENDPOINT_KEY must be 32 bytes hex (64 hex chars)")
	}
	return key, nil
}

// SealToken 加密明文 token。空 token 原样返回空(无鉴权)。
func SealToken(plain string) (string, error) {
	if plain == "" {
		return "", nil
	}
	key, err := envKey()
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plain), nil)
	return encPrefix + base64.StdEncoding.EncodeToString(sealed), nil
}

// OpenToken 解密密文 token。空串原样返回空。
func OpenToken(enc string) (string, error) {
	if enc == "" {
		return "", nil
	}
	if !strings.HasPrefix(enc, encPrefix) {
		return "", fmt.Errorf("endpoint token not in encrypted form (want %s prefix)", encPrefix)
	}
	key, err := envKey()
	if err != nil {
		return "", err
	}
	data, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(enc, encPrefix))
	if err != nil {
		return "", fmt.Errorf("decode token: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(data) < gcm.NonceSize() {
		return "", fmt.Errorf("token too short")
	}
	nonce, ciphertext := data[:gcm.NonceSize()], data[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt token: %w", err)
	}
	return string(plain), nil
}
