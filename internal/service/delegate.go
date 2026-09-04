package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/glacierzzz26/one-person-company-os/internal/endpoint"
	"github.com/glacierzzz26/one-person-company-os/internal/task"
)

// 执行委派闭环(Phase 8.2,修订 B:执行 = 集成 agent CLI 委派)。
// writer 阶段(live)不再文本产出,由 OS 委派 claude Code(claude 二进制 agent 模式)在任务
// git workspace 自主读/改/跑;委派结束 OS 用 git 捕获真实 diff 作该 writer 阶段产出,逐次委派留审计。
// 判读(planner/test/review/intake triage)仍走网关模型文本(modelCall,见 engine.go)。
// 这是加接线不改语义:scripted 下 writer/test/review 全走 engScripted,与 6.2 一致。

const (
	// delegateMax = 单次委派墙钟上限(claudeDelegator 内部 WithTimeout;仍受任务级 runContext 预算约束)。
	delegateMax = 20 * time.Minute
)

// --- Delegator 接口 ---

// Delegator 把有界工程任务委派给集成 agent CLI 在任务 workspace 自主执行(修订 B 8.2)。
// 8.2 实现 = claudeDelegator(claude 二进制 agent 模式);codex 等工具族选型属 8.3。
type Delegator interface {
	// Delegate 运行一次委派。实现须把 cwd 钉在 spec.Workspace。
	// 真实路径下 Diff 由调用方 captureChanges 统一捕获(OS 不信任 agent 自述);Diff 字段保留给
	// 未来自行捕获的实现,空值即「未提供」。
	Delegate(ctx context.Context, spec DelegateSpec) (DelegateResult, error)
}

// DelegateSpec 一次有界委派的输入。
type DelegateSpec struct {
	Workspace string        // 任务 git workspace(委派 cwd,唯一可写面)
	Brief     string        // 委派简报(任务 + 约束,见 delegateBrief)
	ModelEnv  []string      // 可选 cheap 配对:ANTHROPIC_BASE_URL/ANTHROPIC_AUTH_TOKEN/ANTHROPIC_MODEL
	Timeout   time.Duration // 单次委派墙钟上限(≤ 任务剩余预算)
}

// DelegateResult 委派产出。Report = agent 自述(改了哪些文件 / 自测命令与结果),落审计与 execution 供人复核。
type DelegateResult struct {
	Diff   string // workspace 相对 HEAD 的真实改动(git diff 文本);真实路径由调用方捕获,此处可为空
	Report string // agent 自述
}

// claudeDelegator 真实委派:claude 二进制 agent 模式,cwd 钉在 spec.Workspace。
// flags(按 `claude --help` 核实的 8.2 固定契约;本机授权/版本差异归 live 验收):
//
//	claude -p <brief> --output-format json --permission-mode bypassPermissions --permission-prompts none
//
// bypassPermissions 的信任面 = OS 自有 git workspace(创建方是 OS/仓库登记,非用户随意目录);
// 治理仍由 round 审批/熔断/审计兜底。输出 JSON,取 result 文本作 Report。
type claudeDelegator struct{}

type claudeStdout struct {
	Result string `json:"result"`
}

func (d *claudeDelegator) Delegate(ctx context.Context, spec DelegateSpec) (DelegateResult, error) {
	timeout := spec.Timeout
	if timeout <= 0 {
		timeout = delegateMax
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "claude", "-p", spec.Brief,
		"--output-format", "json",
		"--permission-mode", "bypassPermissions",
		"--permission-prompts", "none")
	cmd.Dir = spec.Workspace
	cmd.Env = append(os.Environ(), spec.ModelEnv...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return DelegateResult{}, fmt.Errorf("claude delegate: %w: %s", err, truncate(stderr.String(), 400))
	}
	var parsed claudeStdout
	if err := json.Unmarshal(stdout.Bytes(), &parsed); err != nil {
		// 非 JSON(异常退出/残留文本):原样 stdout 作报告兜底,产出仍以 git 捕获为准。
		return DelegateResult{Report: truncate(stdout.String(), 2000)}, nil
	}
	return DelegateResult{Report: strings.TrimSpace(parsed.Result)}, nil
}

// --- git 助手(service 内固定命令,不经用户串;镜像 tool/git.go 本地白名单) ---

// gitDirCmd 在 workspace 内执行一条固定 git 命令,stdout+stderr 合并返回;失败带命令回显。
func gitDirCmd(ctx context.Context, ws string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", ws}, args...)...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, truncate(out.String(), 200))
	}
	return out.String(), nil
}

// wsIsGit workspace 是否为 git 仓库。
func wsIsGit(ctx context.Context, ws string) bool {
	if strings.TrimSpace(ws) == "" {
		return false
	}
	_, err := gitDirCmd(ctx, ws, "rev-parse", "--is-inside-work-tree")
	return err == nil
}

// wsPorcelainClean 工作树/index 相对 HEAD 是否无改动(含未跟踪)。
func wsPorcelainClean(ctx context.Context, ws string) bool {
	out, err := gitDirCmd(ctx, ws, "status", "--porcelain")
	return err == nil && strings.TrimSpace(out) == ""
}

// captureChanges 捕获工作区相对 HEAD 的真实改动(含新增/删除/改名)。add -A 顺带把改动暂存留痕。
func captureChanges(ctx context.Context, ws string) (string, error) {
	if _, err := gitDirCmd(ctx, ws, "add", "-A"); err != nil {
		return "", err
	}
	return gitDirCmd(ctx, ws, "diff", "--cached")
}

// --- delegateWriter:engCall 的 live writer 分支 ---

// delegateWriter 执行一次 writer 委派(live),返回 workspace 相对 HEAD 的真实 diff 作阶段产出。
// 与旧 writer 文本路径同语义:输出继续被 test/review 判读消费(累积 diff)。
func (s *Service) delegateWriter(ctx context.Context, t task.Task, c engCallCtx) (string, error) {
	ws := t.WorkspacePath
	if ws == "" || !wsIsGit(ctx, ws) {
		return "", fmt.Errorf("writer delegation requires a git workspace (task workspace=%q is not a git repo; bind an os repo checkout)", ws)
	}
	spec := DelegateSpec{
		Workspace: ws,
		Brief:     delegateBrief(t, c, ws),
		ModelEnv:  s.delegateEnv(ctx, t),
		Timeout:   delegateTimeout(ctx, t, c),
	}
	res, err := s.delegator.Delegate(ctx, spec)
	if err != nil {
		return "", fmt.Errorf("delegate (round=%d retry=%d): %w", c.round, c.retry, err)
	}
	diff, err := captureChanges(ctx, ws)
	if err != nil {
		return "", fmt.Errorf("capture diff: %w", err)
	}
	if diff == "" {
		return "", fmt.Errorf("delegation produced no workspace changes (round=%d retry=%d)", c.round, c.retry)
	}
	if _, err := s.audit(ctx, "task", t.ID, "eng_delegate", taskActor(t), delegateAuditDetail(t, c, spec, res, diff)); err != nil {
		return "", err
	}
	return diff, nil
}

// delegateBaseline 在进入 round 循环的首个 writer 委派前(live、非熔断续跑)校验认领起点:
//   - workspace 非 git → 硬错误(需给工程任务绑定 os repo checkout);
//   - workspace 脏 且本任务从未委派过(无 eng_delegate 审计)→ 硬错误:认领起点须 clean,
//     不吞并先前存在的无关改动(commit/stash 或换干净 checkout);
//   - workspace 脏 但已有本任务的 eng_delegate 审计 → 放行:残留是任务自己先前 attempt/返工的
//     产物,在同一工作树累积迭代(修订 B 非破坏语义),熔断续跑/requeue 不被自己的残留卡死。
//
// scripted 跳过(走 engScripted,不落委派)。
func (s *Service) delegateBaseline(ctx context.Context, t task.Task) error {
	if strings.EqualFold(os.Getenv("OS_ENGINE_MODE"), "scripted") {
		return nil
	}
	ws := t.WorkspacePath
	if !wsIsGit(ctx, ws) {
		return fmt.Errorf("writer delegation requires a git workspace (task workspace=%q is not a git repo; bind an os repo checkout)", ws)
	}
	if wsPorcelainClean(ctx, ws) {
		return nil
	}
	prior, err := s.store.HasAuditAction(ctx, "task", t.ID, "eng_delegate")
	if err != nil {
		return err
	}
	if !prior {
		return fmt.Errorf("task workspace %q is dirty at claim and this task has not delegated before; refusing to sweep unrelated pre-existing changes (commit or stash them, or bind a clean checkout)", ws)
	}
	return nil
}

// delegateEnv writer_endpoint_id(可选)→ claude 委派 env(cheap 配对)。没设 → nil(claude 自带鉴权/模型)。
// 方言自负:claude CLI 走 Anthropic /v1/messages;OpenAI 方言网关不能作 claude 后端(见 8.2 契约 §六)。
func (s *Service) delegateEnv(ctx context.Context, t task.Task) []string {
	if t.WriterEndpointID == nil || *t.WriterEndpointID == "" {
		return nil
	}
	e, err := s.store.GetEndpoint(ctx, *t.WriterEndpointID)
	if err != nil || e.Status != "active" {
		return nil
	}
	tok, err := endpoint.OpenToken(e.TokenEnc)
	if err != nil || tok == "" {
		return nil
	}
	out := make([]string, 0, 3)
	if e.BaseURL != "" {
		out = append(out, "ANTHROPIC_BASE_URL="+e.BaseURL)
	}
	out = append(out, "ANTHROPIC_AUTH_TOKEN="+tok)
	if e.SelectedModel != "" {
		out = append(out, "ANTHROPIC_MODEL="+e.SelectedModel)
	}
	return out
}

// delegateTimeout 单次委派墙钟上限:min(任务剩余预算, delegateMax);无任务预算 → delegateMax。
func delegateTimeout(ctx context.Context, _ task.Task, _ engCallCtx) time.Duration {
	if dl, ok := ctx.Deadline(); ok {
		if rem := time.Until(dl); rem < delegateMax {
			if rem < 0 {
				return time.Second // 已过期:让 CommandContext 尽快失败
			}
			return rem
		}
	}
	return delegateMax
}

// delegateAuditDetail 逐次委派审计明细(约束 + 自述 + 产出规模,供人复核)。
func delegateAuditDetail(t task.Task, c engCallCtx, spec DelegateSpec, res DelegateResult, diff string) string {
	report := strings.TrimSpace(res.Report)
	if i := strings.IndexByte(report, '\n'); i >= 0 {
		report = report[:i] // 审计 detail 只留首行自述
	}
	if report == "" {
		report = "(no agent self-report)"
	}
	return fmt.Sprintf("claude round=%d retry=%d ws=%s diff=%dB report=%s",
		c.round, c.retry, spec.Workspace, len(diff), report)
}

// delegateBrief 一段有界任务简报(专为真干 agent 写;旧「只输出 diff 围栏」提示不再适用 writer)。
func delegateBrief(t task.Task, c engCallCtx, ws string) string {
	var b strings.Builder
	b.WriteString("You are the OS-delegated coding engineer, working inside one bounded git workspace on a task.\n\n")
	fmt.Fprintf(&b, "Task: %s\n", t.Title)
	if strings.TrimSpace(t.Description) != "" {
		fmt.Fprintf(&b, "Description: %s\n", strings.TrimSpace(t.Description))
	}
	fmt.Fprintf(&b, "Context: round=%d reviewer_conflicts=%d", c.round, c.conflict)
	if c.retry > 0 {
		fmt.Fprintf(&b, " — this is rewrite attempt #%d in the current round (earlier attempt did not pass test judging)", c.retry+1)
	}
	if c.humanOverride {
		b.WriteString(" — a human approved after the fuse; resolve the previously reported concerns")
	}
	b.WriteString("\n\nBoundaries:\n")
	fmt.Fprintf(&b, "- Work only inside this workspace: %s. Only touch files directly related to the task.\n", ws)
	b.WriteString("- Do NOT run: git commit / push / fetch / pull. The OS captures your changes itself.\n")
	b.WriteString("- After editing, run the relevant tests/build yourself so the change is self-consistent.\n")
	b.WriteString("- Do NOT modify governance files: permission policies, approvals, audit records, or the config that gates them.\n")
	b.WriteString("\nEnd report must list: the files you changed; the commands you ran and their outcomes.\n")
	return b.String()
}

// truncate(过长文本截断)在 endpoint.go 已有同签名实现;错误/审计简报共用。
