package cli

import (
	"fmt"
	"strings"

	"github.com/glacierzzz26/one-person-company-os/internal/settings"
)

// os server 生效配置解析(Phase 9.3,契约 runtime-knobs-web.md §3.5,决策「server 零参数」)。
// 权威源 = Web 设置页写入的 app_setting 行;server flags 保留仅作显式覆盖(Changed),标记 deprecated。
// 本文件为纯函数(无 cobra 依赖),单测直接构造 settings.AppSetting + changed 集断言。

// serverConfig 是 os server 生效配置。
type serverConfig struct {
	HTTPPort         int    // listen port
	PollMin          int    // GitHub 轮询间隔(分钟)
	QueueIntervalSec int    // 队列认领间隔(秒,仅 queue_work 开时用)
	QueueWork        bool   // 是否自消费任务队列
	Digest           string // "HH:MM" 摘要时刻;"" = 关(off)
}

// flagVals 是 server cmd 上保留覆盖能力的 flag 当前值(未显式传时 = 零值/默认,仅 Changed 才生效)。
type flagVals struct {
	port, pollMin, queueIntervalSec int
	queueWork                       bool
	digest                          string
}

// effectiveServerConfig 由 DB app_setting 行求生效配置;flag 未显式传(Changed=false)时用 DB 值。
//
//   - http_port:       DB app.HTTPPort(缺行内置默认 8787);--port  Changed → flag
//   - poll_min:        DB app.PollMin(缺省 5);        --poll   Changed → flag
//   - queue_work:      DB app.QueueWork(缺省 false);  --queue-work  Changed → flag
//   - queue_interval:  DB app.QueueIntervalSec(缺省 10); --queue-interval Changed → flag
//   - digest_time:     DB app.DigestTime(缺省 09:00); --digest Changed 且非空 → flag("off"=关);DB 空/"off" → 关
//
// 范围防护(越界 → error,boot 清晰报错不静默):port∈[1,65535]、poll≥1、interval≥1;
// digest 格式("HH:MM")交给 server.SetDigestTime 判(本函数只归一化 off→空)。
func effectiveServerConfig(app settings.AppSetting, f flagVals, changed func(name string) bool) (serverConfig, error) {
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
	if changed("port") {
		cfg.HTTPPort = f.port
	}
	if changed("poll") {
		cfg.PollMin = f.pollMin
	}
	if changed("queue-work") {
		cfg.QueueWork = f.queueWork
	}
	if changed("queue-interval") {
		cfg.QueueIntervalSec = f.queueIntervalSec
	}
	if changed("digest") && strings.TrimSpace(f.digest) != "" {
		cfg.Digest = strings.TrimSpace(f.digest)
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
