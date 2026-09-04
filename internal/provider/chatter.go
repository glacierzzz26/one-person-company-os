package provider

import "context"

// Chatter 是 Phase 8.1 起的模型生成契约(OpenAI Chat Completions 方言,对接自建网关)。
// 取代 Generate 的"单次文本补全"形态:
//
//	调用方持完整会话,循环:
//	  resp, _ := Chat(req)                       → assistant 文本 与/或 ToolCalls
//	  有 ToolCalls → Go 执行工具 → 逐条 ToolResult 回填追加进 Messages → 再 Chat
//	  无 ToolCalls  → 阶段收敛(文本或结构化信号由 8.2/8.3 判读)
//
// Provider/Generate 为 0-7 兼容保留(见 provider.go / claudecli.go,标注废弃不删);
// 8.x 新能力一律走 Chatter。实现:OpenAI(网关)、ChatStub(离线确定性档)。
type Chatter interface {
	Model() string
	Chat(ctx context.Context, req ChatRequest) (ChatResponse, error)
}
