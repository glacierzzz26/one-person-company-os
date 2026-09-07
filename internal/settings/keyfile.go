package settings

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"sync"
)

// masterKey 进程级主密钥(settings 机密加解密;CLI root 与 /setup 开库后注入,nil = 未注入)。
// Phase 9.3 契约 runtime-knobs-web.md §3.2:与 endpoint/seal.go 的包级 holder 平行,供 service
// 机密运行期读写与换主密钥(re-key)使用。明文密文不出 service,key 只在本包与 CLI root/server 注入点可见。
var (
	masterKey   []byte
	masterKeyMu sync.RWMutex
)

// UseMasterKey 注入/清除主密钥。nil = 清除;非 nil 拷贝存(防调用方复用缓冲)。
func UseMasterKey(key []byte) {
	masterKeyMu.Lock()
	defer masterKeyMu.Unlock()
	if key == nil {
		masterKey = nil
		return
	}
	masterKey = append([]byte(nil), key...)
}

// MasterKey 返回当前注入的主密钥 + 是否已注入(service 机密读写 / rotate-master-key 用)。
func MasterKey() ([]byte, bool) {
	masterKeyMu.RLock()
	defer masterKeyMu.RUnlock()
	if masterKey == nil {
		return nil, false
	}
	return append([]byte(nil), masterKey...), true
}

// CurrentKeySource 描述当前密钥源("master-key" | "none"),日志/测试用,不泄 key 本身。
func CurrentKeySource() string {
	if _, ok := MasterKey(); ok {
		return "master-key"
	}
	return "none"
}

// 主密钥文件。Phase 9 方向(config-governance.md D2):Web /setup 首启生成-显示一次-落盘;
// 路径 = <dbPath> + ".key",内容 = 64 hex(GenerateKey),权限 0600。

// KeyFilePerm 主密钥文件权限(owner rw,group/other 无)。
const KeyFilePerm os.FileMode = 0o600

// KeyPath 主密钥文件路径:<dbPath> + ".key"(与 SQLite db 同目录同名同权限域)。
func KeyPath(dbPath string) string { return dbPath + ".key" }

// GenerateKey 生成 32 字节 hex(64 hex chars)。crypto/rand 失败极罕见,panic 保证不产出空密钥。
func GenerateKey() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("settings: crypto/rand failed: %v", err))
	}
	return hex.EncodeToString(b)
}

// SaveKey 落盘主密钥(0600,创建/覆盖)。空 key 拒绝,防误写清空。
func SaveKey(path, key string) error {
	k := strings.TrimSpace(key)
	if k == "" {
		return fmt.Errorf("settings: refusing to save empty master key")
	}
	return os.WriteFile(path, []byte(k+"\n"), KeyFilePerm)
}

// LoadKey 读主密钥(trim)。文件缺失或为空 → error。
func LoadKey(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	k := strings.TrimSpace(string(raw))
	if k == "" {
		return "", fmt.Errorf("settings: master key file %s is empty", path)
	}
	return k, nil
}
