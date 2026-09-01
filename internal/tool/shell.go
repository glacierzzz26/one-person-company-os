package tool

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/glacierzzz26/one-person-company-os/internal/task"
)

// ShellTool 在 Docker Sandbox 内执行 Task 指令。方向基线 7.2:Sandbox 必须 Docker。
// 挂载 Task 独立 Workspace 到 /workspace,无网络、受限 CPU/内存。
type ShellTool struct{}

func init() { Register(&ShellTool{}) }

func (s *ShellTool) Name() string        { return "shell" }
func (s *ShellTool) Description() string { return "在 Docker 沙箱内执行 shell 命令(无网络、受限资源)" }

// Permission:subject 由调用方按 agent.Role 解析,此处只声明 Action/Resource。
func (s *ShellTool) Permission() Permission {
	return Permission{Action: "execute", Resource: "shell"}
}

func (s *ShellTool) Risk() string { return "medium" }

func (s *ShellTool) InputSchema() string {
	return `{"type":"object","properties":{"command":{"type":"string"}},"required":["command"]}`
}

func (s *ShellTool) OutputSchema() string {
	return `{"type":"object","properties":{"output":{"type":"string"}},"required":["output"]}`
}

const shellImage = "busybox:latest"

func (s *ShellTool) Execute(ctx context.Context, t task.Task) (Result, error) {
	ws := t.WorkspacePath
	if ws == "" {
		return Result{}, fmt.Errorf("task %s: workspace path is empty", t.ID)
	}
	if err := os.MkdirAll(ws, 0o755); err != nil {
		return Result{}, err
	}
	args := []string{
		"run", "--rm",
		"--network", "none",
		"--cpus", "1",
		"--memory", "256m",
		"-v", ws + ":/workspace",
		"-w", "/workspace",
		shellImage,
		"sh", "-c", t.Description,
	}
	cmd := exec.CommandContext(ctx, "docker", args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return Result{}, err
	}
	return Result{Output: out.String()}, nil
}
