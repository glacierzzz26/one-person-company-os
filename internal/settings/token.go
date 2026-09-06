package settings

import (
	"crypto/sha256"
	"encoding/hex"
)

// 控制台访问令牌(console token)。DB 只存哈希不存明文(app_setting.console_token_hash,
// ''=未初始化,/setup 开放)——服务端 bearer 校验为 sha256hex(请求 token) 与哈希 constant-time 比较。

// HashConsoleToken 计算 console 访问令牌的 sha256 hex(DB 存储形态)。
// 不做加盐:单令牌随机性即熵源(轮换即重随);加盐会使服务端明文比较复杂化而收益为零。
func HashConsoleToken(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}
