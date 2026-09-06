package settings

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
)

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
