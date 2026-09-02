package service

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/glacierzzz26/one-person-company-os/internal/endpoint"
	"github.com/glacierzzz26/one-person-company-os/internal/provider"
	"github.com/glacierzzz26/one-person-company-os/internal/task"
)

// Engineering Driver 的模型调用分界(Phase 6.2)。
// 三阶段(写/测/审)各自一次模型调用,输出走容错结构化解析(见下方 parse* 助手)。
// 真实代码落盘/测试执行属于 6.3(repos);此处模型产出 diff 文本与 verdict,为 6.3 的
// 落地动作留出衔接面。
//
// 模式切换(§12 #1 mock/real):OS_ENGINE_MODE
//   - live(默认,缺省偏 fail-closed):端点行 → 解密 token → claude -p。任务未挂端点 → 清晰报错。
//   - scripted:确定性离线冒烟(test double);writer/test/review 结果由 env 控制:
//        OS_SCRIPT_TEST   pass(默认)| fail-once(每轮首次失败后过) | fail-all
//        OS_SCRIPT_REVIEW approve(默认)| reject(持续驳回直到熔断;humanOverride 后放行)

const (
	engRoleWriter = "writer"
	engRoleTest   = "test"
	engRoleReview = "review"
)

// engCallCtx 是一次工程阶段模型调用的上下文。retry 为本轮内该阶段的第几次尝试(0 起)。
type engCallCtx struct {
	role          string // engRoleWriter | engRoleTest | engRoleReview
	round         int64
	conflict      int64
	humanOverride bool   // 熔断后人工 approve 续跑(提示 reviewer 重新决断)
	retry         int    // 本轮内重试次数(test 免费返工)
	prompt        string // 阶段指令 + 上下文(由 driver 组)
}

// engCall 执行一次工程阶段模型调用,返回模型原始输出。
func (s *Service) engCall(ctx context.Context, t task.Task, c engCallCtx) (string, error) {
	if strings.EqualFold(os.Getenv("OS_ENGINE_MODE"), "scripted") {
		return engScripted(c), nil
	}
	epID := s.engEndpointFor(t, c.role)
	if epID == "" {
		return "", fmt.Errorf("engineering task %s: no %s endpoint set (use --writer-endpoint/--reviewer-endpoint, or OS_ENGINE_MODE=scripted for offline smoke)", short8(t.ID), c.role)
	}
	e, err := s.store.GetEndpoint(ctx, epID)
	if err != nil {
		return "", fmt.Errorf("engineering task %s: %s endpoint: %w", short8(t.ID), c.role, err)
	}
	if e.Status != "active" {
		return "", fmt.Errorf("engineering task %s: endpoint %q status=%s", short8(t.ID), e.Name, e.Status)
	}
	out, err := s.modelCall(ctx, e, c.prompt)
	if err != nil {
		return "", fmt.Errorf("engineering task %s: %s: %w", short8(t.ID), c.role, err)
	}
	return out, nil
}

// modelCall 对给定端点执行一次真实模型提问(claude -p)。6.2 工程阶段与 6.3
// intake triage 共用同一真实调用路径。token 解密不入日志。
func (s *Service) modelCall(ctx context.Context, e endpoint.Endpoint, prompt string) (string, error) {
	token, err := endpoint.OpenToken(e.TokenEnc)
	if err != nil {
		return "", err
	}
	p := provider.NewClaudeEndpoint(e.Name, e.SelectedModel, e.BaseURL, token)
	resp, err := p.Generate(ctx, provider.Request{Prompt: prompt})
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

// engEndpointFor 解析某阶段使用的端点:id 为空 → agent 默认(Phase 6 先单端点:
// test/review 回退 writer 端点)。
func (s *Service) engEndpointFor(t task.Task, role string) string {
	switch role {
	case engRoleTest, engRoleReview:
		if t.ReviewerEndpointID != nil && *t.ReviewerEndpointID != "" {
			return *t.ReviewerEndpointID
		}
	}
	if t.WriterEndpointID != nil {
		return *t.WriterEndpointID
	}
	return ""
}

// engScripted 是离线确定性 test double:按阶段/env 返回可解析的固定内容。
func engScripted(c engCallCtx) string {
	switch c.role {
	case engRoleWriter:
		return fmt.Sprintf("PATCH\n```diff\n@@ round=%d conflict=%d\n- before\n+ fix (writer deterministic, retry=%d)\n```\n",
			c.round, c.conflict, c.retry)
	case engRoleTest:
		switch os.Getenv("OS_SCRIPT_TEST") {
		case "fail-all":
			return "TEST FAIL: scripted permanent failure"
		case "fail-once":
			if c.retry == 0 {
				return "TEST FAIL: scripted flake (first attempt)"
			}
			return "TEST OK"
		default:
			return "TEST OK"
		}
	case engRoleReview:
		// 熔断后人工 approve 续跑:无条件放行,让任务确定性收敛。
		if c.humanOverride {
			return "VERDICT: approve (human approved after fuse)"
		}
		v := strings.ToLower(strings.TrimSpace(os.Getenv("OS_SCRIPT_REVIEW")))
		switch {
		case v == "", v == "approve":
			return "VERDICT: approve"
		case v == "reject":
			return "VERDICT: needs_changes: scripted reviewer disagreement"
		default:
			// reject:N → conflict < N 时驳回,达到 N 后放行
			if n := scriptedRejectN(v); n >= 0 && c.conflict < n {
				return "VERDICT: needs_changes: scripted reviewer disagreement"
			}
			return "VERDICT: approve"
		}
	}
	return ""
}

// scriptedRejectN 解析 "reject:N";非该形态返回 -1。
func scriptedRejectN(v string) int64 {
	const p = "reject:"
	if !strings.HasPrefix(v, p) {
		return -1
	}
	var n int64
	if _, err := fmt.Sscanf(v[len(p):], "%d", &n); err != nil {
		return -1
	}
	return n
}

// ---- 容错结构化解析 ----

var (
	reviewVerdictRe = regexp.MustCompile(`(?i)VERDICT\s*[:=]\s*(approve|needs_changes|changes|reject|approved|rejected)\b`)
	testOkRe        = regexp.MustCompile(`(?i)\bTEST\s+(OK|PASS(ED)?)\b`)
	testFailRe      = regexp.MustCompile(`(?i)\bTEST\s+FAIL\b|(?i)\b(FAIL|ERROR|FAILED)\b`)
)

// parseReviewVerdict 从输出提取 approve / needs_changes;找不到 → ""。
func parseReviewVerdict(out string) string {
	m := reviewVerdictRe.FindStringSubmatch(out)
	if m == nil {
		return ""
	}
	switch strings.ToLower(m[1]) {
	case "approve", "approved":
		return "approve"
	case "needs_changes", "changes", "reject", "rejected":
		return "needs_changes"
	}
	return ""
}

// parseTestPass 判定测试是否通过:显式 TEST OK/PASS 或「无失败信号」→ 过;
// 显式 TEST FAIL / FAIL / ERROR → 不过。
func parseTestPass(out string) bool {
	if testOkRe.MatchString(out) {
		return true
	}
	if testFailRe.MatchString(out) {
		return false
	}
	return true
}

// extractDiff 从 writer 输出提取 diff:优先取 ``` 围栏内正文;无围栏 → 整段原文。
func extractDiff(out string) string {
	if i := strings.Index(out, "```"); i >= 0 {
		rest := out[i+3:]
		rest = strings.TrimPrefix(rest, "diff\n")
		rest = strings.TrimPrefix(rest, "diff\r\n")
		if j := strings.Index(rest, "```"); j >= 0 {
			return strings.TrimSpace(rest[:j])
		}
		return strings.TrimSpace(rest)
	}
	return strings.TrimSpace(out)
}

func short8(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}
