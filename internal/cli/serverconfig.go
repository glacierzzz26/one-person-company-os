package cli

import (
	"fmt"
	"strings"

	"github.com/glacierzzz26/one-person-company-os/internal/settings"
)

// os server 生效配置解析(Phase 9.3 runtime-knobs-web.md §3.5;9.4 cli-readonly.md §3.2,决策③)。
// 权威源 = Web 设置页写入的 app_setting 行(唯一);业务 flag 已删(9.4),os server 零业务参数。
// 本文件为纯函数(无 cobra 依赖),单测直接构造 settings.AppSetting 断言。

// serverConfig 是 os server 生效配置。
type serverConfig struct {
	HTTPPort         int    // listen port
	PollMin          int    // GitHub 轮询间隔(分钟)
	QueueIntervalSec int    // 队列认领间隔(秒,仅 queue_work 开时用)
	QueueWork        bool   // 是否自消费任务队列
	Digest           string // "HH:MM" 摘要时刻;"" = 关(off)
}

// serverConfigFromApp 由 DB app_setting 行求 os server 生效配置(9.4 决策③:无 flag 覆盖)。
// svc.AppSetting 缺行已回内置默认(9.1),零值填默认保留为防御(手工零列行);digest 归一化
// "" / "off" → "" = 关(SetDigestTime 语义);digest 格式("HH:MM")交给 server.SetDigestTime 判。
// 范围防护(越界 → error,boot 清晰报错不静默):port∈[1,65535]、poll≥1、interval≥1。
func serverConfigFromApp(app settings.AppSetting) (serverConfig, error) {
	cfg := serverConfig{
		HTTPPort:         app.HTTPPort,
		PollMin:          app.PollMin,
		QueueWork:        app.QueueWork,
		QueueIntervalSec: app.QueueIntervalSec,
		Digest:           app.DigestTime,
	}
	if cfg.HTTPPort == 0 {
		cfg.HTTPPort = settings.DefaultHTTPPort
	}
	if cfg.PollMin == 0 {
		cfg.PollMin = settings.DefaultPollMin
	}
	if cfg.QueueIntervalSec == 0 {
		cfg.QueueIntervalSec = settings.DefaultQueueIntervalSec
	}
	if cfg.Digest == "" || strings.EqualFold(cfg.Digest, "off") {
		cfg.Digest = "" // 归一化:空/"off" = 关(SetDigestTime 语义)
	}

	if cfg.HTTPPort < 1 || cfg.HTTPPort > 65535 {
		return serverConfig{}, fmt.Errorf("http_port must be 1..65535 (got %d)", cfg.HTTPPort)
	}
	if cfg.PollMin < 1 {
		return serverConfig{}, fmt.Errorf("poll_min must be >= 1 (got %d)", cfg.PollMin)
	}
	if cfg.QueueIntervalSec < 1 {
		return serverConfig{}, fmt.Errorf("queue_interval_sec must be >= 1 (got %d)", cfg.QueueIntervalSec)
	}
	return cfg, nil
}
