package tool

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/glacierzzz26/one-person-company-os/internal/task"
)

// FileReadTool / FileWriteTool 在宿主 workspace 内读写文件。方向基线 7.3:文件作为工程工具。
// 最小权限拆分:读角色(review)只需 read/file,写角色(coding)才需要 write/file。
// 强制 workspace 边界:目标路径必须位于 task.workspace_path 之下,防目录穿越。
type FileReadTool struct{}

func init() {
	Register(&FileReadTool{})
	Register(&FileWriteTool{})
}

func (f *FileReadTool) Name() string        { return "file-read" }
func (f *FileReadTool) Description() string { return "读取 workspace 内文件内容(相对路径,禁止越界)" }

func (f *FileReadTool) Permission() Permission {
	return Permission{Action: "read", Resource: "file"}
}

func (f *FileReadTool) Risk() string { return "low" }

func (f *FileReadTool) InputSchema() string {
	return `{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`
}

func (f *FileReadTool) OutputSchema() string {
	return `{"type":"object","properties":{"content":{"type":"string"}},"required":["content"]}`
}

func (f *FileReadTool) Execute(ctx context.Context, t task.Task) (Result, error) {
	ws := t.WorkspacePath
	if ws == "" {
		return Result{}, fmt.Errorf("task %s: workspace path is empty", t.ID)
	}
	cmd := strings.TrimSpace(t.Description)
	path, _ := strings.CutPrefix(cmd, "read ")
	path = strings.TrimSpace(path)
	full, err := resolveWorkspacePath(ws, path)
	if err != nil {
		return Result{}, err
	}
	content, err := os.ReadFile(full)
	if err != nil {
		return Result{}, err
	}
	return Result{Output: string(content)}, nil
}

type FileWriteTool struct{}

func (f *FileWriteTool) Name() string        { return "file-write" }
func (f *FileWriteTool) Description() string { return "写入 workspace 内文件(格式:write <path>\\n<content>,禁止越界)" }

func (f *FileWriteTool) Permission() Permission {
	return Permission{Action: "write", Resource: "file"}
}

func (f *FileWriteTool) Risk() string { return "low" }

func (f *FileWriteTool) InputSchema() string {
	return `{"type":"object","properties":{"path":{"type":"string"},"content":{"type":"string"}},"required":["path"]}`
}

func (f *FileWriteTool) OutputSchema() string {
	return `{"type":"object","properties":{"output":{"type":"string"}},"required":["output"]}`
}

func (f *FileWriteTool) Execute(ctx context.Context, t task.Task) (Result, error) {
	ws := t.WorkspacePath
	if ws == "" {
		return Result{}, fmt.Errorf("task %s: workspace path is empty", t.ID)
	}
	desc := strings.TrimSpace(t.Description)
	rest, ok := strings.CutPrefix(desc, "write ")
	if !ok {
		return Result{}, errors.New("file-write command must start with 'write <path>\\n<content>'")
	}
	pathLine, content, _ := strings.Cut(rest, "\n")
	path := strings.TrimSpace(pathLine)
	full, err := resolveWorkspacePath(ws, path)
	if err != nil {
		return Result{}, err
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return Result{}, err
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		return Result{}, err
	}
	return Result{Output: fmt.Sprintf("wrote %s (%d bytes)", path, len(content))}, nil
}

// resolveWorkspacePath 把相对路径限定在 workspace 内:拒绝绝对路径与任何越界(../)组件。
func resolveWorkspacePath(ws, target string) (string, error) {
	if target == "" {
		return "", errors.New("path is required")
	}
	if filepath.IsAbs(target) {
		return "", errors.New("absolute path not allowed (must be relative to workspace)")
	}
	absWS, err := filepath.Abs(ws)
	if err != nil {
		return "", err
	}
	clean := filepath.Clean(target)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("path %q escapes workspace", target)
	}
	full := filepath.Join(absWS, clean)
	rel, err := filepath.Rel(absWS, full)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("path %q escapes workspace", target)
	}
	return full, nil
}
