package schedule

import (
	"testing"
	"time"
)

func at(s string, t *testing.T) time.Time {
	t.Helper()
	ts, err := time.ParseInLocation("2006-01-02 15:04", s, time.FixedZone("CST", 8*3600))
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return ts
}

// Parse:合法/非法/不调度分支。
func TestParseValidAndInvalid(t *testing.T) {
	valid := []string{
		"0 9 * * *", "*/30 * * * *", "0 9 * * 1-5", "0 9 1 * *",
		"0 0 * * 7", // 7 = 周日别名
		"0,15,30,45 * * * *", "5-10/2 * * * *", "0 0 13 * 5", "59 23 31 12 0",
	}
	for _, s := range valid {
		if _, ok, err := Parse(s); err != nil || !ok {
			t.Errorf("Parse(%q) should be valid+scheduled, got ok=%v err=%v", s, ok, err)
		}
	}
	for _, s := range []string{"", "off", "OFF", "  "} {
		if _, ok, err := Parse(s); err != nil || ok {
			t.Errorf("Parse(%q) should be not-scheduled, got ok=%v err=%v", s, ok, err)
		}
	}
	invalid := []string{
		"0 9 * *",       // 4 段
		"0 9 * * * *",   // 6 段
		"60 * * * *",    // 分越界
		"* 24 * * *",    // 时越界
		"0 9 * * MON",   // 名字不支持
		"@daily",        // 别名不支持
		"abc",           // 非数字
		"0 9 * * ?",     // ? 不支持
		"0 0 0 * *",     // 日 0 越界
		"0 0 * 13 *",    // 月越界
		"0 9 * * 8",     // 周越界(仅 0-7)
		"*/0 * * * *",   // 步进 0
		"* */x * * *",   // 步进非数
		"1-5/0 * * * *", // 范围步进 0
		"23-5 * * * *",  // 反向范围不支持
		"*, 5 * * * *",  // 空表项(带空格分离是 6 段,先被 len 拦;空项本身也拦)
	}
	for _, s := range invalid {
		if _, ok, err := Parse(s); err == nil || ok {
			t.Errorf("Parse(%q) should error, got ok=%v err=%v", s, ok, err)
		}
	}
}

// Next:命中推进 + 边界(次日常规 / 周内工作日 / dom OR dow / 跨月跨年 / 不存在的 2/31)。
func TestNext(t *testing.T) {
	cases := []struct {
		cron string
		now  string
		want string // 空 = 永不命中(零值)
	}{
		{"0 9 * * *", "2026-09-08 09:00", "2026-09-09 09:00"}, // 恰在点 → 严格晚于 → 明日
		{"0 9 * * *", "2026-09-08 08:00", "2026-09-08 09:00"}, // 未到点 → 今日
		{"*/30 * * * *", "2026-09-08 09:00", "2026-09-08 09:30"},
		{"*/30 * * * *", "2026-09-08 09:29", "2026-09-08 09:30"},
		{"0 9 * * 1-5", "2026-09-11 09:00", "2026-09-14 09:00"}, // 周五(11)→ 下周一(14)
		{"0 0 13 * 5", "2026-09-11 12:00", "2026-09-13 00:00"},  // 周五也 13 号之前?09-13 是周日,09-11 周五已过点 → 命中 13 号(dom OR dow)
		{"0 9 31 12 *", "2026-12-30 00:00", "2026-12-31 09:00"}, // 12/31
		{"0 9 1 1 *", "2026-12-31 12:00", "2027-01-01 09:00"},   // 跨年
		{"0 0 29 2 *", "2026-03-01 00:00", "2028-02-29 00:00"},  // 闰年 2/29(2027 非闰)
		{"0 0 31 2 *", "2026-01-01 00:00", ""},                  // 2/31 永不 → 零值
		{"0 9 1 1 *", "2026-09-08 10:00", "2027-01-01 09:00"},   // 常规远跳
	}
	for _, c := range cases {
		cr, ok, err := Parse(c.cron)
		if err != nil || !ok {
			t.Fatalf("setup cron %q: ok=%v err=%v", c.cron, ok, err)
		}
		// 秒可带;把 now 按分钟语义读(ParseInLocation 不含秒,秒=0 够测)。
		got := cr.Next(at(c.now, t))
		if c.want == "" {
			if !got.IsZero() {
				t.Errorf("Next(%q from %s) = %s, want zero(non-occurring)", c.cron, c.now, got.Format("2006-01-02 15:04"))
			}
			continue
		}
		wantT := at(c.want, t)
		if !got.Equal(wantT) {
			t.Errorf("Next(%q from %s) = %s, want %s", c.cron, c.now, got.Format("2006-01-02 15:04"), c.want)
		}
	}
}

// Next 严格递增:推进两次必单调(防同一命中重复)。
func TestNextStrictlyAdvances(t *testing.T) {
	cr, _, err := Parse("* * * * *")
	if err != nil {
		t.Fatal(err)
	}
	now := at("2026-09-08 09:00", t)
	first := cr.Next(now)
	second := cr.Next(first)
	if !first.Before(second) {
		t.Errorf("Next should strictly advance: %s then %s", first, second)
	}
	// 每分命中:first 应恰为 now+1min。
	want := now.Add(time.Minute)
	if !first.Equal(want) {
		t.Errorf("every-minute Next from %s = %s, want %s", now, first, want)
	}
}
