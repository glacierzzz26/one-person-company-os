package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// OpenAI 是 Phase 8.1 的 OpenAI Chat Completions 兼容客户端,对接用户自建网关
// (方向修订 A:网关单 key 聚合多厂商、function calling 透传底层模型,OS 只讲这一种方言)。
// 路径/鉴权沿用 service/endpoint.go models 拉取约定:{base}/v1/chat/completions,
// Authorization: Bearer <key>(key 为空 = 无鉴权本地网关,不发鉴权头)。
type OpenAI struct {
	baseURL string
	key     string
	model   string
	httpc   *http.Client
}

const chatTimeout = 5 * time.Minute

// NewOpenAI 构造网关 Chat Completions 客户端。
func NewOpenAI(baseURL, key, model string) *OpenAI {
	return &OpenAI{
		baseURL: strings.TrimRight(baseURL, "/"),
		key:     key,
		model:   model,
		httpc:   &http.Client{Timeout: chatTimeout},
	}
}

// withHTTPClient 注入自定义 HTTP 客户端(测试用,可设短超时)。
func (o *OpenAI) withHTTPClient(c *http.Client) *OpenAI {
	o.httpc = c
	return o
}

func (o *OpenAI) Model() string { return o.model }

// Chat 执行一次 Chat Completions 调用,把网关响应归一到 ChatResponse
// (Content 文本 / ToolCalls 工具调用 / FinishReason 停因)。非 2xx → 带体摘要错误。
func (o *OpenAI) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	if req.Model == "" {
		req.Model = o.model
	}
	if req.Model == "" {
		return ChatResponse{}, fmt.Errorf("openai chat: model required (endpoint selected_model or ChatRequest.Model)")
	}
	body := map[string]any{
		"model":    req.Model,
		"messages": req.Messages,
	}
	if len(req.Tools) > 0 {
		body["tools"] = req.Tools
	}
	if req.MaxTokens > 0 {
		body["max_tokens"] = req.MaxTokens
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("openai chat %s: marshal request: %w", req.Model, err)
	}
	url := o.baseURL + "/v1/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return ChatResponse{}, fmt.Errorf("openai chat %s: %w", req.Model, err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if o.key != "" {
		httpReq.Header.Set("Authorization", "Bearer "+o.key)
	}
	resp, err := o.httpc.Do(httpReq)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("openai chat %s: %w", req.Model, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return ChatResponse{}, fmt.Errorf("openai chat %s -> HTTP %d: %s", req.Model, resp.StatusCode, truncate(string(raw), 200))
	}
	return parseChatResponse(req.Model, raw)
}

// parseChatResponse 解析 Chat Completions 响应到中性 ChatResponse。
// 容错:choices 为空 → 错误;content 可为 null;tool_calls[].type 缺省补 "function";
// finish_reason:tool_calls → tool_calls;content_filter → stop;length → length。
func parseChatResponse(model string, raw []byte) (ChatResponse, error) {
	var out struct {
		Choices []struct {
			Message struct {
				Content   *string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Type     string `json:"type"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return ChatResponse{}, fmt.Errorf("openai chat %s: parse response: %w", model, err)
	}
	if len(out.Choices) == 0 {
		return ChatResponse{}, fmt.Errorf("openai chat %s: empty choices", model)
	}
	msg := out.Choices[0].Message
	res := ChatResponse{}
	if msg.Content != nil {
		res.Content = *msg.Content
	}
	for _, tc := range msg.ToolCalls {
		typ := tc.Type
		if typ == "" {
			typ = "function"
		}
		res.ToolCalls = append(res.ToolCalls, ToolCall{
			ID:       tc.ID,
			Type:     typ,
			Function: FunctionCall{Name: tc.Function.Name, Arguments: tc.Function.Arguments},
		})
	}
	fr := out.Choices[0].FinishReason
	if fr == "" && len(res.ToolCalls) > 0 {
		fr = "tool_calls" // 个别网关缺 finish_reason,有工具调用即未收敛
	}
	res.FinishReason = normalizeFinishReason(fr)
	return res, nil
}

// normalizeFinishReason 归一停因;content_filter 视同 stop(记录语义,不当成功/截断)。
func normalizeFinishReason(fr string) string {
	switch strings.ToLower(fr) {
	case "stop", "":
		return "stop"
	case "tool_calls":
		return "tool_calls"
	case "length":
		return "length"
	default: // content_filter 等罕见值,视同 stop 由上层按文本收敛
		return "stop"
	}
}
