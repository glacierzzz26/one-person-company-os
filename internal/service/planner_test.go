package service

import (
	"os"
	"testing"

	"github.com/glacierzzz26/one-person-company-os/internal/task"
)

func TestParsePlan(t *testing.T) {
	cases := []struct {
		name       string
		out        string
		wantAction string
		wantN      int // split 子任务数;-1 表示不校验
	}{
		{"direct JSON", `{"action":"direct"}`, planActionDirect, -1},
		{"direct fenced JSON", "```json\n{\"action\":\"direct\"}\n```", planActionDirect, -1},
		{"split JSON 2", `{"action":"split","subtasks":[{"title":"a","description":"A"},{"title":"b"}]}`, planActionSplit, 2},
		{"ask JSON reason", `{"action":"ask","reason":"too vague"}`, planActionAsk, -1},
		{"split over cap -> ask", `{"action":"split","subtasks":[{"title":"t%d"},{"title":"t%d"},{"title":"t%d"},{"title":"t%d"},{"title":"t%d"},{"title":"t%d"},{"title":"t%d"},{"title":"t%d"},{"title":"t9"}]}`, planActionAsk, -1},
		{"split no subtasks -> ask", `{"action":"split"}`, planActionAsk, -1},
		{"subtask missing title -> ask", `{"action":"split","subtasks":[{"description":"x"}]}`, planActionAsk, -1},
		{"regex direct fallback", `ACTION: direct`, planActionDirect, -1},
		{"regex split w/o specs -> ask", `ACTION: split`, planActionAsk, -1},
		{"garbage -> error", `hello world`, "", -1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parsePlan(tc.out)
			if tc.wantAction == "" {
				if err == nil {
					t.Fatalf("expected error, got %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.action != tc.wantAction {
				t.Fatalf("action = %q, want %q", got.action, tc.wantAction)
			}
			if tc.wantN >= 0 && len(got.subtasks) != tc.wantN {
				t.Fatalf("subtask count = %d, want %d", len(got.subtasks), tc.wantN)
			}
		})
	}
}

func TestEngScriptedPlan(t *testing.T) {
	tk := task.Task{Title: "big request", Description: "desc"}
	prev := os.Getenv("OS_SCRIPT_PLAN")
	defer os.Setenv("OS_SCRIPT_PLAN", prev)

	os.Setenv("OS_SCRIPT_PLAN", "")
	if p := engScriptedPlan(tk); p.action != planActionDirect {
		t.Fatalf("default should be direct, got %q", p.action)
	}
	os.Setenv("OS_SCRIPT_PLAN", "split:3")
	if p := engScriptedPlan(tk); p.action != planActionSplit || len(p.subtasks) != 3 {
		t.Fatalf("split:3 -> got action=%q n=%d", p.action, len(p.subtasks))
	}
	os.Setenv("OS_SCRIPT_PLAN", "split")
	if p := engScriptedPlan(tk); p.action != planActionSplit || len(p.subtasks) != 3 {
		t.Fatalf("split defaults to 3 -> got n=%d", len(p.subtasks))
	}
	os.Setenv("OS_SCRIPT_PLAN", "split:9")
	if p := engScriptedPlan(tk); p.action != planActionAsk {
		t.Fatalf("split:9 (>cap) should ask, got %q", p.action)
	}
	os.Setenv("OS_SCRIPT_PLAN", "ask:why?")
	if p := engScriptedPlan(tk); p.action != planActionAsk || p.reason != "why?" {
		t.Fatalf("ask: got action=%q reason=%q", p.action, p.reason)
	}
}
