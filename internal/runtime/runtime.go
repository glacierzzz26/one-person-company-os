package runtime

import (
	"context"

	"github.com/glacierzzz26/one-person-company-os/internal/task"
)

// Result 是一次执行的结果。
type Result struct {
	Output string
}

// Runtime 是 Agent 的执行环境。方向基线 4.7：架构允许替换，通过 Registry 按名字取。
type Runtime interface {
	Name() string
	Execute(ctx context.Context, t task.Task) (Result, error)
}
