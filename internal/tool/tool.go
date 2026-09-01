package tool

import (
	"context"

	"github.com/glacierzzz26/one-person-company-os/internal/task"
)

// Permission 是调用 Tool 所需的权限声明。方向基线 6.2:最小权限原则。
type Permission struct {
	Action   string // execute | read | write | network | secret | production | admin
	Resource string // shell | git | file | http | ...
}

// Result 是一次 Tool 调用的结果。
type Result struct {
	Output string
}

// Tool 是 Agent 能够使用的最小外部能力。方向基线 4.8:每个 Tool 必须有
// Definition / Permission / Input Schema / Output Schema / Risk Level / Audit。
type Tool interface {
	Name() string
	Description() string
	Permission() Permission
	Risk() string // low | medium | high
	InputSchema() string
	OutputSchema() string
	Execute(ctx context.Context, t task.Task) (Result, error)
}
