package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// wire 类型:网关收到的请求(用于断言请求形状)。
type wireToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type wireMessage struct {
	Role       string         `json:"role"`
	Content    *string        `json:"content"`
	ToolCallID string         `json:"tool_call_id"`
	ToolCalls  []wireToolCall `json:"tool_calls"`
}

type wireTool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Parameters  json.RawMessage `json:"parameters"`
	} `json:"function"`
}

type wireReq struct {
	Model     string        `json:"model"`
	Messages  []wireMessage `json:"messages"`
	Tools     []wireTool    `json:"tools"`
	MaxTokens int           `json:"max_tokens"`
}

func strp(s string) *string { return &s }

// 用例 1:请求形状(文本)。断言 method/路径/鉴权头/body(system 首条/tools 函数形态/max_tokens)。
func TestOpenAIChatRequestShape(t *testing.T) {
	var method, path, auth, ctype string
	var got wireReq
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		auth, ctype = r.Header.Get("Authorization"), r.Header.Get("Content-Type")
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode request: %v", err)
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"你好网关"},"finish_reason":"stop"}]}`))
	}))
	defer srv.Close()

	c := NewOpenAI(srv.URL, "k-123", "m-sonnet").withHTTPClient(srv.Client())
	resp, err := c.Chat(context.Background(), ChatRequest{
		Messages:  []Message{Sys("你是研发"), User("写一个函数")},
		Tools:     []Tool{ToolDef("write_file", "写文件", json.RawMessage(`{"type":"object"}`))},
		MaxTokens: 512,
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if method != http.MethodPost {
		t.Errorf("method = %s, want POST", method)
	}
	if path != "/v1/chat/completions" {
		t.Errorf("path = %s, want /v1/chat/completions", path)
	}
	if auth != "Bearer k-123" {
		t.Errorf("auth = %q, want Bearer k-123", auth)
	}
	if !strings.Contains(ctype, "application/json") {
		t.Errorf("content-type = %s, want application/json", ctype)
	}
	if got.Model != "m-sonnet" {
		t.Errorf("model = %s, want m-sonnet", got.Model)
	}
	if got.MaxTokens != 512 {
		t.Errorf("max_tokens = %d, want 512", got.MaxTokens)
	}
	if len(got.Messages) != 2 || got.Messages[0].Role != "system" || *got.Messages[0].Content != "你是研发" ||
		got.Messages[1].Role != "user" || *got.Messages[1].Content != "写一个函数" {
		t.Errorf("messages shape wrong: %+v", got.Messages)
	}
	if len(got.Tools) != 1 || got.Tools[0].Type != "function" || got.Tools[0].Function.Name != "write_file" ||
		got.Tools[0].Function.Description != "写文件" {
		t.Errorf("tools shape wrong: %+v", got.Tools)
	}
	if resp.Content != "你好网关" || resp.FinishReason != "stop" || len(resp.ToolCalls) != 0 {
		t.Errorf("response = %+v, want text=你好网关 stop", resp)
	}
}

// 用例 2:tool 回传(两轮)。首轮应答 tool_calls → 归一;携回填再 Chat → 假网关断言请求形状 → 收文本。
func TestOpenAIToolRoundTrip(t *testing.T) {
	var bodies []wireReq
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var wreq wireReq
		if err := json.NewDecoder(r.Body).Decode(&wreq); err != nil {
			t.Errorf("decode request: %v", err)
		}
		bodies = append(bodies, wreq)
		switch len(bodies) {
		case 1:
			_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":null,"tool_calls":[{"id":"call_1","type":"function","function":{"name":"write_file","arguments":"{\"path\":\"a.go\",\"content\":\"x\"}"}}]},"finish_reason":"tool_calls"}]}`))
		default:
			_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"done"},"finish_reason":"stop"}]}`))
		}
	}))
	defer srv.Close()

	c := NewOpenAI(srv.URL, "k", "m").withHTTPClient(srv.Client())
	history := []Message{Sys("你是研发"), User("改文件")}

	r1, err := c.Chat(context.Background(), ChatRequest{Messages: history})
	if err != nil {
		t.Fatalf("Chat round1: %v", err)
	}
	if len(r1.ToolCalls) != 1 {
		t.Fatalf("round1 ToolCalls = %d, want 1", len(r1.ToolCalls))
	}
	tc := r1.ToolCalls[0]
	if tc.ID != "call_1" || tc.Function.Name != "write_file" {
		t.Errorf("tool call = %+v, want call_1/write_file", tc)
	}
	if r1.FinishReason != "tool_calls" || r1.Content != "" {
		t.Errorf("round1 = %+v, want tool_calls empty content", r1)
	}
	if _, err := ArgumentsToRaw(tc.Function.Arguments); err != nil {
		t.Errorf("arguments not valid JSON: %v", err)
	}

	next := append(history, AssistantToolCalls(r1.ToolCalls), ToolResult("call_1", "written"))
	r2, err := c.Chat(context.Background(), ChatRequest{Messages: next})
	if err != nil {
		t.Fatalf("Chat round2: %v", err)
	}
	if r2.Content != "done" || r2.FinishReason != "stop" || len(r2.ToolCalls) != 0 {
		t.Errorf("round2 = %+v, want done/stop", r2)
	}

	// 假网关断言第二请求:assistant 工具轮原样 + role=tool 回填(tool_call_id/content)。
	if len(bodies) != 2 {
		t.Fatalf("gateway saw %d calls, want 2", len(bodies))
	}
	var assistantSeen, toolSeen bool
	for _, m := range bodies[1].Messages {
		switch m.Role {
		case "assistant":
			assistantSeen = true
			if m.Content != nil || len(m.ToolCalls) != 1 || m.ToolCalls[0].ID != "call_1" ||
				m.ToolCalls[0].Function.Name != "write_file" {
				t.Errorf("round2 assistant msg wrong: %+v", m)
			}
		case "tool":
			toolSeen = true
			if m.ToolCallID != "call_1" || m.Content == nil || *m.Content != "written" {
				t.Errorf("round2 tool result wrong: %+v", m)
			}
		}
	}
	if !assistantSeen || !toolSeen {
		t.Errorf("round2 messages missing assistant/tool roles: %+v", bodies[1].Messages)
	}
}

// 用例 3:错误路径。非 2xx → error 带 HTTP 码与体摘要;Bearer 头发出;空 key 不发鉴权头。
func TestOpenAIChatHTTPError(t *testing.T) {
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		http.Error(w, `{"error":{"message":"bad key"}}`, http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := NewOpenAI(srv.URL, "k", "m").withHTTPClient(srv.Client())
	_, err := c.Chat(context.Background(), ChatRequest{Messages: []Message{User("hi")}})
	if err == nil {
		t.Fatal("want error on 401")
	}
	if !strings.Contains(err.Error(), "HTTP 401") || !strings.Contains(err.Error(), "bad key") {
		t.Errorf("error = %q, want HTTP 401 + body excerpt", err)
	}
	if auth != "Bearer k" {
		t.Errorf("auth = %q, want Bearer k on error path", auth)
	}

	var auth2 string
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth2 = r.Header.Get("Authorization")
		http.Error(w, `nope`, http.StatusBadRequest)
	}))
	defer srv2.Close()
	c2 := NewOpenAI(srv2.URL, "", "m").withHTTPClient(srv2.Client())
	if _, err := c2.Chat(context.Background(), ChatRequest{Messages: []Message{User("hi")}}); err == nil {
		t.Fatal("want error on 400")
	}
	if auth2 != "" {
		t.Errorf("empty key: auth = %q, want none", auth2)
	}
}
