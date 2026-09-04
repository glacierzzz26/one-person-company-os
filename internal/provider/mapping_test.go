package provider

import (
	"encoding/json"
	"reflect"
	"testing"
)

// 用例 5:映射对拍。网关原生响应 JSON → 中性 ChatResponse 关键字段无损(表驱动),
// 含并行 tool_calls、content=null、arguments 转义;停因归一;空 choices 报错。
func TestParseChatResponseMapping(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want ChatResponse
	}{
		{
			name: "text",
			raw:  `{"choices":[{"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}]}`,
			want: ChatResponse{Content: "hi", FinishReason: "stop"},
		},
		{
			name: "parallel_tool_calls_null_content",
			raw:  `{"choices":[{"message":{"role":"assistant","content":null,"tool_calls":[{"id":"a1","type":"function","function":{"name":"read_file","arguments":"{\"path\":\"x\"}"}},{"id":"a2","type":"function","function":{"name":"write_file","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`,
			want: ChatResponse{
				FinishReason: "tool_calls",
				ToolCalls: []ToolCall{
					FuncCall("a1", "read_file", `{"path":"x"}`),
					FuncCall("a2", "write_file", `{}`),
				},
			},
		},
		{
			name: "missing_type_defaults_function",
			raw:  `{"choices":[{"message":{"role":"assistant","content":"","tool_calls":[{"id":"b1","function":{"name":"run","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`,
			want: ChatResponse{
				FinishReason: "tool_calls",
				ToolCalls:    []ToolCall{FuncCall("b1", "run", `{}`)},
			},
		},
		{
			name: "length_truncated",
			raw:  `{"choices":[{"message":{"role":"assistant","content":"part"},"finish_reason":"length"}]}`,
			want: ChatResponse{Content: "part", FinishReason: "length"},
		},
		{
			name: "content_filter_as_stop",
			raw:  `{"choices":[{"message":{"role":"assistant","content":"filtered"},"finish_reason":"content_filter"}]}`,
			want: ChatResponse{Content: "filtered", FinishReason: "stop"},
		},
		{
			name: "escaped_arguments",
			raw:  `{"choices":[{"message":{"role":"assistant","content":null,"tool_calls":[{"id":"c1","type":"function","function":{"name":"write_file","arguments":"{\"path\":\"a\\tb.go\",\"content\":\"line\\nbreak\"}"}}]},"finish_reason":"tool_calls"}]}`,
			want: ChatResponse{
				FinishReason: "tool_calls",
				ToolCalls:    []ToolCall{FuncCall("c1", "write_file", `{"path":"a\tb.go","content":"line\nbreak"}`)},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseChatResponse("m", []byte(tc.raw))
			if err != nil {
				t.Fatalf("parseChatResponse: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestParseChatResponseEmptyChoices(t *testing.T) {
	if _, err := parseChatResponse("m", []byte(`{"choices":[]}`)); err == nil {
		t.Fatal("want error on empty choices")
	}
	if _, err := parseChatResponse("m", []byte(`not json`)); err == nil {
		t.Fatal("want error on bad json")
	}
}

func TestArgumentsRoundTrip(t *testing.T) {
	raw := json.RawMessage(`{"path":"a b","n":1}`)
	s, err := ArgumentsFromRaw(raw)
	if err != nil {
		t.Fatalf("ArgumentsFromRaw: %v", err)
	}
	if s != `{"path":"a b","n":1}` {
		t.Errorf("arguments = %q", s)
	}
	back, err := ArgumentsToRaw(s)
	if err != nil {
		t.Fatalf("ArgumentsToRaw: %v", err)
	}
	if !reflect.DeepEqual(back, raw) {
		t.Errorf("round trip = %s, want %s", back, raw)
	}
	// 空串 → "{}"
	e, err := ArgumentsToRaw("")
	if err != nil || string(e) != "{}" {
		t.Errorf("empty args = %s err=%v, want {}", e, err)
	}
	// 坏 JSON → 报错
	if _, err := ArgumentsToRaw(`{bad`); err == nil {
		t.Error("want error on bad arguments JSON")
	}
}
