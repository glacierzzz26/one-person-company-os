package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/glacierzzz26/one-person-company-os/internal/endpoint"
	"github.com/glacierzzz26/one-person-company-os/internal/github"
	osrepo "github.com/glacierzzz26/one-person-company-os/internal/repo"
	"github.com/glacierzzz26/one-person-company-os/internal/task"
	"github.com/google/uuid"
)

// 研发 Intake(通道 B:GitHub 自驱,Phase 6.3)。把仓库的 open issues 变成研发队列里
// 的 engineering task。设计 §6.1:每 issue 一次模型调用做分诊,四种处置:
//
//	direct_work → create 一条 engineering task(入队)
//	ask         → 追问(issue 下评论;离线仅落 note)
//	skip        → 跳过附理由
//	merge       → 合并批次:create umbrella engineering task(6.4 起,运行期 driver planner 拆解)
//
// 处置落 issue_sync 账本(UNIQUE(repo_id, issue_number) 原生去重 → webhook 优先 +
// 轮询兜底不重复处理)。分诊模型:OS_ENGINE_MODE=scripted → 确定性脚本(离线冒烟);
// live → 公司 planner/pool 端点真实提问(fail-closed,无端点清晰报错)。
// 真实补丁应用/测试执行属 6.2 边界、planner 拆解属 6.4。

const (
	engDispDirectWork = "direct_work"
	engDispAsk        = "ask"
	engDispSkip       = "skip"
	engDispMerge      = "merge"
)

// IntakeResult 是单仓库一次同步的结果(供 CLI/server 汇总展示)。json tag = /api/v1 契约(phase7)。
type IntakeResult struct {
	Repo         string         `json:"repo"`
	IssuesSeen   int            `json:"issues_seen"`
	Already      int            `json:"already"`       // 账本已存在(webhook/轮询去重)跳过数
	ByDisp       map[string]int `json:"by_disp"`       // disposition → 条数
	CreatedTasks []string       `json:"created_tasks"` // direct_work 建出的 task id
	Asks         []intakeAsk    `json:"asks"`          // 待回帖的追问(live GitHub 源在 issue 下评论)
}

type intakeAsk struct {
	Owner  string `json:"owner"`
	Name   string `json:"name"`
	Number int64  `json:"number"`
	Note   string `json:"note"`
}

// SyncRepos 拉取仓库(companyID 空 = 全部公司)的 open issues 并分诊入库。
// 单仓库失败不中断;返回部分结果 + 聚合错误(CLI 以此决定退出码)。
func (s *Service) SyncRepos(ctx context.Context, companyID string) ([]IntakeResult, error) {
	src, err := issueSource()
	if err != nil {
		return nil, err
	}
	var repos []osrepo.Repo
	if companyID == "" {
		repos, err = s.store.ListAllRepos(ctx)
	} else {
		repos, err = s.store.ListRepos(ctx, companyID)
	}
	if err != nil {
		return nil, err
	}
	if len(repos) == 0 {
		return nil, fmt.Errorf("no registered repos (add one: os repo add --company <id> --name <n> --repo-url <github url> --workspace <dir>)")
	}

	results := make([]IntakeResult, 0, len(repos))
	var errs []string
	for _, r := range repos {
		owner, name, ok := github.ParseOwnerRepo(r.RepoURL)
		if !ok {
			errs = append(errs, fmt.Sprintf("repo %s: cannot parse owner/repo from %q (channel B needs a GitHub repo URL)", r.Name, r.RepoURL))
			continue
		}
		issues, err := src.ListOpenIssues(ctx, owner, name)
		if err != nil {
			errs = append(errs, fmt.Sprintf("repo %s: %v", r.Name, err))
			continue
		}
		res, ierr := s.IntakeIssues(ctx, r, issues)
		if ierr != nil {
			errs = append(errs, fmt.Sprintf("repo %s: %v", r.Name, ierr))
		}
		// ask → 真 GitHub 源在 issue 下回帖追问
		if len(res.Asks) > 0 {
			if c, ok := src.(interface {
				PostComment(ctx context.Context, owner, repo string, number int64, body string) error
			}); ok {
				for _, a := range res.Asks {
					if a.Note == "" {
						continue
					}
					if cerr := c.PostComment(ctx, a.Owner, a.Name, a.Number, "[one-person-company-os] "+a.Note); cerr != nil {
						errs = append(errs, fmt.Sprintf("repo %s: comment #%d: %v", r.Name, a.Number, cerr))
					}
				}
			}
		}
		results = append(results, res)
	}
	if len(errs) > 0 {
		return results, fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return results, nil
}

// IntakeIssues 对给定仓库的一批 issue 逐个分诊处置(供 SyncRepos 与 webhook 共用)。
func (s *Service) IntakeIssues(ctx context.Context, r osrepo.Repo, issues []github.Issue) (IntakeResult, error) {
	res := IntakeResult{Repo: r.Name, ByDisp: map[string]int{}}
	var errs []string
	for _, it := range issues {
		// 幂等:该 (repo, issue) 已有账本 → 视为已处置(webhook 优先 + 轮询兜底不重复建任务)。
		if _, gerr := s.store.GetIssueSync(ctx, r.ID, it.Number); gerr == nil {
			res.Already++
			continue
		} else if !errors.Is(gerr, sql.ErrNoRows) {
			errs = append(errs, fmt.Sprintf("#%d: ledger check: %v", it.Number, gerr))
			continue
		}
		disp, note, err := s.triageIssue(ctx, r.CompanyID, it)
		if err != nil {
			errs = append(errs, fmt.Sprintf("#%d: %v", it.Number, err))
			continue
		}
		var taskID *string
		switch disp {
		case engDispDirectWork, engDispMerge:
			// 6.4 起 merge 与 direct_work 一样物化 umbrella engineering task 入队:
			// 大活是否/如何拆解由运行期 driver 的 planner 决定(≤8 拆 / >8 ask_human)。
			t, terr := s.createIssueTask(ctx, r, it)
			if terr != nil {
				errs = append(errs, fmt.Sprintf("#%d: create task: %v", it.Number, terr))
				continue
			}
			id := t.ID
			taskID = &id
			res.CreatedTasks = append(res.CreatedTasks, id)
			if disp == engDispMerge && note == "" {
				note = "batched; umbrella task queued — planner decomposes at run"
			}
		case engDispAsk:
			if note == "" {
				note = "clarification requested (see issue)"
			}
			res.Asks = append(res.Asks, intakeAsk{Owner: it.RepoOwner, Name: it.RepoName, Number: it.Number, Note: note})
		case engDispSkip:
			if note == "" {
				note = "skipped (no reason given)"
			}
		}
		res.ByDisp[disp]++
		_, err = s.store.UpsertIssueSync(ctx, osrepo.IssueSync{
			ID: uuid.NewString(), CompanyID: r.CompanyID, RepoID: r.ID,
			IssueNumber: it.Number, Title: it.Title, Disposition: disp,
			TaskID: taskID, Note: note, CreatedAt: time.Now().Unix(),
		})
		if err != nil {
			errs = append(errs, fmt.Sprintf("#%d: ledger: %v", it.Number, err))
			continue
		}
		_, _ = s.audit(ctx, "issue", r.ID, "triage", "intake:"+intakeMode(),
			fmt.Sprintf("#%d %q -> %s", it.Number, firstLine(it.Title), disp))
	}
	res.IssuesSeen = len(issues)
	if len(errs) > 0 {
		return res, fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return res, nil
}

// createIssueTask 把 direct_work 的 issue 落地成一条 engineering task。
// 归属:(capability=engineering, role=coding) 解析 agent,尽力而为(无则空,engineering
// 路径不强制 agent)。workspace 沿用仓库登记路径(边界)。endpoints 由 planner/人工指派(6.4)。
func (s *Service) createIssueTask(ctx context.Context, r osrepo.Repo, it github.Issue) (task.Task, error) {
	var capID, agentID *string
	if cap, cerr := s.store.GetCapabilityByCode(ctx, r.CompanyID, "engineering"); cerr == nil && cap.ID != "" {
		capID = &cap.ID
		if a, aerr := s.store.GetAgentByCapabilityAndRole(ctx, cap.ID, "coding"); aerr == nil && a.ID != "" {
			agentID = &a.ID
		}
	}
	link := it.HTMLURL
	if link == "" {
		link = fmt.Sprintf("%s/%s#%d", it.RepoOwner, it.RepoName, it.Number)
	}
	title := strings.TrimSpace(it.Title)
	if title == "" {
		title = fmt.Sprintf("%s#%d", r.Name, it.Number)
	}
	desc := strings.TrimSpace(it.Body)
	if desc != "" {
		desc += "\n\n"
	}
	desc += "source: " + link
	return s.createTask(ctx, TaskParams{
		CompanyID: r.CompanyID, CapabilityID: capID, AgentID: agentID,
		Title: title, Description: desc, ToolName: "engineering",
		Risk: "medium", Workspace: r.WorkspacePath,
	}, "intake")
}

// triageIssue 对单条 issue 做分诊,返回处置 + 附注。scripted → 确定性;live → 模型。
func (s *Service) triageIssue(ctx context.Context, companyID string, it github.Issue) (disp, note string, err error) {
	if strings.EqualFold(os.Getenv("OS_ENGINE_MODE"), "scripted") {
		disp, note = engScriptedTriage()
		return disp, note, nil
	}
	e, err := s.intakeEndpoint(ctx, companyID)
	if err != nil {
		return "", "", err
	}
	out, err := s.modelCall(ctx, e, triagePrompt(it))
	if err != nil {
		return "", "", fmt.Errorf("triage model: %w", err)
	}
	disp, note = parseDisposition(out)
	if disp == "" {
		return "", "", fmt.Errorf("cannot parse disposition from model output: %s", firstLine(out))
	}
	return disp, note, nil
}

// intakeEndpoint 解析分诊用端点:公司 role=planner 的活动端点优先,否则任一活动 pool 端点。
func (s *Service) intakeEndpoint(ctx context.Context, companyID string) (endpoint.Endpoint, error) {
	list, err := s.store.ListEndpoints(ctx, companyID)
	if err != nil {
		return endpoint.Endpoint{}, err
	}
	var pool endpoint.Endpoint
	for _, e := range list {
		if e.Status != "active" {
			continue
		}
		if e.Role == "planner" {
			return e, nil
		}
		if pool.ID == "" {
			pool = e
		}
	}
	if pool.ID != "" {
		return pool, nil
	}
	return endpoint.Endpoint{}, fmt.Errorf("company %s: no active model endpoint (add with 'os endpoint add', or OS_ENGINE_MODE=scripted for offline smoke)", short8(companyID))
}

// issueSource 按 OS_ISSUE_SOURCE 解析 issue 源(github 缺省 fail-closed;fixture 离线冒烟)。
func issueSource() (github.Source, error) {
	switch mode := strings.ToLower(strings.TrimSpace(os.Getenv("OS_ISSUE_SOURCE"))); mode {
	case "fixture":
		p := os.Getenv("OS_FIXTURE_ISSUES")
		if p == "" {
			return nil, fmt.Errorf("OS_ISSUE_SOURCE=fixture requires OS_FIXTURE_ISSUES=<json path>")
		}
		return github.LoadFixture(p)
	case "", "github":
		tok := os.Getenv("OS_GITHUB_TOKEN")
		if tok == "" {
			return nil, fmt.Errorf("OS_GITHUB_TOKEN is required for GitHub issue sync (offline smoke: OS_ISSUE_SOURCE=fixture + OS_FIXTURE_ISSUES)")
		}
		return github.NewClient(tok), nil
	default:
		return nil, fmt.Errorf("OS_ISSUE_SOURCE must be github|fixture (got %q)", mode)
	}
}

func intakeMode() string {
	if strings.EqualFold(os.Getenv("OS_ENGINE_MODE"), "scripted") {
		return "scripted"
	}
	return "live"
}

// ---- 分诊模型调用 ----

func triagePrompt(it github.Issue) string {
	return fmt.Sprintf("You are the R&D intake triage for a one-person company.\n"+
		"GitHub issue #%d:\nTitle: %s\nBody:\n%s\n\n"+
		"Decide how the engineering team should take it in. Reply with EXACTLY ONE JSON object, no prose, no code fence:\n"+
		"  {\"disposition\":\"direct_work\"}                    — well-scoped, start now as one engineering task\n"+
		"  {\"disposition\":\"ask\",\"note\":\"<question>\"}      — need clarification, post one question\n"+
		"  {\"disposition\":\"skip\",\"note\":\"<reason>\"}       — duplicate / out of scope / not actionable\n"+
		"  {\"disposition\":\"merge\",\"note\":\"<note>\"}        — larger effort, planner should decompose it\n",
		it.Number, it.Title, it.Body)
}

// engScriptedTriage 是离线确定性分诊:OS_SCRIPT_TRIAGE
// "" / direct_work(默认) | ask:<q> | skip:<reason> | merge:<note>。
func engScriptedTriage() (string, string) {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("OS_SCRIPT_TRIAGE")))
	switch {
	case v == "", v == engDispDirectWork:
		return engDispDirectWork, ""
	case strings.HasPrefix(v, "ask:"):
		return engDispAsk, strings.TrimSpace(v[len("ask:"):])
	case strings.HasPrefix(v, "skip:"):
		return engDispSkip, strings.TrimSpace(v[len("skip:"):])
	case strings.HasPrefix(v, "merge:"):
		return engDispMerge, strings.TrimSpace(v[len("merge:"):])
	default:
		return engDispDirectWork, ""
	}
}

// parseDisposition 已迁 internal/service/judge.go(8.3 A:JSON 主 + 旧标记兜底)。
