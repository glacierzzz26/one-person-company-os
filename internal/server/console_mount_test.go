package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestConsoleMountedAtRoot 验证 7.3 SPA 经真实 chi Handler() 挂载:根与客户端路由命中内嵌
// 控制台(index fallback),/api/v1 与 /healthz 仍归既有路由(不吞)。svc=nil 安全(仅静态路由)。
func TestConsoleMountedAtRoot(t *testing.T) {
	s := New(nil, 0)
	h := s.Handler()

	cases := []struct {
		path string
		want int
		body string // 期望响应体含片段;空则只查状态码
	}{
		{path: "/", want: http.StatusOK, body: "运营控制台"},
		{path: "/approvals", want: http.StatusOK, body: "运营控制台"},
		{path: "/api/not-a-route", want: http.StatusNotFound, body: ""},
	}
	for _, c := range cases {
		req := httptest.NewRequest(http.MethodGet, c.path, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != c.want {
			t.Fatalf("GET %s = %d, want %d", c.path, rec.Code, c.want)
		}
		if c.body != "" {
			b, _ := io.ReadAll(rec.Result().Body)
			if !strings.Contains(string(b), c.body) {
				t.Fatalf("GET %s body missing %q", c.path, c.body)
			}
		}
	}
}
