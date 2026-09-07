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
	"sync"
)

// Token 加密/解密。运行期密钥源:进程级主密钥(endpoint.UseMasterKey,产品路径)——由 CLI/server
// 开库后 LoadKey(<db>.key) 注入(Phase 9 方向 config-governance §3.4 / D1-D2)。未注入时:
//   - 测试 seam 打开(envSeam,go test TestMain)→ 回落环境变量 OS_ENDPOINT_KEY(32 字节 hex);
//   - 产品(envSeam 默认关,9.4 决策①反向门控,契约 cli-readonly.md §3.1)→ 失败关闭,报
//     「master key not loaded(<db>.key);run /setup」,不再读 OS_ENDPOINT_KEY(enc:v1 迁移后作废)。
// re-key 与主密钥注入路径走下方显式 key 的 SealTokenWith/OpenTokenWith。
// 安全策略:含 token 的写入在无密钥时失败关闭,不留明文;token 为空则跳过(无鉴权端点)。

const encPrefix = "enc:v1:"

// masterKey 进程级主密钥(UseMasterKey 注入;<db>.key 解码结果)。nil = 未注入,回落 env seam。
// sync/atomic 语义经 RWMutex:注入/清除与并发解密互斥,防读到半写缓冲。
var (
	masterKey   []byte
	masterKeyMu sync.RWMutex
)

// envSeam 测试 seam 反向门控(9.4,契约 cli-readonly.md §3.1):默认 false = 产品恒不读 OS_ENDPOINT_KEY;
// 仅 endpoint 包 go test TestMain 经 SetEnvSeam(true) 打开。非测试代码不得调用。
var envSeam = false

// SetEnvSeam 开关 envKey()(OS_ENDPOINT_KEY)测试 seam。
func SetEnvSeam(on bool) { envSeam = on }

// UseMasterKey 设置进程级主密钥(key 为 32 字节解码后 hex)。key=nil 清除(未注入,产品报错/测试回落 env seam)。
// 调用方缓冲在调用后可复用(内部拷贝),避免外部并发改写。
func UseMasterKey(key []byte) {
	masterKeyMu.Lock()
	defer masterKeyMu.Unlock()
	if key == nil {
		masterKey = nil
		return
	}
	masterKey = append([]byte(nil), key...)
}

// CurrentKeySource 当前密钥源标识("master-key" | "env"),日志/测试用,不泄密钥内容。
func CurrentKeySource() string {
	masterKeyMu.RLock()
	defer masterKeyMu.RUnlock()
	if masterKey != nil {
		return "master-key"
	}
	return "env"
}

// currentKey 解析运行期密钥:主密钥(若有)→ envKey()(测试 seam)。
func currentKey() ([]byte, error) {
	masterKeyMu.RLock()
	defer masterKeyMu.RUnlock()
	if masterKey != nil {
		return append([]byte(nil), masterKey...), nil
	}
	return envKey()
}

func envKey() ([]byte, error) {
	if !envSeam {
		// 产品语义(9.4):OS_ENDPOINT_KEY 不再兜底(enc:v1 迁移后作废);须先 /setup 生成主密钥(<db>.key)。
		return nil, fmt.Errorf("endpoint master key not loaded (<db>.key); run /setup to initialize (or re-key)")
	}
	raw := os.Getenv("OS_ENDPOINT_KEY")
	if raw == "" {
		return nil, fmt.Errorf("OS_ENDPOINT_KEY not set: required to encrypt endpoint token (test seam)")
	}
	key, err := hex.DecodeString(raw)
	if err != nil || len(key) != 32 {
		return nil, fmt.Errorf("OS_ENDPOINT_KEY must be 32 bytes hex (64 hex chars)")
	}
	return key, nil
}

// SealToken 加密明文 token(运行期密钥源:主密钥 → env 测试 seam)。空 token 原样返回空(无鉴权)。
func SealToken(plain string) (string, error) {
	if plain == "" {
		return "", nil
	}
	key, err := currentKey()
	if err != nil {
		return "", err
	}
	return SealTokenWith(key, plain)
}

// OpenToken 解密密文 token(运行期密钥源:主密钥 → env 测试 seam)。空串原样返回空。
func OpenToken(enc string) (string, error) {
	if enc == "" {
		return "", nil
	}
	key, err := currentKey()
	if err != nil {
		return "", err
	}
	return OpenTokenWith(key, enc)
}

// SealTokenWith 用显式 key 加密明文 token(re-key / 主密钥路径)。空 token 原样返回空。
func SealTokenWith(key []byte, plain string) (string, error) {
	if plain == "" {
		return "", nil
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

// OpenTokenWith 用显式 key 解密密文 token。空串原样返回空。
func OpenTokenWith(key []byte, enc string) (string, error) {
	if enc == "" {
		return "", nil
	}
	if !strings.HasPrefix(enc, encPrefix) {
		return "", fmt.Errorf("endpoint token not in encrypted form (want %s prefix)", encPrefix)
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
