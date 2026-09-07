package cli

// Phase 9.4(契约 cli-readonly.md §五 D1-D2;9.3 原 effectiveServerConfig D 用例适配)。
// 业务 flag 已删(9.4 决策③),serverConfigFromApp 只由 DB app_setting 行求生效配置(纯函数,无 cobra)。
// 语义:0 值行列 = 零填内置默认(不越界);bounds 错误只来自非零越界行值;digest 空/"off" = 关。

import (
	"strings"
	"testing"

	"github.com/glacierzzz26/one-person-company-os/internal/settings"
)

// D1 DB 行值生效;零值行 = 内置默认;digest 空/"off" 归一关;正常 "HH:MM" 保留。
func TestD1ServerConfigFromApp(t *testing.T) {
	// 零值行(等价未配置)→ 内置默认(server.go 的 app 实为 svc.AppSetting():缺行已回 DefaultAppSetting,
	// 此分支只兜手工零列行)。
	cfg, err := serverConfigFromApp(settings.AppSetting{})
	if err != nil {
		t.Fatalf("zero-row config: %v", err)
	}
	if cfg.HTTPPort != settings.DefaultHTTPPort || cfg.PollMin != settings.DefaultPollMin ||
		cfg.QueueIntervalSec != settings.DefaultQueueIntervalSec || cfg.QueueWork ||
		cfg.Digest != "" { // 零值 DigestTime="" → 关(非 09:00;默认时刻由 DefaultAppSetting 行携带)
		t.Fatalf("zero-row config = %+v", cfg)
	}

	// DefaultAppSetting(Web 未改的首行)→ 内置默认 + digest 09:00 保留。
	cfg, err = serverConfigFromApp(settings.DefaultAppSetting())
	if err != nil {
		t.Fatalf("default app config: %v", err)
	}
	if cfg.HTTPPort != settings.DefaultHTTPPort || cfg.PollMin != settings.DefaultPollMin ||
		cfg.QueueIntervalSec != settings.DefaultQueueIntervalSec || cfg.QueueWork ||
		cfg.Digest != settings.DefaultDigestTime {
		t.Fatalf("defaults config = %+v", cfg)
	}

	// DB 行值生效:port/poll/queue 全被行值驱动;digest_time="" = 关。
	app := settings.AppSetting{HTTPPort: 9001, PollMin: 3, QueueWork: true, QueueIntervalSec: 25, DigestTime: ""}
	cfg, err = serverConfigFromApp(app)
	if err != nil {
		t.Fatalf("db-row config: %v", err)
	}
	if cfg.HTTPPort != 9001 || cfg.PollMin != 3 || !cfg.QueueWork || cfg.QueueIntervalSec != 25 || cfg.Digest != "" {
		t.Fatalf("db-row config = %+v", cfg)
	}

	// digest 归一:空/"off"(大小写)= 关;正常 "HH:MM" 保留。
	for dbv, want := range map[string]string{"09:00": "09:00", "off": "", "OFF": "", "": ""} {
		a := settings.DefaultAppSetting()
		a.DigestTime = dbv
		cfg, err = serverConfigFromApp(a)
		if err != nil {
			t.Fatalf("digest %q config: %v", dbv, err)
		}
		if cfg.Digest != want {
			t.Fatalf("digest db=%q → %q; want %q", dbv, cfg.Digest, want)
		}
	}
}

// D2 越界防护(仅非零越界行值可达;0 = 零填内置默认不越界):port>65535、port/poll/interval 负数 → error。
func TestD2ServerConfigFromAppBounds(t *testing.T) {
	cases := []struct {
		name string
		app  settings.AppSetting
		want string // error 子串;空 = 期望成功
	}{
		// 非零越界行值 → error(boot 清晰报错不静默)。
		{"port 65536", settings.AppSetting{HTTPPort: 65536}, "http_port"},
		{"port -1", settings.AppSetting{HTTPPort: -1}, "http_port"},
		{"poll -1", settings.AppSetting{PollMin: -1}, "poll_min"},
		{"interval -1", settings.AppSetting{QueueIntervalSec: -1}, "queue_interval_sec"},
		// 0 = 内置默认,不越界;合法边界通过。
		{"port 0 zero-fill default", settings.AppSetting{}, ""},
		{"port 1 ok", settings.AppSetting{HTTPPort: 1}, ""},
		{"port 65535 ok", settings.AppSetting{HTTPPort: 65535}, ""},
		{"poll 1 ok", settings.AppSetting{PollMin: 1}, ""},
		{"interval 1 ok", settings.AppSetting{QueueIntervalSec: 1}, ""},
	}
	for _, c := range cases {
		_, err := serverConfigFromApp(c.app)
		if c.want == "" {
			if err != nil {
				t.Errorf("%s: unexpected error %v", c.name, err)
			}
			continue
		}
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v; want containing %q", c.name, err, c.want)
		}
	}
}
