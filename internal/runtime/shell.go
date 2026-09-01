package runtime

import (
	"bytes"
	"context"
	"os"
	"os/exec"

	"github.com/glacierzzz26/one-person-company-os/internal/task"
)

// ShellRuntime 在 Task 的工作区目录下执行其指令描述(Phase 1.1 语义:无 LLM Agent,
// Shell 直接执行 task.Description 作为命令)。受 ctx 超时控制。
type ShellRuntime struct{}

func NewShellRuntime() *ShellRuntime { return &ShellRuntime{} }

func init() { Register(NewShellRuntime()) }

func (s *ShellRuntime) Name() string { return "shell" }

func (s *ShellRuntime) Execute(ctx context.Context, t task.Task) (Result, error) {
	dir := t.WorkspacePath
	if dir == "" {
		dir = os.TempDir()
	} else if err := os.MkdirAll(dir, 0o755); err != nil {
		return Result{}, err
	}
	var out bytes.Buffer
	cmd := exec.CommandContext(ctx, "sh", "-c", t.Description)
	cmd.Dir = dir
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return Result{}, err
	}
	return Result{Output: out.String()}, nil
}
