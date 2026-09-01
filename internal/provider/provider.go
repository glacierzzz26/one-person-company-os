package provider

import (
	"context"
	"fmt"
)

// Request 是一次模型生成请求。Phase 2 仅定义契约,不产生真实模型调用。
type Request struct {
	Prompt string
}

// Response 是模型生成结果。
type Response struct {
	Content string
}

// Provider 是模型提供方抽象。方向基线:Agent → Runtime → Model。
// 属性(Name/Type/Model/Endpoint)来自 config 声明;Generate 为契约入口,
// Phase 2 所有实现为 stub(mock 或 not implemented),真实接入后续落地。
type Provider interface {
	Name() string
	Type() string
	Model() string
	Endpoint() string
	Generate(ctx context.Context, req Request) (Response, error)
}

// Stub 是未接入真实 LLM 的占位 Provider:属性来自 config,
// Generate 返回 mock 内容(真实接入约定见 engineering-capability.md §8)。
type Stub struct {
	name     string
	typ      string
	model    string
	endpoint string
}

func NewStub(name, typ, model, endpoint string) *Stub {
	return &Stub{name: name, typ: typ, model: model, endpoint: endpoint}
}

func (s *Stub) Name() string     { return s.name }
func (s *Stub) Type() string     { return s.typ }
func (s *Stub) Model() string    { return s.model }
func (s *Stub) Endpoint() string { return s.endpoint }

// Generate 返回 mock 内容;API key 由 config 的 api_key_env 注入(真实接入时)。
func (s *Stub) Generate(_ context.Context, req Request) (Response, error) {
	return Response{Content: fmt.Sprintf("[mock %s/%s] %s", s.typ, s.model, req.Prompt)}, nil
}
