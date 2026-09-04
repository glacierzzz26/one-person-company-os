package provider

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

// 用例 4:ChatStub 确定性。同输入同输出;script 先吐 tool_calls 再收文本;耗尽重复最后;错误注入。
func TestChatStubDeterministic(t *testing.T) {
	ctx := context.Background()
	s := NewChatStubScript("stub-m",
		ChatResponse{ToolCalls: []ToolCall{FuncCall("c1", "write_file", `{"path":"a.go"}`)}, FinishReason: "tool_calls"},
		ChatResponse{Content: "ok", FinishReason: "stop"},
	)
	if s.Model() != "stub-m" {
		t.Errorf("Model() = %s, want stub-m", s.Model())
	}
	r1, err := s.Chat(ctx, ChatRequest{})
	if err != nil || len(r1.ToolCalls) != 1 || r1.ToolCalls[0].ID != "c1" || r1.FinishReason != "tool_calls" {
		t.Fatalf("call1 = %+v err=%v, want tool_calls c1", r1, err)
	}
	r2, err := s.Chat(ctx, ChatRequest{})
	if err != nil || r2.Content != "ok" || r2.FinishReason != "stop" || len(r2.ToolCalls) != 0 {
		t.Fatalf("call2 = %+v err=%v, want ok/stop", r2, err)
	}
	// 同输入两次同输出(耗尽管段:重复最后一条,确定性成立)
	r2b, err := s.Chat(ctx, ChatRequest{})
	if err != nil || !reflect.DeepEqual(r2, r2b) {
		t.Fatalf("call3 = %+v err=%v, want repeat of call2", r2b, err)
	}
}

func TestChatStubTextAndError(t *testing.T) {
	ctx := context.Background()
	text := NewChatStubText("m", "你好")
	a, err := text.Chat(ctx, ChatRequest{})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	b, _ := text.Chat(ctx, ChatRequest{})
	if !reflect.DeepEqual(a, b) || a.Content != "你好" || a.FinishReason != "stop" {
		t.Fatalf("text stub not deterministic: %+v vs %+v", a, b)
	}

	er := NewChatStubError("m", errors.New("boom"))
	if _, err := er.Chat(ctx, ChatRequest{}); err == nil {
		t.Fatal("want error from error stub")
	}
}
