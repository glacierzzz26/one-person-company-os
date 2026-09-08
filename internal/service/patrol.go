package service

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/glacierzzz26/one-person-company-os/internal/pipeline"
	"github.com/glacierzzz26/one-person-company-os/internal/task"
)

// Phase 10.2 — ops_patrol 巡检驱动(契约 docs/phase10/design/ops-patrol-schedule.md §3.5/§3.6)。
// runPatrol 是 worker 认领 kind==ops_patrol 的 run 时的独立执行体(分流在 execution.go runClaimed):
//   - 项目目录内只读巡检(live = 委派集成 agent CLI 照流水线 description 检查项干;scripted = OS 写
//     确定性 fixture 报告,离线红线)→ 报告/原始证据落 <root>/patrol/<taskID>.md;
//   - OS git 提交该报告文件(只 `git add -- patrol/<file>`,不吞用户/无关改动);
//   - OS 读回报告正文(限长)→ 判读(只对证据文本,D4):live = 网关模型(frontier;reviewer→writer 兜底);
//     scripted = 确定性桩(OS_SCRIPT_PATROL 仅 scripted 分支可达,缺省 ok;产品不可达);
//   - ok → 完成任务;非 ok → notifyCompany(best-effort)+ fix&high&同项目 active bugfix 链拉 / 留人。
// 完成态 = task completed + result 文本一体可查(result 以 "patrol:" 标记;读端点白名单)。
// 失败(无报告/不可解析/委派失败/超时)→ engFail 不默认绿(requeue 按 attempt/max_attempts)。

const (
	// PatrolDirName 报告子目录(项目根下;OS 类型化工作目录)。导出 = 契约级文件布局,server 读端点 join 用。
	PatrolDirName = "patrol"
	// PatrolReportCap 报告读回/判读/HTTP 读端点限长(字节;契约 3.8)。导出供 server 读端点同源。
	PatrolReportCap = 64 * 1024
	// PatrolResultTag task.result 前缀标记(确系巡检产物;Web/读端点据此显形态)。
	PatrolResultTag = "patrol:"
)

// runPatrol 执行一次巡检 run(认领后同步跑完:报告产出 → 提交 → 判读 → 完成/处置)。
func (s *Service) runPatrol(ctx context.Context, workerID string, t task.Task, pl pipeline.Pipeline) error {
	// 前置审批(risk=high 或 approval policy),与 runEngineering/runClaimed 同语义。
	if need, reason, err := s.needsApproval(ctx, t); err != nil {
		return err
	} else if need {
		return s.requestApproval(ctx, t, reason)
	}
	if _, err := s.store.MarkTaskRunning(ctx, t.ID); err != nil {
		return err
	}
	if _, err := s.audit(ctx, "task", t.ID, "patrol_start", taskActor(t), "ops patrol "+pl.Name+" kind=ops_patrol"); err != nil {
		return err
	}
	runCtx, cancel := runContext(ctx, t.TimeoutSec)
	defer cancel()

	ws := strings.TrimSpace(t.WorkspacePath)
	if ws == "" {
		return s.engFail(ctx, t, "patrol", runCtx, fmt.Errorf("patrol task %s has no workspace path", t.ID))
	}
	reportRel := filepath.Join(PatrolDirName, t.ID+".md")
	reportAbs := filepath.Join(ws, reportRel)

	// 产出报告:live = 委派 claude 只读巡检(报告由 agent 写);scripted = OS 写确定性 fixture 报告。
	scripted := s.engineScripted(ctx, t.CompanyID)
	if scripted {
		if err := writeScriptedPatrolReport(t, pl, reportAbs); err != nil {
			return s.engFail(ctx, t, "patrol", runCtx, err)
		}
	} else {
		if err := s.delegatePatrol(runCtx, t, pl, reportAbs); err != nil {
			return s.engFail(ctx, t, "patrol", runCtx, err)
		}
	}

	// OS 提交报告(只 add 该文件)。失败 → engFail,不默认绿。
	if err := s.commitPatrolReport(runCtx, ws, reportRel, pl, t); err != nil {
		return s.engFail(ctx, t, "patrol", runCtx, err)
	}
	if _, err := s.audit(ctx, "task", t.ID, "patrol_delegate", taskActor(t),
		fmt.Sprintf("report=%s pipeline=%s", reportRel, pl.Name)); err != nil {
		return err
	}

	// OS 读回报告正文(限长)→ 判读(只对证据)。
	report, _, err := readReportCapped(reportAbs)
	if err != nil {
		return s.engFail(ctx, t, "patrol", runCtx, fmt.Errorf("read report %s: %w", reportRel, err))
	}
	var raw string
	if scripted {
		raw = scriptedPatrolOutput()
	} else {
		out, err := s.patrolJudge(runCtx, t, pl, report)
		if err != nil {
			return s.engFail(ctx, t, "patrol", runCtx, err)
		}
		raw = out
	}
	v, ok := parsePatrolVerdict(raw)
	if !ok {
		return s.engFail(ctx, t, "patrol", runCtx,
			fmt.Errorf("cannot parse patrol verdict from output: %s", firstLine(raw)))
	}

	// 完成任务(result 一体可查:标记 + 报告路径 + ok/severity/action + summary 首行)。
	result := fmt.Sprintf("%s%s|ok=%t|severity=%s|action=%s|%s",
		PatrolResultTag, reportRel, v.Ok, v.Severity, v.Action, firstLine(v.Summary))
	if _, err := s.store.CompleteTask(ctx, t.ID, result); err != nil {
		return err
	}
	if _, err := s.audit(ctx, "task", t.ID, "patrol_complete", taskActor(t), patrolVerdictAudit(v, reportRel)); err != nil {
		return err
	}

	// 非 ok → 处置(通知 + 链拉 / 留人;dispose best-effort 不回传错误 —— 任务已完成)。
	if !v.Ok {
		s.disposePatrolFindings(ctx, t, pl, v)
	}
	return nil
}

// delegatePatrol 委派一次只读巡检(claudeDelegator 等,同 writer 族;cwd=项目根)。报告由 agent
// 写到 patrol/<taskID>.md;委派结束 OS 校验报告文件确实产出(无 → 硬错误,不默认绿)。
func (s *Service) delegatePatrol(ctx context.Context, t task.Task, pl pipeline.Pipeline, reportAbs string) error {
	ws := t.WorkspacePath
	if !wsIsGit(ctx, ws) {
		return fmt.Errorf("patrol delegation requires a git workspace (task workspace=%q is not a git repo)", ws)
	}
	if err := os.MkdirAll(filepath.Join(ws, PatrolDirName), 0o755); err != nil {
		return fmt.Errorf("mkdir %s/: %w", PatrolDirName, err)
	}
	family, err := s.agentCLI(ctx, t.CompanyID)
	if err != nil {
		return fmt.Errorf("resolve agent cli: %w", err)
	}
	d, err := s.delegatorFor(family)
	if err != nil {
		return err
	}
	spec := DelegateSpec{
		Workspace: ws,
		Brief:     patrolBrief(t, pl, ws),
		ModelEnv:  s.delegateEnv(ctx, t),
		Timeout:   delegateTimeout(ctx, t, engCallCtx{}),
		Family:    family,
	}
	if _, err := d.Delegate(ctx, spec); err != nil {
		return fmt.Errorf("patrol delegate (family=%s): %w", family, err)
	}
	if _, err := os.Stat(reportAbs); err != nil {
		return fmt.Errorf("patrol delegate produced no report file %s — the inspector must write its findings there", reportAbs)
	}
	return nil
}

// commitPatrolReport 提交巡检报告:只 `git add -- patrol/<rel>` + OS 唯一提交者 commit(8.3 C1 语义)。
// 不 add -A → 绝不吞用户未提交改动/残留(契约 §六 脏工作树)。文件无实质改动 → commitDelegation skip。
func (s *Service) commitPatrolReport(ctx context.Context, ws, reportRel string, pl pipeline.Pipeline, t task.Task) error {
	if !wsIsGit(ctx, ws) {
		return fmt.Errorf("patrol report commit requires a git workspace (workspace=%q)", ws)
	}
	if _, err := gitDirCmd(ctx, ws, "add", "--", reportRel); err != nil {
		return err
	}
	return commitDelegation(ctx, ws, fmt.Sprintf("os-patrol: %s task=%s", pl.Name, short8(t.ID)))
}

// disposePatrolFindings 非 ok 处置(契约 3.6):通知(best-effort)→ fix&high&同项目 active bugfix 则
// 链式拉起 bugfix run(actor=system:patrol_chain;request=summary;变更仍过既有审批门)→ 成功审计
// patrol_chain;链缺/忙/非 fix → 审计 patrol_manual 留人。全程 best-effort,不回传错误(任务已完成)。
func (s *Service) disposePatrolFindings(ctx context.Context, t task.Task, pl pipeline.Pipeline, v patrolVerdict) {
	reportRel := filepath.Join(PatrolDirName, t.ID+".md")
	if err := s.notifyCompany(ctx, t.CompanyID, patrolAlertText(pl, t, v, reportRel)); err != nil {
		log.Printf("notify [patrol] pipeline %s: %v", short8(pl.ID), err)
	}
	if !(v.Action == "fix" && v.Severity == "high") {
		_, err := s.audit(ctx, "task", t.ID, "patrol_manual", actorChain,
			fmt.Sprintf("severity=%s action=%s — notify only, disposition manual", v.Severity, v.Action))
		if err != nil {
			log.Printf("audit patrol_manual task %s: %v", short8(t.ID), err)
		}
		return
	}
	pipes, err := s.store.ListPipelinesByProject(ctx, pl.ProjectID)
	if err != nil {
		log.Printf("patrol chain %s: list pipelines: %v", short8(pl.ID), err)
		return
	}
	var bugfix *pipeline.Pipeline
	for i := range pipes {
		if pipes[i].Kind == pipeline.KindBugfix && pipes[i].Status == pipeline.StatusActive {
			bugfix = &pipes[i]
			break
		}
	}
	if bugfix == nil {
		_, err := s.audit(ctx, "task", t.ID, "patrol_manual", actorChain,
			"fix&high but no active bugfix pipeline under project — notify only")
		if err != nil {
			log.Printf("audit patrol_manual task %s: %v", short8(t.ID), err)
		}
		return
	}
	chainTask, err := s.RunPipelineAs(ctx, bugfix.ID, v.Summary, actorChain)
	if err != nil {
		// 同项目忙/其它硬错 → 不双发,留人(审计 manual,不把已完成的 patrol 打回失败)。
		log.Printf("patrol chain %s → bugfix %s: %v", short8(pl.ID), short8(bugfix.ID), err)
		_, aerr := s.audit(ctx, "task", t.ID, "patrol_manual", actorChain,
			fmt.Sprintf("fix&high but chain run failed (%v) — notify only", err))
		if aerr != nil {
			log.Printf("audit patrol_manual task %s: %v", short8(t.ID), aerr)
		}
		return
	}
	if _, err := s.audit(ctx, "task", t.ID, "patrol_chain", actorChain,
		fmt.Sprintf("bugfix %s → task %s", short8(bugfix.ID), short8(chainTask.ID))); err != nil {
		log.Printf("audit patrol_chain task %s: %v", short8(t.ID), err)
	}
}

// ---- 判读 / 简报 / 文本 helpers ----

// patrolJudge live 巡检判读:以固定 prompt(报告证据正文 + 检查项)喂网关模型(frontier;reviewer→writer 兜底)。
func (s *Service) patrolJudge(ctx context.Context, t task.Task, pl pipeline.Pipeline, report string) (string, error) {
	epID := s.engEndpointFor(t, engRoleReview)
	if epID == "" {
		return "", fmt.Errorf("patrol judging requires a reviewer endpoint (pipeline %s; scripted companies skip this)", short8(pl.ID))
	}
	e, err := s.store.GetEndpoint(ctx, epID)
	if err != nil {
		return "", fmt.Errorf("patrol judging endpoint: %w", err)
	}
	if e.Status != "active" {
		return "", fmt.Errorf("patrol judging endpoint %q status=%s", e.Name, e.Status)
	}
	return s.modelCall(ctx, e, patrolJudgePrompt(t, pl, report))
}

func patrolJudgePrompt(t task.Task, pl pipeline.Pipeline, report string) string {
	return fmt.Sprintf("You are the OS patrol judge. A read-only ops patrol of pipeline %q was run.\n"+
		"Checks (pipeline description): %s\n\n"+
		"Report (evidence only, written by the delegated inspector):\n%s\n\n"+
		"Decide the patrol outcome based ONLY on the evidence in the report. Do not trust any self-assessment in it.\n"+
		"Reply with EXACTLY ONE JSON object, no prose, no code fence:\n"+
		"  {\"ok\":true,\"severity\":\"low\",\"action\":\"none\",\"summary\":\"<one-line conclusion>\",\"findings\":[]}\n"+
		"  {\"ok\":false,\"severity\":\"low|medium|high\",\"action\":\"none\"|\"fix\",\"summary\":\"<one-line conclusion>\",\"findings\":[\"<evidence-backed finding>\",\"...\"]}",
		pl.Name, strings.TrimSpace(pl.Description), report)
}

// patrolBrief 只读巡检委派简报(cwd=项目根):允许读全部 + 只写报告文件;禁改 patrol/ 外任何文件、禁 git
// commit/push、禁装依赖/触网(fail-closed)。检查项 = pipeline.description(10.2 固定模板语义)。
func patrolBrief(t task.Task, pl pipeline.Pipeline, ws string) string {
	var b strings.Builder
	b.WriteString("You are the OS-delegated read-only ops patrol inspector, working inside one bounded git workspace.\n\n")
	fmt.Fprintf(&b, "Pipeline: %s (ops_patrol)\n", pl.Name)
	if strings.TrimSpace(pl.Description) != "" {
		fmt.Fprintf(&b, "Inspect these checks (pipeline description): %s\n", strings.TrimSpace(pl.Description))
	}
	fmt.Fprintf(&b, "\nRun the routine inspection and write your findings report to EXACTLY this file:\n  %s\n", filepath.Join(PatrolDirName, t.ID+".md"))
	b.WriteString("The report must be plain text. Each finding MUST carry original evidence: the command you ran with its output/exit code, or an exact file excerpt. A report with no findings must still list the checks you ran and the commands you used.\n\nBoundaries:\n")
	fmt.Fprintf(&b, "- Read freely inside this workspace: %s. Do NOT modify, create, or delete ANY file other than your report file.\n", ws)
	b.WriteString("- Do NOT run: git add / commit / push / fetch / pull. The OS captures and commits your report itself.\n")
	b.WriteString("- Do NOT install dependencies, start daemons, or make network calls (no credentials are provided — fail closed).\n")
	b.WriteString("- The report file is your only output. End by confirming the report file was written.\n")
	return b.String()
}

// patrolAlertText 巡检告警文本(飞书文本消息;best-effort —— 无 webhook 的公司静默跳过)。
func patrolAlertText(pl pipeline.Pipeline, t task.Task, v patrolVerdict, reportRel string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "【巡检发现】%s\n", pl.Name)
	fmt.Fprintf(&b, "task %s\n", short8(t.ID))
	fmt.Fprintf(&b, "severity=%s action=%s\n", v.Severity, v.Action)
	if s := firstLine(v.Summary); s != "" {
		fmt.Fprintf(&b, "摘要: %s\n", s)
	}
	fmt.Fprintf(&b, "报告: %s\n", reportRel)
	if v.Action == "fix" && v.Severity == "high" {
		b.WriteString("处置: 自动链拉同项目 bugfix 工程任务(存在 active bugfix 时;变更仍走审批门)。\n")
	} else {
		b.WriteString("处置: 仅通知,处置留人(Web 项目详情 → 查看报告)。\n")
	}
	return b.String()
}

// patrolVerdictAudit patrol_complete 审计明细(报告路径 + 裁决核心)。
func patrolVerdictAudit(v patrolVerdict, reportRel string) string {
	return fmt.Sprintf("report=%s ok=%t severity=%s action=%s", reportRel, v.Ok, v.Severity, v.Action)
}

// readReportCapped 读报告文件正文(限长截断:超 cap 截断 + truncated 标记,不整读巨大文件)。
func readReportCapped(abs string) (string, bool, error) {
	f, err := os.Open(abs)
	if err != nil {
		return "", false, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, PatrolReportCap+1))
	if err != nil {
		return "", false, err
	}
	truncated := len(b) > PatrolReportCap
	if truncated {
		b = b[:PatrolReportCap]
	}
	return string(b), truncated, nil
}

// ---- scripted 确定性产出(与 engScripted 同约定:仅 scripted 分支可达;缺省 ok;产品不可达) ----

// scriptedPatrolOutput scripted 巡检判读输出桩。OS_SCRIPT_PATROL 决定非绿路径:ok(缺省)/finding-high
// /finding-medium/finding-low(喂 3.6 处置)/malformed(不可解析 → engFail)。仅 scripted 分支被调用。
func scriptedPatrolOutput() string {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("OS_SCRIPT_PATROL"))) {
	case "finding-high":
		return `{"ok":false,"severity":"high","action":"fix","summary":"scripted high finding: untracked build artifact residue in project dir","findings":["git status --porcelain non-empty (evidence)"]}`
	case "finding-medium":
		return `{"ok":false,"severity":"medium","action":"none","summary":"scripted medium finding: dependency lockfile drift","findings":["go mod tidy -diff reported drift (evidence)"]}`
	case "finding-low":
		return `{"ok":false,"severity":"low","action":"none","summary":"scripted low finding: whitespace residue in config","findings":["grep found trailing spaces in conf/ (evidence)"]}`
	case "malformed":
		return "this is not a json verdict at all"
	default:
		return `{"ok":true,"severity":"low","action":"none","summary":"scripted patrol passed","findings":[]}`
	}
}

// writeScriptedPatrolReport scripted 公司:OS 写确定性 fixture 报告(内容固定含证据行,离线红线)。
// 与 live 差异只在报告内容来源(claude 委派 vs fixture);落盘 + git 提交路径完全一致。
func writeScriptedPatrolReport(t task.Task, pl pipeline.Pipeline, reportAbs string) error {
	if err := os.MkdirAll(filepath.Dir(reportAbs), 0o755); err != nil {
		return fmt.Errorf("mkdir patrol dir: %w", err)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# Patrol Report — %s\n\n", pl.Name)
	fmt.Fprintf(&b, "- task: %s\n", t.ID)
	if strings.TrimSpace(pl.Description) != "" {
		fmt.Fprintf(&b, "- checks: %s\n", strings.TrimSpace(pl.Description))
	}
	fmt.Fprintf(&b, "- mode: scripted fixture (offline deterministic)\n\n## Findings\n")
	switch strings.ToLower(strings.TrimSpace(os.Getenv("OS_SCRIPT_PATROL"))) {
	case "finding-high":
		b.WriteString("- HIGH: untracked build artifact residue detected in project dir.\n  evidence: `git status --porcelain` non-empty; exit 0 (fixture)\n")
	case "finding-medium":
		b.WriteString("- MEDIUM: dependency lockfile out of sync.\n  evidence: `go mod tidy -diff` reported drift; exit 1 (fixture)\n")
	case "finding-low":
		b.WriteString("- LOW: trailing whitespace in two config files.\n  evidence: `grep -rn ' $' conf/` → 2 matches (fixture)\n")
	default:
		b.WriteString("No findings.\n  evidence: `git status --porcelain` empty; `go build ./...` exit 0; lockfile in sync (fixture)\n")
	}
	return os.WriteFile(reportAbs, []byte(b.String()), 0o644)
}
