package provider

import (
	"context"
	"fmt"
)

// ChatStub 是离线确定性 Chatter(Phase 8.1 scripted 保真;8.2 tool 回合的假后端)。
// 服务层 scripted(engScripted / engScriptedPlan)是 0-7 文本语义、不动;此档补"工具语义下可复现":
// 按构造时编排的应答序列逐次返回(可先吐 ToolCalls 让调用方走"执行→回填→再 Chat"闭环,再收文本)。
// 确定性:同一次调用序号同输入 → 同输出;序列耗尽后固定重复最后一个应答。
// 非并发安全(仅离线冒烟/单协程驱动用)。
type ChatStub struct {
	model    string
	script   []ChatResponse // 逐次弹出;耗尽后重复最后一条
	err      error          // 恒定错误(故障注入)
	fallback ChatResponse   // script 为空时的兜底应答
	calls    int
}

// NewChatStubText 构造恒定返回给定文本(stop)的桩。
func NewChatStubText(model, text string) *ChatStub {
	return &ChatStub{model: model, fallback: ChatResponse{Content: text, FinishReason: "stop"}}
}

// NewChatStubScript 构造按序应答的桩:第 i 次 Chat 返回 script[i](越界重复最后一条)。
func NewChatStubScript(model string, script ...ChatResponse) *ChatStub {
	s := &ChatStub{model: model, fallback: ChatResponse{FinishReason: "stop"}}
	if len(script) > 0 {
		s.script = script
	}
	return s
}

// NewChatStubError 构造恒定返回错误的桩(故障注入,离线验证 fail-closed 路径)。
func NewChatStubError(model string, err error) *ChatStub {
	return &ChatStub{model: model, err: err}
}

func (s *ChatStub) Model() string { return s.model }

func (s *ChatStub) Chat(_ context.Context, _ ChatRequest) (ChatResponse, error) {
	if s.err != nil {
		return ChatResponse{}, fmt.Errorf("chat stub: %w", s.err)
	}
	if len(s.script) == 0 {
		return s.fallback, nil
	}
	idx := s.calls
	if idx >= len(s.script) {
		idx = len(s.script) - 1
	}
	s.calls++
	return s.script[idx], nil
}
