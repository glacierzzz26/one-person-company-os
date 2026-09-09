package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Issue 是通道 B(研发 Intake)的最小 issue 视图——真实 GitHub issue 与 webhook
// payload 都归一到这个形状;离线 fixture 也产出它(owner/repo 与注册仓库的
// repo_url 解析结果一致即可路由)。
type Issue struct {
	RepoOwner string // owner(repo_url 解析 / webhook repository.full_name 前半)
	RepoName  string
	Number    int64
	Title     string
	Body      string
	HTMLURL   string
}

// Source 拉取某仓库的 open issues。真实 GitHub 用 Client;离线冒烟用 Fixture。
type Source interface {
	ListOpenIssues(ctx context.Context, owner, repo string) ([]Issue, error)
}

// Client 是 GitHub REST 客户端。token 由调用方经 NewClient(token) 注入(service 按公司解
// github_token 机密,9.3+ 不再读 env),不入库不入日志。
type Client struct {
	token string
	http  *http.Client
	api   string
}

func NewClient(token string) *Client {
	return NewClientAt(token, "https://api.github.com")
}

// NewClientAt 构造指向自定义 apiBase 的 Client(测试经 httptest.Server 覆写 base;生产用 NewClient)。
func NewClientAt(token, apiBase string) *Client {
	return &Client{token: token, http: &http.Client{Timeout: 20 * time.Second}, api: strings.TrimSuffix(apiBase, "/")}
}

var ownerRepoRe = regexp.MustCompile(`(?i)(?:github\.com[:/]|github\.com/|git@github\.com:)([^/\s]+)/([^/\s#]+?)(?:\.git)?$`)

// ParseOwnerRepo 从 repo_url 提取 owner/repo。支持
// https://github.com/o/r / https://github.com/o/r.git / git@github.com:o/r.git。
// 解析不出 → ok=false(如本地路径仓库,通道 B 无法路由,需 github URL)。
func ParseOwnerRepo(repoURL string) (owner, name string, ok bool) {
	m := ownerRepoRe.FindStringSubmatch(strings.TrimSpace(repoURL))
	if m == nil {
		return "", "", false
	}
	return strings.TrimSuffix(m[1], ":"), strings.TrimSuffix(m[2], ".git"), true
}

// ListOpenIssues 拉某仓库 open issues(GitHub issue API 会把 PR 一并返回,pull_request
// 字段存在即排除)。
func (c *Client) ListOpenIssues(ctx context.Context, owner, repo string) ([]Issue, error) {
	u := fmt.Sprintf("%s/repos/%s/%s/issues?state=open&per_page=100", c.api, url.PathEscape(owner), url.PathEscape(repo))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	c.auth(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github %s/%s: HTTP %d: %s", owner, repo, resp.StatusCode, truncate(string(body), 300))
	}
	var raw []struct {
		Number  int64           `json:"number"`
		Title   string          `json:"title"`
		Body    string          `json:"body"`
		HTMLURL string          `json:"html_url"`
		Pull    json.RawMessage `json:"pull_request"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parse github issues: %w", err)
	}
	out := make([]Issue, 0, len(raw))
	for _, it := range raw {
		if len(it.Pull) > 0 && string(it.Pull) != "null" {
			continue // PR 不是 issue
		}
		out = append(out, Issue{RepoOwner: owner, RepoName: repo, Number: it.Number,
			Title: it.Title, Body: it.Body, HTMLURL: it.HTMLURL})
	}
	return out, nil
}

// PostComment 在 issue 下评论(triage 处置 ask 追问用)。离线 fixture 无此能力。
func (c *Client) PostComment(ctx context.Context, owner, repo string, number int64, body string) error {
	u := fmt.Sprintf("%s/repos/%s/%s/issues/%d/comments", c.api, url.PathEscape(owner), url.PathEscape(repo), number)
	payload, _ := json.Marshal(map[string]string{"body": body})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	c.auth(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("github comment %s/%s#%d: HTTP %d: %s", owner, repo, number, resp.StatusCode, truncate(string(b), 300))
	}
	return nil
}

func (c *Client) auth(req *http.Request) {
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// ---- Phase 10.5 PR 发布(契约 github-roundtrip-pr.md §四 D:push 分支后开 PR / 兜底 base) ----

// Pull 是最小 PR 视图(收尾幂等账本落 pull_request_url/number 用)。
type Pull struct {
	Number  int64  `json:"number"`
	HTMLURL string `json:"html_url"`
}

// PullParams 建 PR 入参(映射 POST /repos/{o}/{r}/pulls 请求体)。
type PullParams struct {
	Title string `json:"title"`
	Head  string `json:"head"` // 源分支名(feature/<issue#>-<slug>)
	Base  string `json:"base"` // 目标分支名(origin/HEAD 解析 → 分支名,兜底 API default_branch)
	Body  string `json:"body"` // 收尾带 "Resolves #<issue>"
}

// PullPublisher 发布 PR(service 收尾 / 人工 publish-pr 用)。真实实现 = *Client;
// service 测试注入 fake 记录 CreatePull 参(不触网)。
type PullPublisher interface {
	CreatePull(ctx context.Context, owner, repo string, p PullParams) (Pull, error)
}

var _ PullPublisher = (*Client)(nil)

// DefaultBranch 返回仓库 default_branch(PR base 兜底:origin/HEAD symbolic-ref 失效时用)。
func (c *Client) DefaultBranch(ctx context.Context, owner, repo string) (string, error) {
	u := fmt.Sprintf("%s/repos/%s/%s", c.api, url.PathEscape(owner), url.PathEscape(repo))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	c.auth(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("github repo %s/%s: HTTP %d: %s", owner, repo, resp.StatusCode, truncate(string(body), 300))
	}
	var raw struct {
		DefaultBranch string `json:"default_branch"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return "", fmt.Errorf("parse github repo: %w", err)
	}
	if raw.DefaultBranch == "" {
		return "", fmt.Errorf("github repo %s/%s: no default_branch in response", owner, repo)
	}
	return raw.DefaultBranch, nil
}

// CreatePull 在仓库开一条 PR(收尾发 PR 的唯一出口)。成功 → Pull{number, html_url};201 以外的
// 状态码按错误带正文返回(与 PostComment 错误形状一致,供 audit pr_fail 留痕)。
func (c *Client) CreatePull(ctx context.Context, owner, repo string, p PullParams) (Pull, error) {
	u := fmt.Sprintf("%s/repos/%s/%s/pulls", c.api, url.PathEscape(owner), url.PathEscape(repo))
	payload, err := json.Marshal(p)
	if err != nil {
		return Pull{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(payload))
	if err != nil {
		return Pull{}, err
	}
	c.auth(req)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return Pull{}, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		return Pull{}, fmt.Errorf("github pull %s/%s (%s→%s): HTTP %d: %s", owner, repo, p.Head, p.Base, resp.StatusCode, truncate(string(body), 300))
	}
	var pr Pull
	if err := json.Unmarshal(body, &pr); err != nil {
		return Pull{}, fmt.Errorf("parse github pull response: %w", err)
	}
	if pr.Number == 0 {
		return Pull{}, fmt.Errorf("github pull %s/%s: no number in response", owner, repo)
	}
	return pr, nil
}
