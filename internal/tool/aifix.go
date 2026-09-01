package tool

import (
	"context"
	"fmt"
	"strings"

	"github.com/glacierzzz26/one-person-company-os/internal/task"
)

// AifixTool 是 AI-Fix 外部能力(AI 自动修复)的接入占位。方向基线:AI-Fix 作为
// 工程能力之一。契约 = 标准 Tool 接口(execute/aifix, low);Phase 2 为 mock
// 实现——真实接入端点约定见 docs/phase2/design/engineering-capability.md §8。
type AifixTool struct{}

func init() { Register(&AifixTool{}) }

func (t *AifixTool) Name() string { return "aifix" }
func (t *AifixTool) Description() string {
	return "AI 自动修复(mock):输入问题描述,返回模拟修复方案"
}

func (t *AifixTool) Permission() Permission {
	return Permission{Action: "execute", Resource: "aifix"}
}

func (t *AifixTool) Risk() string { return "low" }

func (t *AifixTool) InputSchema() string {
	return `{"type":"object","properties":{"problem":{"type":"string"}},"required":["problem"]}`
}

func (t *AifixTool) OutputSchema() string {
	return `{"type":"object","properties":{"fix":{"type":"string"}},"required":["fix"]}`
}

func (t *AifixTool) Execute(_ context.Context, task task.Task) (Result, error) {
	problem := strings.TrimSpace(task.Description)
	problem = strings.TrimPrefix(problem, "aifix ")
	return Result{Output: fmt.Sprintf("[mock fix] 对问题 %q 生成修复方案(真实 AI-Fix 待接入,见 §8)", strings.TrimSpace(problem))}, nil
}
