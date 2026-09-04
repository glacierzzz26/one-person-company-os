package provider

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Phase 8.1 messages+tools 契约(OpenAI Chat Completions 方言)。
// 契约直接以自建网关协议为唯一方言 —— 网关即厂商,OS 不再按厂商适配
// (方向修订 A,见 docs/phase8/design/provider-upgrade.md §三)。
// json tag 即网关线格式,字段与 Chat Completions wire 对齐。

// Message 是一条会话消息:
//
//	role=system|user    → Content 文本
//	role=assistant      → Content 文本(可空);若本轮调用工具则带 ToolCalls
//	role=tool           → ToolCallID 引用某次调用 + Content 输出(工具结果回填)
type Message struct {
	Role       string     `json:"role"`                   // system | user | assistant | tool
	Content    string     `json:"content,omitempty"`      // 文本;assistant 纯工具轮可空
	ToolCallID string     `json:"tool_call_id,omitempty"` // role=tool 回填引用
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`   // role=assistant
}

// ToolCall 是一次函数调用请求(assistant 发起)。Type 恒 "function"(8.1 只支持 function)。
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

// FunctionCall 是函数调用描述;Arguments 为参数 JSON 的序列化字符串(网关协议如此)。
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// Tool 是注入模型的能力描述;Function.Parameters = JSON Schema 对象原样转发。
type Tool struct {
	Type     string       `json:"type"` // "function"
	Function ToolFunction `json:"function"`
}

type ToolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"` // JSON Schema
}

// ChatRequest 是一次 Chat 调用。Model 缺省回退 Provider 构造时 model;
// MaxTokens ≤0 不发送(部分网关模型需给,由调用方按阶段设)。
type ChatRequest struct {
	Model     string    `json:"model"`
	Messages  []Message `json:"messages"`
	Tools     []Tool    `json:"tools,omitempty"`
	MaxTokens int       `json:"max_tokens,omitempty"`
}

// ChatResponse 是 assistant 一次返回。ToolCalls 非空 → 需执行工具后回填再 Chat;
// FinishReason 归一:stop | tool_calls | length。
type ChatResponse struct {
	Content      string     `json:"content"`
	ToolCalls    []ToolCall `json:"tool_calls"`
	FinishReason string     `json:"finish_reason"`
}

// ---- 消息构造助手(调用方组会话用) ----

func Sys(text string) Message {
	return Message{Role: "system", Content: text}
}

func User(text string) Message {
	return Message{Role: "user", Content: text}
}

// Assistant 构造 assistant 文本回复。
func Assistant(text string) Message {
	return Message{Role: "assistant", Content: text}
}

// AssistantToolCalls 构造 assistant 纯工具轮(text 为空,线与网关 content=null 等价)。
func AssistantToolCalls(calls []ToolCall) Message {
	return Message{Role: "assistant", ToolCalls: calls}
}

// ToolResult 构造 role=tool 回填,引用某次 assistant tool_calls 的 id。
func ToolResult(callID, output string) Message {
	return Message{Role: "tool", ToolCallID: callID, Content: output}
}

// FuncCall 构造一次函数调用请求。arguments 为参数 JSON 序列化字符串
// (把结构体经 ArgumentsFromRaw 编码得到)。
func FuncCall(id, name, arguments string) ToolCall {
	return ToolCall{ID: id, Type: "function", Function: FunctionCall{Name: name, Arguments: arguments}}
}

// ToolDef 构造注入模型的工具描述;schema 为 JSON Schema 对象。
func ToolDef(name, description string, schema json.RawMessage) Tool {
	return Tool{Type: "function", Function: ToolFunction{Name: name, Description: description, Parameters: schema}}
}

// ---- arguments 互转助手 ----

// ArgumentsFromRaw 把参数 JSON 对象编码为网关线格式字符串(入模型用)。空 raw → "{}"。
func ArgumentsFromRaw(raw json.RawMessage) (string, error) {
	if len(raw) == 0 {
		return "{}", nil
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return "", fmt.Errorf("marshal arguments: %w", err)
	}
	return string(b), nil
}

// ArgumentsToRaw 解析网关返回的 arguments 字符串为 JSON 对象(Go 侧解析工具入参用)。
// 空/缺省串按 "{}" 处理。
func ArgumentsToRaw(args string) (json.RawMessage, error) {
	if strings.TrimSpace(args) == "" {
		return json.RawMessage("{}"), nil
	}
	var raw json.RawMessage
	if err := json.Unmarshal([]byte(args), &raw); err != nil {
		return nil, fmt.Errorf("parse arguments: %w", err)
	}
	return raw, nil
}
