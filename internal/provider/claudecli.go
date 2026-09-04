package provider

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Deprecated: ClaudeCLI 是 0-7 保留的 legacy 文本补全后端(exec claude -p,文本进/文本出)。
// 8.1 起模型文本与工具调用改走自建网关的 OpenAI 兼容 Chat(Chatter / OpenAI 实现,见 openai.go);
// claude Code agent 委派属 8.5(独立路径,复用 claude 二进制 agent 模式,非本类型)。0-7 路径继续可用。
//
// ClaudeCLI 是真调 claude CLI 无头模式(claude -p)的 Provider(Phase 6.1)。
// 换端点/换模型 = 换环境变量,零改码:ANTHROPIC_BASE_URL / ANTHROPIC_AUTH_TOKEN / ANTHROPIC_MODEL。
// 凭据两路:apiKeyEnv = API key 所在环境变量名(config 声明),Generate 时实时读取;
// token(NewClaudeEndpoint 直传,Phase 6.2 endpoint 解密 token)= 已解密明文,优先于 apiKeyEnv。
type ClaudeCLI struct {
	name      string
	model     string
	endpoint  string
	apiKeyEnv string
	token     string
}

const claudeTimeout = 30 * time.Minute

func NewClaude(name, model, endpoint, apiKeyEnv string) *ClaudeCLI {
	return &ClaudeCLI{name: name, model: model, endpoint: endpoint, apiKeyEnv: apiKeyEnv}
}

// NewClaudeEndpoint 由 endpoint 行(base_url + selected_model + 解密 token)构造
// 真实 Provider;token 为空 = 无鉴权本地端点。
func NewClaudeEndpoint(name, model, endpoint, token string) *ClaudeCLI {
	return &ClaudeCLI{name: name, model: model, endpoint: endpoint, token: token}
}

func (c *ClaudeCLI) Name() string     { return c.name }
func (c *ClaudeCLI) Type() string     { return "claude" }
func (c *ClaudeCLI) Model() string    { return c.model }
func (c *ClaudeCLI) Endpoint() string { return c.endpoint }

// Generate 执行一次无头生成。claude -p 输出按文本解析(非 JSON 流);错误带 stderr 摘要。
func (c *ClaudeCLI) Generate(ctx context.Context, req Request) (Response, error) {
	ctx, cancel := context.WithTimeout(ctx, claudeTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "claude", "-p", req.Prompt, "--output-format", "text")
	cmd.Env = append(os.Environ(), c.env()...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return Response{}, fmt.Errorf("claude -p (model %s): %w: %s", c.model, err, truncate(stderr.String(), 500))
	}
	return Response{Content: strings.TrimSpace(stdout.String())}, nil
}

func (c *ClaudeCLI) env() []string {
	out := []string{}
	if c.endpoint != "" {
		out = append(out, "ANTHROPIC_BASE_URL="+c.endpoint)
	}
	tok := c.token
	if tok == "" && c.apiKeyEnv != "" {
		tok = os.Getenv(c.apiKeyEnv)
	}
	if tok != "" {
		out = append(out, "ANTHROPIC_AUTH_TOKEN="+tok)
	}
	if c.model != "" {
		out = append(out, "ANTHROPIC_MODEL="+c.model)
	}
	return out
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// New 按 type 构造 Provider:stub → Stub(占位 mock);claude → ClaudeCLI(真调 claude CLI);未知 → Stub。
// mock/real 切换 = config providers[].type 设为 stub 或 claude,无 key 冒烟用 stub。
func New(typ, name, model, endpoint, apiKeyEnv string) Provider {
	if typ == "claude" {
		return NewClaude(name, model, endpoint, apiKeyEnv)
	}
	return NewStub(name, typ, model, endpoint)
}
