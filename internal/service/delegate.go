package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/glacierzzz26/one-person-company-os/internal/endpoint"
	"github.com/glacierzzz26/one-person-company-os/internal/task"
)

// 执行委派闭环(Phase 8.2 修订 B + Phase 8.3 B/C)。
// writer 阶段(live)由 OS 委派**集成 agent CLI**(agent CLI 工具族,默认 claude Code)在任务
// git workspace 自主读/改/跑;委派结束 OS 用 git 捕获真实 diff 作阶段产出,逐次委派留审计,
// 并 **OS 逐次委派自动 commit**(8.3 C1:baseline ref 钉任务起点 + commit 让工作树每委派后归 clean,
// 根治 split ≤8 子任务共享同一 repo workspace 的自锁)。
// 族选型:OS_AGENT_CLI(claude 默认;codex 注册槽位)。判读(planner/test/review/intake triage)
// 仍走网关模型文本(modelCall,engine.go)/结构化 JSON(judge.go)。
// scripted 下 writer/test/review 全走 engScripted,与 6.2 一致(离线可复现红线)。

const (
	// delegateMax = 单次委派墙钟上限(claudeDelegator 内部 WithTimeout;仍受任务级 runContext 预算约束)。
	delegateMax = 20 * time.Minute
)

// agent CLI 工具族(修订 B「claude 首位 → codex 等」;8.3 B 注册 + 选型)。
const (
	agentCLIClaude = "claude" // claude Code(agent 模式;真实现,claudeDelegator)
	agentCLICodex  = "codex"  // codex CLI:注册槽位 + LookPath 可用性门;真实适配留 live 验收
)

// agentCLIRegistry 族名 → Delegator 适配器。
var agentCLIRegistry = map[string]Delegator{
	agentCLIClaude: &claudeDelegator{},
	agentCLICodex:  &codexDelegator{},
}

// knownAgentCLIs 列已知族(报错/提示用,确定性顺序)。
const knownAgentCLIs = "claude, codex"

// agentCLIFromEnv 解析委派工具族:OS_AGENT_CLI,缺省 claude。合法性留 delegatorFor 判。
func agentCLIFromEnv() string {
	if v := strings.TrimSpace(os.Getenv("OS_AGENT_CLI")); v != "" {
		return strings.ToLower(v)
	}
	return agentCLIClaude
}

// --- Delegator 接口 ---

// Delegator 把有界工程任务委派给集成 agent CLI 在任务 workspace 自主执行(修订 B)。
// 8.3 B:族注册/选型(agentCLIRegistry),claude = 真实现,codex = 注册槽位。
type Delegator interface {
	// Delegate 运行一次委派。实现须把 cwd 钉在 spec.Workspace。
	// 真实路径下 Diff 由调用方 captureNet 统一捕获(相对任务 baseline ref,OS 不信任 agent 自述);
	// Diff 字段保留给未来自行捕获的实现,空值即「未提供」。
	Delegate(ctx context.Context, spec DelegateSpec) (DelegateResult, error)
}

// DelegateSpec 一次有界委派的输入。
type DelegateSpec struct {
	Workspace string        // 任务 git workspace(委派 cwd,唯一可写面)
	Brief     string        // 委派简报(任务 + 约束,见 delegateBrief)
	ModelEnv  []string      // 可选 cheap 配对:ANTHROPIC_BASE_URL/ANTHROPIC_AUTH_TOKEN/ANTHROPIC_MODEL
	Timeout   time.Duration // 单次委派墙钟上限(≤ 任务剩余预算)
	Family    string        // agent CLI 工具族(claude/codex,8.3 B;落审计供人复核)
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

// codexDelegator 注册槽位(8.3 B):codex CLI 真实适配(flags/授权)留 live 验收。
// 已装 codex 也返回明确 not-implemented,不假装能跑(防误以为 codex 已真执行)。
type codexDelegator struct{}

func (d *codexDelegator) Delegate(ctx context.Context, spec DelegateSpec) (DelegateResult, error) {
	if _, err := exec.LookPath("codex"); err != nil {
		return DelegateResult{}, fmt.Errorf("agent CLI %q selected but codex binary not installed (OS_AGENT_CLI=%q); install codex or use %q",
			agentCLICodex, agentCLICodex, agentCLIClaude)
	}
	return DelegateResult{}, fmt.Errorf("agent CLI %q is registered (8.3 slot) but not implemented for live delegation yet; authoring + flags verification pending live acceptance (use OS_AGENT_CLI=claude)", agentCLICodex)
}

// delegatorFor 按族解析本次委派工具:
//   - claude(默认):走注入缝 s.delegator(默认 claudeDelegator,测试注入 fake 记录 spec/落盘);
//   - 其他族:走 agentCLIRegistry 注册槽位(codex = LookPath 门 + not-implemented);
//   - 未知族:报错列已知族。
func (s *Service) delegatorFor(family string) (Delegator, error) {
	if family == "" {
		family = agentCLIClaude
	}
	if family == agentCLIClaude {
		if s.delegator != nil {
			return s.delegator, nil
		}
		return &claudeDelegator{}, nil
	}
	d, ok := agentCLIRegistry[family]
	if !ok {
		return nil, fmt.Errorf("unknown agent CLI %q (known: %s)", family, knownAgentCLIs)
	}
	return d, nil
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

// --- C1 git 助手(baseline ref + 逐委派 commit,8.3;替换 8.2 captureChanges「相对 HEAD 不 commit」语义) ---

// wsBaselineRef 任务起点 ref 名:refs/os/tasks/<taskID>。轻量引用,首委派钉 HEAD,永不前移;
// 判读/完成结果消费的净 diff 以它为锚(与 8.2 相对不动的 HEAD 的累积 diff 内容等价)。
func wsBaselineRef(taskID string) string {
	return "refs/os/tasks/" + taskID
}

// excludeWorkspaceTools 把委派工具残留目录(.claude/、.codex/)追加进 .git/info/exclude。
// 幂等、仅本地(不进 commit)、不入 add -A → 残留不脏 clean 门、不进 diff(C4)。
func excludeWorkspaceTools(ctx context.Context, ws string) error {
	p := filepath.Join(ws, ".git", "info", "exclude")
	data, err := os.ReadFile(p)
	if err != nil {
		return fmt.Errorf("read .git/info/exclude: %w", err)
	}
	need := []string{}
	for _, pat := range []string{".claude/", ".codex/"} {
		if !strings.Contains(string(data), pat) {
			need = append(need, pat)
		}
	}
	if len(need) == 0 {
		return nil
	}
	var b strings.Builder
	b.Write(data)
	if len(data) > 0 && data[len(data)-1] != '\n' {
		b.WriteByte('\n')
	}
	b.WriteString("\n# one-person-company-os: agent CLI tool residue (Phase 8.3 C1)\n")
	for _, pat := range need {
		b.WriteString(pat)
		b.WriteByte('\n')
	}
	return os.WriteFile(p, []byte(b.String()), 0o644)
}

// ensureBaseline 首委派钉任务起点:ref 不存在 → 排除工具残留目录 + update-ref 钉 HEAD;
// ref 已存在(续跑/返工/commit 前被 kill 后重认领)→ no-op,不重钉。
func ensureBaseline(ctx context.Context, ws, taskID string) error {
	ref := wsBaselineRef(taskID)
	if _, err := gitDirCmd(ctx, ws, "rev-parse", "--verify", "--quiet", ref); err == nil {
		return nil // 已钉
	}
	if err := excludeWorkspaceTools(ctx, ws); err != nil {
		return err
	}
	if _, err := gitDirCmd(ctx, ws, "update-ref", ref, "HEAD"); err != nil {
		return fmt.Errorf("pin baseline ref %s: %w", ref, err)
	}
	return nil
}

// captureNet 捕获任务自起点(ref)以来的净 diff:add -A(含中间 OS commit + 未提交改动)后 diff --cached <ref>。
// 与 8.2 的累积脏 diff 语义等价(§二 委派收口语义前提);空 = 相对任务起点无任何改动。
func captureNet(ctx context.Context, ws, ref string) (string, error) {
	if _, err := gitDirCmd(ctx, ws, "add", "-A"); err != nil {
		return "", err
	}
	return gitDirCmd(ctx, ws, "diff", "--cached", ref)
}

// commitDelegation OS 提交本次委派(唯一提交者;agent 简报已禁自行 commit)。
// 有已暂存改动 → commit(当前分支直接提交,仓库身份;无身份 → 清晰报错给 git config 指引);
// 无暂存(agent 只回退到基线 / 上一委派已收口)→ skip,工作树保持 clean。
func commitDelegation(ctx context.Context, ws, msg string) error {
	if _, err := gitDirCmd(ctx, ws, "diff", "--cached", "--quiet"); err == nil {
		return nil // 无已暂存改动 → 无新提交
	}
	if _, err := gitDirCmd(ctx, ws, "commit", "-m", msg); err != nil {
		return fmt.Errorf("OS commit of delegation failed (set repo identity: git config user.name / user.email): %w", err)
	}
	return nil
}

// delegateCommitMsg 逐次委派提交的确定性 message(带 task8 + round/retry/family,历史可归属)。
func delegateCommitMsg(t task.Task, c engCallCtx, family string) string {
	return fmt.Sprintf("os-delegate: %s round=%d retry=%d family=%s", short8(t.ID), c.round, c.retry, family)
}

// --- delegateWriter:engCall 的 live writer 分支 ---

// delegateWriter 执行一次 writer 委派(live),返回任务自起点(ref)以来的净 diff 作阶段产出。
// 流程(8.3 C1):git 前置 → ensureBaseline(钉任务起点)→ 选型委派(family=agentCLIFromEnv)→
// captureNet(净 diff,空 → 既有「no workspace changes」报错)→ audit eng_delegate(带 family+ref,
// 先行记录保证 commit 失败后重认领仍可放行)→ commitDelegation。每次委派成功后工作树归 clean。
// 与旧 writer 文本路径同语义:输出继续被 test/review 判读消费(净 diff = 8.2 累积 diff 等价)。
func (s *Service) delegateWriter(ctx context.Context, t task.Task, c engCallCtx) (string, error) {
	ws := t.WorkspacePath
	if ws == "" || !wsIsGit(ctx, ws) {
		return "", fmt.Errorf("writer delegation requires a git workspace (task workspace=%q is not a git repo; bind an os repo checkout)", ws)
	}
	if err := ensureBaseline(ctx, ws, t.ID); err != nil {
		return "", fmt.Errorf("ensure baseline: %w", err)
	}
	ref := wsBaselineRef(t.ID)

	family := agentCLIFromEnv()
	d, err := s.delegatorFor(family)
	if err != nil {
		return "", err
	}
	spec := DelegateSpec{
		Workspace: ws,
		Brief:     delegateBrief(t, c, ws),
		ModelEnv:  s.delegateEnv(ctx, t),
		Timeout:   delegateTimeout(ctx, t, c),
		Family:    family,
	}
	res, err := d.Delegate(ctx, spec)
	if err != nil {
		return "", fmt.Errorf("delegate (round=%d retry=%d family=%s): %w", c.round, c.retry, family, err)
	}

	diff, err := captureNet(ctx, ws, ref)
	if err != nil {
		return "", fmt.Errorf("capture diff: %w", err)
	}
	if diff == "" {
		return "", fmt.Errorf("delegation produced no workspace changes (round=%d retry=%d)", c.round, c.retry)
	}
	// 审计先行(commit 失败也留痕:重认领走「脏 + 本任务 eng_delegate」放行续收口,不丢产出)。
	if _, err := s.audit(ctx, "task", t.ID, "eng_delegate", taskActor(t),
		delegateAuditDetail(t, c, spec, res, ref, diff)); err != nil {
		return "", err
	}
	if err := commitDelegation(ctx, ws, delegateCommitMsg(t, c, family)); err != nil {
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

// delegateAuditDetail 逐次委派审计明细(族 + 起点 ref + 约束 + 自述 + 净 diff 规模,供人复核)。
// 8.3 B/C:detail 带 family + baseline ref —— 复核者看到「谁在被委派」「相对任务起点改了多少」。
func delegateAuditDetail(t task.Task, c engCallCtx, spec DelegateSpec, res DelegateResult, ref, diff string) string {
	report := strings.TrimSpace(res.Report)
	if i := strings.IndexByte(report, '\n'); i >= 0 {
		report = report[:i] // 审计 detail 只留首行自述
	}
	if report == "" {
		report = "(no agent self-report)"
	}
	family := spec.Family
	if family == "" {
		family = agentCLIClaude
	}
	return fmt.Sprintf("family=%s ref=%s round=%d retry=%d ws=%s diff=%dB report=%s",
		family, ref, c.round, c.retry, spec.Workspace, len(diff), report)
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
	// 8.3 A3:回喂上一判读失败/驳回原因(test 失败摘要 / review needs_changes reason)。
	if strings.TrimSpace(c.hint) != "" {
		fmt.Fprintf(&b, "\nPrevious judging feedback: %s", strings.TrimSpace(c.hint))
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
