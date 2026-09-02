package github

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Webhook 是 GitHub issues webhook payload 的最小形状(OS 只消费 issue 事件)。
// 事件头 x-github-event 取值 issues;action ∈ opened | reopened | edited 都触发
// triage,closed 忽略(关闭即无需研发)。PR 事件(action=opened,带 pull_request)不消费。
type Webhook struct {
	Action string `json:"action"`
	Issue  struct {
		Number    int64           `json:"number"`
		Title     string          `json:"title"`
		Body      string          `json:"body"`
		HTMLURL   string          `json:"html_url"`
		Pull      json.RawMessage `json:"pull_request"`
	} `json:"issue"`
	Repository struct {
		FullName string `json:"full_name"` // owner/name
	} `json:"repository"`
}

// DecodeWebhook 解析 /api/webhook/github 请求体。无效 JSON → error。
func DecodeWebhook(r *http.Request) (*Webhook, error) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	var w Webhook
	if err := json.Unmarshal(body, &w); err != nil {
		return nil, fmt.Errorf("decode github webhook: %w", err)
	}
	return &w, nil
}

// ToIssue 把 webhook 里的 issue 归一到 Issue。PR(pull_request 存在)→ ok=false。
func (w *Webhook) ToIssue() (Issue, bool) {
	if len(w.Issue.Pull) > 0 && string(w.Issue.Pull) != "null" {
		return Issue{}, false
	}
	owner, name, ok := ParseOwnerRepo("https://github.com/" + w.Repository.FullName)
	if !ok {
		owner = w.Repository.FullName
		name = ""
	}
	return Issue{
		RepoOwner: owner, RepoName: name, Number: w.Issue.Number,
		Title: w.Issue.Title, Body: w.Issue.Body, HTMLURL: w.Issue.HTMLURL,
	}, true
}
