package console

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// h 内嵌占位 ui(测试期仅 index.html),语义与构建后一致。
func do(t *testing.T, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	rec := httptest.NewRecorder()
	Handler().ServeHTTP(rec, req)
	return rec
}

func TestServesIndexAtRoot(t *testing.T) {
	rec := do(t, http.MethodGet, "/")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET / = %d, want 200", rec.Code)
	}
	body, _ := io.ReadAll(rec.Result().Body)
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("content-type = %q, want text/html", ct)
	}
	if !strings.Contains(string(body), "运营控制台") {
		t.Fatalf("index body missing placeholder title")
	}
}

func TestServesClientRouteFallback(t *testing.T) {
	// 无扩展名且不存在的路径 = 客户端路由 → 回 index.html(200)。
	for _, p := range []string{"/approvals", "/tasks/abc", "/spa/deep/route"} {
		rec := do(t, http.MethodGet, p)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s = %d, want 200 (index fallback)", p, rec.Code)
		}
	}
}

func TestMissingAssetIs404(t *testing.T) {
	rec := do(t, http.MethodGet, "/assets/missing.js")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET /assets/missing.js = %d, want 404", rec.Code)
	}
}

func TestAPIPathsNotSwallowed(t *testing.T) {
	for _, p := range []string{"/api/foo", "/api/v1/tasks", "/healthz"} {
		rec := do(t, http.MethodGet, p)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("GET %s = %d, want 404 (owned by server routes)", p, rec.Code)
		}
	}
}

func TestMethodNotAllowed(t *testing.T) {
	rec := do(t, http.MethodPost, "/")
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST / = %d, want 405", rec.Code)
	}
}
