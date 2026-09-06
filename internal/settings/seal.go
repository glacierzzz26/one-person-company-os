package settings

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"
)

// secret 表密文(显式主密钥,enc:v2:)。与 endpoint.token 的 enc:v1 共用同一 AES-GCM 结构,
// 仅密钥来源不同:主密钥来自 DB 旁 .key 文件(9.2 Web 注入),显式入参无 env seam。

const SecretPrefix = "enc:v2:"

// SealSecret 用主密钥加密明文机密。空 plain → 空(空即无该 secret,不落密文)。
func SealSecret(key []byte, plain string) (string, error) {
	if plain == "" {
		return "", nil
	}
	sealed, err := sealAESGCM(key, []byte(plain))
	if err != nil {
		return "", err
	}
	return SecretPrefix + base64.StdEncoding.EncodeToString(sealed), nil
}

// OpenSecret 用主密钥解密密文机密。空 enc → 空。前缀不对 → 明确报错。
func OpenSecret(key []byte, enc string) (string, error) {
	if enc == "" {
		return "", nil
	}
	if !strings.HasPrefix(enc, SecretPrefix) {
		return "", fmt.Errorf("secret not in encrypted form (want %s prefix)", SecretPrefix)
	}
	data, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(enc, SecretPrefix))
	if err != nil {
		return "", fmt.Errorf("decode secret: %w", err)
	}
	plain, err := openAESGCM(key, data)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

// sealAESGCM 生成随机 nonce 并加密,输出 = nonce||ciphertext(nonce 前置,与 endpoint seal 同布局)。
func sealAESGCM(key, plain []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, plain, nil), nil
}

// openAESGCM 拆 nonce 并解密。数据过短 / 错 key → error。
func openAESGCM(key, data []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(data) < gcm.NonceSize() {
		return nil, fmt.Errorf("secret too short")
	}
	nonce, ciphertext := data[:gcm.NonceSize()], data[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt secret: %w", err)
	}
	return plain, nil
}
