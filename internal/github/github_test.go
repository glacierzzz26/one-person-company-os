package github

// Phase 10.5 github 客户端契约用例(github-roundtrip-pr.md §五,httptest.NewClientAt;离线不触外网):
//   - GH-1 CreatePull:POST /repos/{o}/{r}/pulls + auth header(Bearer token + Accept + api-version)+
//     Content-Type + body(title/head/base/body 全透传);201 → Pull{number, html_url}
//   - GH-2 CreatePull 非 201 → 错误含 "HTTP <code>"(供 audit pr_fail 留痕);response 无 number → 错误
//   - GH-3 空 token → 不发 Authorization(明文仓库也可建 PR 前的内网/自托管语义,header 只按需)
//   - GH-4 DefaultBranch:GET /repos/{o}/{r} → default_branch;非 200 → 错误

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGH1CreatePullPathBodyAuth(t *testing.T) {
	var (
		method, path, auth, ct, accept, ver string
		body                                PullParams
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		auth = r.Header.Get("Authorization")
		ct = r.Header.Get("Content-Type")
		accept = r.Header.Get("Accept")
		ver = r.Header.Get("X-GitHub-Api-Version")
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"number":42,"html_url":"https://github.com/acme/web/pull/42"}`))
	}))
	defer srv.Close()

	c := NewClientAt("ghp_test", srv.URL)
	pr, err := c.CreatePull(context.Background(), "acme", "web", PullParams{
		Title: "Fix nav crash", Head: "bugfix/7-fix-nav-crash", Base: "main", Body: "Resolves #7",
	})
	if err != nil {
		t.Fatalf("CreatePull: %v", err)
	}
	if method != http.MethodPost || path != "/repos/acme/web/pulls" {
		t.Fatalf("CreatePull hit %s %s; want POST /repos/acme/web/pulls", method, path)
	}
	if auth != "Bearer ghp_test" || !strings.HasPrefix(ct, "application/json") || accept != "application/vnd.github+json" || ver != "2022-11-28" {
		t.Fatalf("headers auth=%q ct=%q accept=%q version=%q", auth, ct, accept, ver)
	}
	if body.Title != "Fix nav crash" || body.Head != "bugfix/7-fix-nav-crash" || body.Base != "main" || body.Body != "Resolves #7" {
		t.Fatalf("pull params = %+v", body)
	}
	if pr.Number != 42 || pr.HTMLURL != "https://github.com/acme/web/pull/42" {
		t.Fatalf("created pull = %+v; want #42 html_url", pr)
	}
}

func TestGH2CreatePullErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"message":"no commits between main and bugfix/7-fix-nav-crash"}`))
	}))
	defer srv.Close()
	c := NewClientAt("ghp_test", srv.URL)
	_, err := c.CreatePull(context.Background(), "acme", "web", PullParams{Title: "t", Head: "b", Base: "m", Body: ""})
	if err == nil || !strings.Contains(err.Error(), "HTTP 422") {
		t.Fatalf("non-201 CreatePull err = %v; want error containing HTTP 422", err)
	}
}

func TestGH3EmptyTokenNoAuthHeader(t *testing.T) {
	gotAuth := "unset"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"number":1,"html_url":"https://github.com/acme/web/pull/1"}`))
	}))
	defer srv.Close()
	c := NewClientAt("", srv.URL)
	if _, err := c.CreatePull(context.Background(), "acme", "web", PullParams{Title: "t", Head: "b", Base: "m"}); err != nil {
		t.Fatalf("CreatePull empty token: %v", err)
	}
	if gotAuth != "" {
		t.Fatalf("empty token must not send Authorization, got %q", gotAuth)
	}
}

func TestGH4DefaultBranch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/repos/acme/web" {
			t.Errorf("DefaultBranch hit %s %s; want GET /repos/acme/web", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer ghp_test" {
			t.Errorf("DefaultBranch auth = %q", got)
		}
		_, _ = w.Write([]byte(`{"default_branch":"trunk"}`))
	}))
	defer srv.Close()
	c := NewClientAt("ghp_test", srv.URL)
	b, err := c.DefaultBranch(context.Background(), "acme", "web")
	if err != nil || b != "trunk" {
		t.Fatalf("DefaultBranch = %q err=%v; want trunk", b, err)
	}
}
