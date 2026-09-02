package server

import (
	"testing"
	"time"
)

// TestNextDigestFire 摘要触发时刻计算:未到点 → 今日;过点(>2min 宽限)→ 次日;跨午夜正确。
func TestNextDigestFire(t *testing.T) {
	loc := time.FixedZone("CST", 8*3600)
	date := func(s string) time.Time {
		ts, err := time.ParseInLocation("2006-01-02 15:04", s, loc)
		if err != nil {
			t.Fatalf("parse %q: %v", s, err)
		}
		return ts
	}
	cases := []struct {
		now  string
		want string
	}{
		{"2026-09-02 08:00", "2026-09-02 09:00"}, // 未到点 → 今日
		{"2026-09-02 09:00", "2026-09-02 09:00"}, // 恰好在点 → 今日(2min 宽限内)
		{"2026-09-02 09:01", "2026-09-02 09:00"}, // 宽限内 → 今日
		{"2026-09-02 10:00", "2026-09-03 09:00"}, // 过点 → 次日
		{"2026-09-02 23:59", "2026-09-03 09:00"}, // 跨午夜
	}
	for _, c := range cases {
		got := nextDigestFire(date(c.now), 9, 0).In(loc).Format("2006-01-02 15:04")
		if got != c.want {
			t.Errorf("nextDigestFire(%s, 9,0) = %s, want %s", c.now, got, c.want)
		}
	}
}

// TestSetDigestTime 摘要时刻配置解析与关闭。
func TestSetDigestTime(t *testing.T) {
	s := &Server{}
	if err := s.SetDigestTime("09:30"); err != nil {
		t.Fatalf("SetDigestTime: %v", err)
	}
	if !s.DigestEnabled() || s.DigestTime() != "09:30" {
		t.Errorf("digest = enabled:%v time:%q, want true 09:30", s.DigestEnabled(), s.DigestTime())
	}
	if err := s.SetDigestTime("off"); err != nil {
		t.Fatalf("SetDigestTime(off): %v", err)
	}
	if s.DigestEnabled() {
		t.Error("digest should be disabled after off")
	}
	for _, bad := range []string{"9:00am", "24:00", "10:99", "abc"} {
		s := &Server{}
		if err := s.SetDigestTime(bad); err == nil {
			t.Errorf("SetDigestTime(%q) should error", bad)
		}
	}
}
