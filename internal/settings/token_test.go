package settings

import "testing"

// B2 console 令牌哈希:sha256 hex,确定性;不同令牌不同。
func TestHashConsoleToken(t *testing.T) {
	h1 := HashConsoleToken("tok-123")
	if len(h1) != 64 {
		t.Fatalf("HashConsoleToken len = %d, want 64 (sha256 hex)", len(h1))
	}
	if h2 := HashConsoleToken("tok-123"); h1 != h2 {
		t.Fatalf("HashConsoleToken not deterministic: %q vs %q", h1, h2)
	}
	if h3 := HashConsoleToken("tok-456"); h1 == h3 {
		t.Fatal("HashConsoleToken different tokens collide")
	}
	if h4 := HashConsoleToken("tok-1234"); h1 == h4 {
		t.Fatal("HashConsoleToken prefix-extension collision")
	}
}
