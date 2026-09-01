package tool

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/glacierzzz26/one-person-company-os/internal/task"
)

// GitTool 在宿主 workspace 内调用 Git CLI(本地操作白名单),绕过 Docker 无网络限制。
// 方向基线 7.3:git 作为工程工具;网络操作(clone/pull/push/fetch)本阶段不支持,后续接入审批。
type GitTool struct{}

func init() { Register(&GitTool{}) }

func (g *GitTool) Name() string { return "git" }
func (g *GitTool) Description() string {
	return "在宿主 workspace 内执行本地 git 操作(init/status/add/commit/diff/log 等;网络操作不支持)"
}

func (g *GitTool) Permission() Permission {
	return Permission{Action: "execute", Resource: "git"}
}

func (g *GitTool) Risk() string { return "low" }

func (g *GitTool) InputSchema() string {
	return `{"type":"object","properties":{"command":{"type":"string"}},"required":["command"]}`
}

func (g *GitTool) OutputSchema() string {
	return `{"type":"object","properties":{"output":{"type":"string"}},"required":["output"]}`
}

// gitLocalSubcommands:本阶段放行的本地操作(默认拒绝其余)。
var gitLocalSubcommands = map[string]bool{
	"init": true, "status": true, "add": true, "rm": true, "mv": true,
	"commit": true, "diff": true, "log": true, "show": true, "branch": true,
	"config": true, "rev-parse": true, "tag": true, "grep": true,
}

// gitNetworkSubcommands:需要网络的操作,2.1 明确拒绝。
var gitNetworkSubcommands = map[string]bool{
	"clone": true, "pull": true, "push": true, "fetch": true,
	"remote": true, "ls-remote": true, "submodule": true,
}

func (g *GitTool) Execute(ctx context.Context, t task.Task) (Result, error) {
	ws := t.WorkspacePath
	if ws == "" {
		return Result{}, fmt.Errorf("task %s: workspace path is empty", t.ID)
	}
	if err := os.MkdirAll(ws, 0o755); err != nil {
		return Result{}, err
	}

	cmdline := strings.TrimSpace(t.Description)
	if rest, ok := strings.CutPrefix(cmdline, "git "); ok {
		cmdline = strings.TrimSpace(rest)
	}
	args, err := splitArgs(cmdline)
	if err != nil {
		return Result{}, fmt.Errorf("parse git command: %w", err)
	}
	if len(args) == 0 {
		return Result{}, errors.New("git command is empty")
	}
	sub := args[0]
	if gitNetworkSubcommands[sub] {
		return Result{}, fmt.Errorf("git %s is a network operation, not supported in Phase 2.1 (requires approval, later)", sub)
	}
	if !gitLocalSubcommands[sub] {
		return Result{}, fmt.Errorf("git subcommand %q not allowed", sub)
	}

	argv := append([]string{"-C", ws, sub}, args[1:]...)
	cmd := exec.CommandContext(ctx, "git", argv...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return Result{}, err
	}
	return Result{Output: out.String()}, nil
}

// splitArgs 按空白切分参数,支持单/双引号与反斜杠转义(用于 commit -m "msg with spaces")。
func splitArgs(s string) ([]string, error) {
	var args []string
	var cur strings.Builder
	inArg := false
	for i := 0; i < len(s); {
		switch c := s[i]; c {
		case ' ', '\t', '\n':
			if inArg {
				args = append(args, cur.String())
				cur.Reset()
				inArg = false
			}
			i++
		case '"', '\'':
			inArg = true
			quote := c
			i++
			for i < len(s) && s[i] != quote {
				if quote == '"' && s[i] == '\\' && i+1 < len(s) {
					i++
				}
				cur.WriteByte(s[i])
				i++
			}
			if i >= len(s) {
				return nil, fmt.Errorf("unterminated %c quote", quote)
			}
			i++
		default:
			inArg = true
			cur.WriteByte(c)
			i++
		}
	}
	if inArg {
		args = append(args, cur.String())
	}
	return args, nil
}
