package service

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/glacierzzz26/one-person-company-os/internal/endpoint"
	"github.com/glacierzzz26/one-person-company-os/internal/provider"
	"github.com/glacierzzz26/one-person-company-os/internal/task"
)

// Engineering Driver 的执行/判读分界(Phase 6.2 + Phase 8.2 修订 B + Phase 8.3 A)。
// 三阶段(写/测/审)各自一次执行/模型调用;判读(test/review)输出 = 结构化 JSON 信号,
// 解析在 judge.go(JSON 主契约 + 旧标记兜底)。
//
// 修订 B 后(2026-09-04):writer(需真动手的执行阶段)live = 委派集成 agent CLI(claude Code)
// 在任务 git workspace 自主干,OS 用 git 捕获真实 diff(见 delegate.go delegateWriter);
// 判读角色(planner/triage/test/review)live = 网关模型 text Chat(proto=openai,见下方 modelCall)。
//
// 模式切换(§12 #1 mock/real):OS_ENGINE_MODE
//   - live(默认,缺省偏 fail-closed):writer → 委派(需 git workspace);判读 → 网关 text Chat。
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
	hint          string // 上一判读失败原因(test 失败摘要 / review 驳回理由),喂下一 writer 简报(8.3 A3)
	prompt        string // 阶段指令 + 上下文(由 driver 组)
}

// engCall 执行一次工程阶段调用(live):writer → 委派集成 agent(git 捕获真实 diff);
// 其余判读角色 → 网关 text Chat。scripted → engScripted(确定性,writer 返回假 diff)。
func (s *Service) engCall(ctx context.Context, t task.Task, c engCallCtx) (string, error) {
	if strings.EqualFold(os.Getenv("OS_ENGINE_MODE"), "scripted") {
		return engScripted(c), nil
	}
	if c.role == engRoleWriter {
		out, err := s.delegateWriter(ctx, t, c)
		if err != nil {
			return "", fmt.Errorf("engineering task %s: writer: %w", short8(t.ID), err)
		}
		return out, nil
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

// modelCall 对给定端点执行一次网关判读(修订 B:OS 回合判读只走 text Chat)。
// 端点必须 proto=openai(自建网关);claude CLI legacy(ClaudeCLI)不再被 modelCall 消费,
// 其 Generate 路径仅 0-7 遗留消费方保留(provider 标 Deprecated,代码不动)。
// planner 拆解与 intake triage 与 6.2 工程判读共用此路径,统一切网关。token 解密不入日志。
func (s *Service) modelCall(ctx context.Context, e endpoint.Endpoint, prompt string) (string, error) {
	if !strings.EqualFold(e.Proto, "openai") {
		return "", fmt.Errorf("judging requires proto=openai gateway endpoint %q (proto=%s); add an OpenAI-compatible gateway endpoint", e.Name, e.Proto)
	}
	token, err := endpoint.OpenToken(e.TokenEnc)
	if err != nil {
		return "", err
	}
	c := provider.NewOpenAI(e.BaseURL, token, e.SelectedModel)
	resp, err := c.Chat(ctx, provider.ChatRequest{
		Messages: []provider.Message{provider.User(prompt)},
	})
	if err != nil {
		return "", err
	}
	if resp.FinishReason == "length" {
		return "", fmt.Errorf("judge output truncated (finish_reason=length)")
	}
	return resp.Content, nil
}

// engEndpointFor 解析某阶段使用的端点(8.4 起 test 分槽):test → test_endpoint_id → reviewer → writer;
// review → reviewer → writer。显式端点永远覆盖默认;空 → agent 默认(既有语义,scripted 不查)。
func (s *Service) engEndpointFor(t task.Task, role string) string {
	if role == engRoleTest && t.TestEndpointID != nil && *t.TestEndpointID != "" {
		return *t.TestEndpointID
	}
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
		// 8.3 A2:scripted test/review 输出切结构化 JSON(与 parseTest/parseReview 主契约同构)。
		switch os.Getenv("OS_SCRIPT_TEST") {
		case "fail-all":
			return `{"pass":false,"summary":"scripted permanent failure"}`
		case "fail-once":
			if c.retry == 0 {
				return `{"pass":false,"summary":"scripted flake (first attempt)"}`
			}
			return `{"pass":true,"summary":"scripted pass after flake"}`
		default:
			return `{"pass":true,"summary":"scripted pass"}`
		}
	case engRoleReview:
		// 熔断后人工 approve 续跑:无条件放行,让任务确定性收敛。
		if c.humanOverride {
			return `{"verdict":"approve","reason":"human approved after fuse"}`
		}
		v := strings.ToLower(strings.TrimSpace(os.Getenv("OS_SCRIPT_REVIEW")))
		switch {
		case v == "", v == "approve":
			return `{"verdict":"approve"}`
		case v == "reject":
			return `{"verdict":"needs_changes","reason":"scripted reviewer disagreement"}`
		default:
			// reject:N → conflict < N 时驳回,达到 N 后放行
			if n := scriptedRejectN(v); n >= 0 && c.conflict < n {
				return `{"verdict":"needs_changes","reason":"scripted reviewer disagreement"}`
			}
			return `{"verdict":"approve"}`
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

func short8(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}
