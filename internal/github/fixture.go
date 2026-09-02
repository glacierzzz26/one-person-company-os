package github

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
)

// Fixture 是离线确定性 issue 源(OS_ISSUE_SOURCE=fixture + OS_FIXTURE_ISSUES=<json 路径>)。
// 无真实 GitHub token / 无公网时,用本地 fixture 验证 轮询+intake sync 全链路;
// 条目字段同 Issue,owner/name 与注册仓库 repo_url 解析结果一致才被路由。
type Fixture struct {
	issues []Issue
}

type fixtureEntry struct {
	RepoOwner string `json:"repo_owner"`
	RepoName  string `json:"repo_name"`
	Number    int64  `json:"number"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	HTMLURL   string `json:"html_url"`
}

func LoadFixture(path string) (*Fixture, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("load issue fixture %s: %w", path, err)
	}
	var raw []fixtureEntry
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, fmt.Errorf("parse issue fixture %s: %w", path, err)
	}
	f := &Fixture{}
	for _, e := range raw {
		f.issues = append(f.issues, Issue{
			RepoOwner: e.RepoOwner, RepoName: e.RepoName, Number: e.Number,
			Title: e.Title, Body: e.Body, HTMLURL: e.HTMLURL,
		})
	}
	return f, nil
}

func (f *Fixture) ListOpenIssues(ctx context.Context, owner, repo string) ([]Issue, error) {
	out := []Issue{}
	for _, it := range f.issues {
		if it.RepoOwner == owner && it.RepoName == repo {
			out = append(out, it)
		}
	}
	return out, nil
}
