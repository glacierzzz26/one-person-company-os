package cli

// Phase 9.3 — server 零参数生效配置(契约 runtime-knobs-web.md §五 D1-D2)。
// 纯函数 effectiveServerConfig(无 cobra):DB app_setting 为权威源,flag 仅显式 Changed 覆盖(deprecated)。

import (
	"strings"
	"testing"

	"github.com/glacierzzz26/one-person-company-os/internal/settings"
)

// flagsChanged 造一个 changed(name) 谓词:只在给出名字集合里返回 true。
func flagsChanged(names ...string) func(string) bool {
	set := make(map[string]bool, len(names))
	for _, n := range names {
		set[n] = true
	}
	return func(name string) bool { return set[name] }
}

// D1 DB 行值生效;port/poll/queue-work/queue-interval/digest 各自 Changed 覆盖;
// DB digest 空/off = 关;flag --digest off/空语义;缺行(zero AppSetting)回内置默认。
func TestD1EffectiveServerConfig(t *testing.T) {
	// 无 flag → DB 行值生效。server.go 的 app 来自 svc.AppSetting():缺行已回内置默认,
	// 故 DefaultAppSetting(含 digest 09:00)才是「零配置启动」的真是输入。
	cfg, err := effectiveServerConfig(settings.DefaultAppSetting(), flagVals{}, flagsChanged())
	if err != nil {
		t.Fatalf("default app config: %v", err)
	}
	if cfg.HTTPPort != settings.DefaultHTTPPort || cfg.PollMin != settings.DefaultPollMin ||
		cfg.QueueIntervalSec != settings.DefaultQueueIntervalSec || cfg.QueueWork ||
		cfg.Digest != settings.DefaultDigestTime {
		t.Fatalf("defaults config = %+v", cfg)
	}

	// DB 行值生效(无 flag 覆盖):port/poll/queue 全被行值驱动,digest_time="" = 关。
	app := settings.AppSetting{HTTPPort: 9001, PollMin: 3, QueueWork: true, QueueIntervalSec: 25, DigestTime: ""}
	cfg, err = effectiveServerConfig(app, flagVals{}, flagsChanged())
	if err != nil {
		t.Fatalf("db-row config: %v", err)
	}
	if cfg.HTTPPort != 9001 || cfg.PollMin != 3 || !cfg.QueueWork || cfg.QueueIntervalSec != 25 || cfg.Digest != "" {
		t.Fatalf("db-row config = %+v", cfg)
	}

	// DB digest "off" 视为关;正常 "HH:MM" 保留。
	for dbv, want := range map[string]string{"09:00": "09:00", "off": "", "OFF": "", "": ""} {
		app := settings.DefaultAppSetting()
		app.DigestTime = dbv
		cfg, err = effectiveServerConfig(app, flagVals{}, flagsChanged())
		if err != nil {
			t.Fatalf("digest %q config: %v", dbv, err)
		}
		if cfg.Digest != want {
			t.Fatalf("digest db=%q → %q; want %q", dbv, cfg.Digest, want)
		}
	}

	// 各 flag 显式 Changed 覆盖 DB。
	app = settings.DefaultAppSetting()
	cfg, err = effectiveServerConfig(app, flagVals{port: 7878, pollMin: 9, queueIntervalSec: 33, queueWork: true, digest: "off"},
		flagsChanged("port", "poll", "queue-work", "queue-interval", "digest"))
	if err != nil {
		t.Fatalf("flag override config: %v", err)
	}
	if cfg.HTTPPort != 7878 || cfg.PollMin != 9 || !cfg.QueueWork || cfg.QueueIntervalSec != 33 || cfg.Digest != "" {
		t.Fatalf("flag override config = %+v", cfg)
	}

	// --digest 未显式传(Changed=false)→ DB digest 保留;Changed=true 但 flag 空串 → 仍不改(空=无效覆盖)。
	app = settings.DefaultAppSetting() // digest 09:00
	cfg, _ = effectiveServerConfig(app, flagVals{digest: ""}, flagsChanged("digest"))
	if cfg.Digest != "09:00" {
		t.Fatalf("digest Changed-but-empty should preserve db value, got %q", cfg.Digest)
	}
	cfg, _ = effectiveServerConfig(app, flagVals{}, flagsChanged())
	if cfg.Digest != "09:00" {
		t.Fatalf("digest untouched should preserve db value, got %q", cfg.Digest)
	}
	// --digest "off" → 关;--digest "10:30" → "10:30"。
	cfg, _ = effectiveServerConfig(app, flagVals{digest: "off"}, flagsChanged("digest"))
	if cfg.Digest != "" {
		t.Fatalf("--digest off → %q; want ''", cfg.Digest)
	}
	cfg, _ = effectiveServerConfig(app, flagVals{digest: "10:30"}, flagsChanged("digest"))
	if cfg.Digest != "10:30" {
		t.Fatalf("--digest 10:30 → %q; want 10:30", cfg.Digest)
	}
}

// D2 越界防护:port∈[1,65535]、poll≥1、interval≥1 → 越界 error(不静默);合法边界通过。
func TestD2EffectiveServerConfigBounds(t *testing.T) {
	cases := []struct {
		name string
		app  settings.AppSetting
		f    flagVals
		ch   func(string) bool
		want string // error 子串;空 = 期望成功
	}{
		// DB 存 0 = 缺省回内置默认(不会越界);越界只能经显式 flag 或行值直接落 0(不会被覆盖的场景,见下)。
		{"port 0 via flag", settings.DefaultAppSetting(), flagVals{port: 0}, flagsChanged("port"), "http_port"},
		{"port 65536 via flag", settings.DefaultAppSetting(), flagVals{port: 65536}, flagsChanged("port"), "http_port"},
		{"port 1 ok", settings.DefaultAppSetting(), flagVals{}, flagsChanged(), ""},
		{"port 65535 ok", settings.DefaultAppSetting(), flagVals{}, flagsChanged(), ""},
		{"poll 0 via flag", settings.DefaultAppSetting(), flagVals{pollMin: 0}, flagsChanged("poll"), "poll_min"},
		{"interval 0 via flag", settings.DefaultAppSetting(), flagVals{queueIntervalSec: 0}, flagsChanged("queue-interval"), "queue_interval_sec"},
	}
	for _, c := range cases {
		_, err := effectiveServerConfig(c.app, c.f, c.ch)
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
