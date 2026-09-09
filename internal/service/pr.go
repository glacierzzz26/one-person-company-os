package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/glacierzzz26/one-person-company-os/internal/github"
	"github.com/glacierzzz26/one-person-company-os/internal/task"
)

// Phase 10.5 收尾发 PR(契约 github-roundtrip-pr.md §四 D;回写半环最后一跳)。
// issue→分支工程:来源 GitHub issue 的任务在绑定仓库上于认领起点(委托基线)骑到
// bugfix/feature/<issue#>_<slug> 分支,每相位 OS 提交骑分支;完成点经 finishRun best-effort
// 发 PR(push 分支 + CreatePull),最终合入由人在 GitHub PR 页面完成(OS 永不 merge)。
// 所有 git push / 建分支动作都在宿主侧(经 gitDirCmd / exec),不经 internal/tool/git.go agent 墙。

// prBranch 描述一次收尾 PR 的目标:owner/repo(推与建 PR 的对象 = 工作区 origin 解析)、分支名、
// issue 号与标题(标题经 issue_sync 回链,收尾 body 带 Resolves)。
type prBranch struct {
	owner  string
	repo   string
	branch string
	issue  int64
	title  string
}

// prTarget 判定任务是否落在 PR 闭环上(契约 §一.3 回链法 + §四 D 前置):
// 任务归属项目(ProjectID)+ 项目确有绑定代码源(GetRepoByProject)+ 工作区 origin 是 GitHub URL
// (可 push + 开 PR)+ issue_sync.task_id 回链命中(该工程任务是某 GitHub issue 的载体)。任一不满足
// → (零值, false),调用方静默不建分支不发 PR(非 issue 任务 / 本地-only 项目)。
func (s *Service) prTarget(ctx context.Context, t task.Task) (prBranch, bool) {
	if t.ProjectID == nil || *t.ProjectID == "" || t.WorkspacePath == "" {
		return prBranch{}, false
	}
	if _, err := s.store.GetRepoByProject(ctx, *t.ProjectID); err != nil {
		return prBranch{}, false // 项目无代码源 → 通道 B 不适用
	}
	owner, name, ok := github.ParseOwnerRepo(gitRemoteOrigin(ctx, t.WorkspacePath))
	if !ok {
		return prBranch{}, false // 本地-only / 非 GitHub origin → 无法 push 开 PR
	}
	iss, err := s.store.GetIssueSyncByTask(ctx, t.ID)
	if err != nil {
		return prBranch{}, false // 非 issue 来源 → 不发 PR(走 PR 的圈定仅靠 issue_sync 回链)
	}
	title := iss.Title
	if title == "" {
		title = t.Title
	}
	return prBranch{
		owner: owner, repo: name,
		branch: issueBranchName(iss.IssueNumber, title),
		issue:  iss.IssueNumber, title: title,
	}, true
}

// ---- 分支命名:prefix + slug(决策三.5;确定性,同 issue 重跑同名幂等)----

var bugPrefixRe = regexp.MustCompile(`(?i)bug|fix|defect|crash|修复|缺陷|崩溃|错误`)

// prPrefix issue 标题 → 分支前缀:bug 类词命中 → bugfix,否则 feature。
func prPrefix(title string) string {
	if bugPrefixRe.MatchString(title) {
		return "bugfix"
	}
	return "feature"
}

// issueSlug 把 issue 标题压成 URL 安全短 slug:小写 ASCII 字母/数字 + 连字符,≤48,空 → "issue"。
func issueSlug(title string) string {
	var b strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(title) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		default:
			if b.Len() > 0 && !prevDash {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	slug := strings.Trim(b.String(), "-")
	if len(slug) > 48 {
		slug = strings.TrimRight(slug[:48], "-")
	}
	if slug == "" {
		return "issue"
	}
	return slug
}

// issueBranchName issue → 分支名:<prefix>/<issue#>-<slug>(契约 §一.4;示例 feature/7-add-user-auth)。
func issueBranchName(number int64, title string) string {
	return fmt.Sprintf("%s/%d-%s", prPrefix(title), number, issueSlug(title))
}

// ensureIssueBranch 把工作区骑到 issue 分支(决策三.3,幂等):
//   - 已在目标分支 → no-op;
//   - 分支存在 → checkout(可能带未提交残留一并切换,调用方确保 clean);
//   - 分支不存在 → checkout -b(自当前 HEAD,允许携带既有提交/改动)。
//
// 非 git / 空工作区 → no-op(调用方对 git 前置另行硬判)。scripted 不建分支(决策三.7)。
func ensureIssueBranch(ctx context.Context, ws, branch string) error {
	if ws == "" || !wsIsGit(ctx, ws) {
		return nil
	}
	if cur, err := gitDirCmd(ctx, ws, "branch", "--show-current"); err == nil && strings.TrimSpace(cur) == branch {
		return nil
	}
	if _, err := gitDirCmd(ctx, ws, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch); err == nil {
		_, err = gitDirCmd(ctx, ws, "checkout", branch)
		return err
	}
	_, err := gitDirCmd(ctx, ws, "checkout", "-b", branch)
	return err
}

// originDefaultBranch 解析 origin/HEAD 指向的分支名(PR base 首选;契约 §四 D base=origin/HEAD)。
// symbolic-ref 失败(无 origin/HEAD)→ "" → 调用方走 GitHub default_branch API 兜底。
func originDefaultBranch(ctx context.Context, ws string) string {
	out, err := gitDirCmd(ctx, ws, "symbolic-ref", "--short", "refs/remotes/origin/HEAD")
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(strings.TrimSpace(out), "origin/")
}

// gitAuthEnv 为 https://github.com origin 生成一次 git push 的临时认证 env(决策三.6):
// GIT_CONFIG_COUNT/KEY/VALUE 注入 http.https://github.com/.extraheader =
// Authorization: Basic base64("x-access-token:<token>")。token 只进本次子进程 env,
// 不入 argv、不改写 origin、不落 config。非 https-github origin → nil(明文推)。
func gitAuthEnv(origin, token string) []string {
	if !strings.HasPrefix(origin, "https://github.com/") {
		return nil
	}
	auth := base64.StdEncoding.EncodeToString([]byte("x-access-token:" + token))
	return []string{
		"GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=http.https://github.com/.extraheader",
		"GIT_CONFIG_VALUE_0=Authorization: Basic " + auth,
	}
}

// pushIssueBranch 把当前(issue)分支推到 origin(30s 超时,stdout+stderr 合并)。https-github
// origin 注入 token env(gitAuthEnv),其余明文推。失败带 git 输出回显(供 audit pr_fail / 人工
// publish-pr 重试看真实原因)。
func pushIssueBranch(ctx context.Context, ws, origin, branch, token string) (string, error) {
	pushCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(pushCtx, "git", "-C", ws, "push", "origin", branch)
	cmd.Env = append(os.Environ(), gitAuthEnv(origin, token)...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git push %s (%s): %w: %s", branch, truncate(origin, 60), err, truncate(out.String(), 400))
	}
	return strings.TrimSpace(out.String()), nil
}

// publishRunPR 为一条已到完成点的任务发布收尾 PR(契约 §四 D)。best-effort:
//   - scripted / 无 prTarget(非 issue 任务 / 本地-only)→ 静默 skip,不建分支不发 PR(决策三.7);
//   - 已置 pull_request_url → 幂等返回(不重复建;人工 publish-pr 重试同语义);
//   - 项目无 github_token → audit pr_skip + 返回 nil(写 token 仅认项目,决策三.2;无 token 是配置态,
//     不是运行错误 —— 任务照常 complete);
//   - 否则 ensureIssueBranch(幂等,兜父路径从未经 delegateBaseline)→ push 分支(宿主侧)→
//     POST /repos/{o}/{r}/pulls(base = origin/HEAD,兜底 GitHub default_branch;body 带 Resolves #N)
//     → SetTaskPullRequest + audit pr_opened。
//   - push / 建 PR 失败 → audit pr_fail 并返回错误(任务完成照常不复活;错误供 finishRun 吞、人工
//     改 token 或 GitHub 抖动后经 publish-pr 端点重试)。
func (s *Service) publishRunPR(ctx context.Context, t task.Task, actor string) error {
	if s.engineScripted(ctx, t.CompanyID) {
		return nil
	}
	if t.PullRequestURL != "" {
		return nil // 已发过 → 幂等
	}
	target, ok := s.prTarget(ctx, t)
	if !ok {
		return nil
	}
	ws := t.WorkspacePath
	if !wsIsGit(ctx, ws) {
		_, _ = s.audit(ctx, "task", t.ID, "pr_skip", actor, "workspace is not a git repo")
		return nil
	}
	tok, ok, err := s.OpenProjectSecretCurrent(ctx, *t.ProjectID, ProjectSecretGitHubToken)
	if err != nil {
		return err
	}
	if !ok || tok == "" {
		_, _ = s.audit(ctx, "task", t.ID, "pr_skip", actor,
			fmt.Sprintf("no project github_token secret (write path = project token only); PR for issue #%d not opened — set it on the project code-source card and retry", target.issue))
		return nil
	}
	if err := ensureIssueBranch(ctx, ws, target.branch); err != nil {
		_, _ = s.audit(ctx, "task", t.ID, "pr_fail", actor, "ensure issue branch "+target.branch+": "+err.Error())
		return err
	}
	origin := gitRemoteOrigin(ctx, ws)
	if origin == "" {
		_, _ = s.audit(ctx, "task", t.ID, "pr_fail", actor, "no origin remote to push "+target.branch)
		return fmt.Errorf("task %s: workspace has no origin remote; cannot push issue branch %q", short8(t.ID), target.branch)
	}
	if _, perr := pushIssueBranch(ctx, ws, origin, target.branch, tok); perr != nil {
		_, _ = s.audit(ctx, "task", t.ID, "pr_fail", actor, "push "+target.branch+": "+perr.Error())
		return perr
	}

	// 建 PR:base = origin/HEAD 分支名;解析不出 → GitHub default_branch API 兜底。
	base := originDefaultBranch(ctx, ws)
	var pub github.PullPublisher = s.prPub
	if pub == nil {
		pub = github.NewClient(tok)
	}
	if base == "" {
		if db, hasDB := pub.(interface {
			DefaultBranch(ctx context.Context, owner, repo string) (string, error)
		}); hasDB {
			if b, derr := db.DefaultBranch(ctx, target.owner, target.repo); derr == nil {
				base = b
			}
		}
	}
	if base == "" {
		_, _ = s.audit(ctx, "task", t.ID, "pr_fail", actor,
			fmt.Sprintf("cannot resolve PR base branch for %s/%s (origin/HEAD missing)", target.owner, target.repo))
		return fmt.Errorf("task %s: cannot resolve PR base branch for %s/%s", short8(t.ID), target.owner, target.repo)
	}
	pr, cerr := pub.CreatePull(ctx, target.owner, target.repo, github.PullParams{
		Title: target.title, Head: target.branch, Base: base,
		Body: fmt.Sprintf("Resolves #%d.\n\n_Opened automatically by one-person-company-os when the task completed._", target.issue),
	})
	if cerr != nil {
		_, _ = s.audit(ctx, "task", t.ID, "pr_fail", actor, "create pull "+target.branch+": "+cerr.Error())
		return cerr
	}
	if err := s.store.SetTaskPullRequest(ctx, t.ID, pr.HTMLURL, pr.Number); err != nil {
		return err
	}
	_, _ = s.audit(ctx, "task", t.ID, "pr_opened", actor,
		fmt.Sprintf("%s/%s %s→%s resolves #%d %s", target.owner, target.repo, target.branch, base, target.issue, pr.HTMLURL))
	return nil
}

// finishRun 完成点统一收尾(契约 §四 D:替换 driver/synthesize/plan_driver 三处既存 store.CompleteTask):
// best-effort 发收尾 PR(publishRunPR,内部记 pr_opened/pr_skip/pr_fail,失败不阻塞)→ CompleteTask。
// 任务照常 complete 不复活;发布缺口(无 token / push 抖动 / 无 default_branch)留 pr_skip/pr_fail 审计 +
// 人工 publish-pr 端点重试(改 token 后 / GitHub 瞬时抖动)。
func (s *Service) finishRun(ctx context.Context, t task.Task, result string) error {
	if perr := s.publishRunPR(ctx, t, taskActor(t)); perr != nil {
		_ = perr // 已记 pr_fail;发布 best-effort,不影响任务完成
	}
	_, err := s.store.CompleteTask(ctx, t.ID, result)
	return err
}

// PublishTaskPR 人工重试发 PR(POST /tasks/{id}/publish-pr,契约 §四 E)。完成态门 + 幂等
// (已置 pull_request_url → 直接返回既有,不重复建 PR)+ prTarget + 项目 token 门;任一不满足 → 明确
// 错误(不发网络)。通过 → publishRunPR 真发,失败错误返回(审计 pr_fail 已留痕)。
func (s *Service) PublishTaskPR(ctx context.Context, taskID, actor string) (task.Task, error) {
	t, err := s.store.GetTask(ctx, taskID)
	if err != nil {
		return task.Task{}, err
	}
	if t.Status != "completed" {
		return task.Task{}, fmt.Errorf("%w: task %s is %s, not completed — publish PR only after task completion", ErrInvalid, short8(taskID), t.Status)
	}
	if t.PullRequestURL != "" {
		return t, nil // 幂等:已发过 → 返回既有,不重复建 PR
	}
	if s.engineScripted(ctx, t.CompanyID) {
		return task.Task{}, fmt.Errorf("%w: engine mode is scripted — issue branches/PRs are not created offline", ErrInvalid)
	}
	if _, ok := s.prTarget(ctx, t); !ok {
		return task.Task{}, fmt.Errorf("%w: task %s is not tied to a GitHub issue on a project code source — nothing to publish", ErrInvalid, short8(taskID))
	}
	if tok, ok, oerr := s.OpenProjectSecretCurrent(ctx, *t.ProjectID, ProjectSecretGitHubToken); oerr != nil {
		return task.Task{}, oerr
	} else if !ok || tok == "" {
		return task.Task{}, fmt.Errorf("%w: project %s has no github_token secret — set it on the project code-source card before publishing the PR", ErrInvalid, short8(*t.ProjectID))
	}
	if err := s.publishRunPR(ctx, t, actor); err != nil {
		return task.Task{}, err
	}
	return s.store.GetTask(ctx, taskID)
}
