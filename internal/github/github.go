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
	return &Client{token: token, http: &http.Client{Timeout: 20 * time.Second}, api: "https://api.github.com"}
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
