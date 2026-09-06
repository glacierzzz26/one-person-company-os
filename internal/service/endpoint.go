package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/glacierzzz26/one-person-company-os/internal/endpoint"
	"github.com/google/uuid"
)

const modelsTimeout = 20 * time.Second

// AddEndpoint 接入一个模型端点:proto 校验、域名自动识别 vendor/proto、token 加密落库。写操作落 Audit。
func (s *Service) AddEndpoint(ctx context.Context, companyID, name, baseURL, token, proto string) (endpoint.Endpoint, error) {
	return s.AddEndpointAs(ctx, companyID, name, baseURL, token, proto, "human:cli")
}

// AddEndpointAs 同 AddEndpoint,审计 actor 用传入值(如 human:console)。
func (s *Service) AddEndpointAs(ctx context.Context, companyID, name, baseURL, token, proto, actor string) (endpoint.Endpoint, error) {
	if companyID == "" || name == "" || baseURL == "" {
		return endpoint.Endpoint{}, fmt.Errorf("--company, --name and --base-url are required")
	}
	if proto == "" {
		proto = "auto"
	}
	switch proto {
	case "auto", "anthropic", "openai":
	default:
		return endpoint.Endpoint{}, fmt.Errorf("--proto must be auto|anthropic|openai (got %q)", proto)
	}
	enc, err := endpoint.SealToken(strings.TrimSpace(token))
	if err != nil {
		return endpoint.Endpoint{}, err
	}
	vendor, defProto := sniffVendor(baseURL)
	if proto == "auto" {
		proto = defProto
	}
	now := time.Now().Unix()
	e := endpoint.Endpoint{
		ID: uuid.NewString(), CompanyID: companyID, Name: name, BaseURL: baseURL,
		TokenEnc: enc, Proto: proto, Vendor: vendor, SelectedModel: "",
		Role: "pool", Status: "active", ModelsCache: "",
		CreatedAt: now, UpdatedAt: now,
	}
	created, err := s.store.CreateEndpoint(ctx, e)
	if err != nil {
		return endpoint.Endpoint{}, err
	}
	_, err = s.audit(ctx, "endpoint", created.ID, "create", actor, name+" "+baseURL)
	return created, err
}

func (s *Service) ListEndpoints(ctx context.Context, companyID string) ([]endpoint.Endpoint, error) {
	return s.store.ListEndpoints(ctx, companyID)
}

func (s *Service) GetEndpoint(ctx context.Context, id string) (endpoint.Endpoint, error) {
	return s.store.GetEndpoint(ctx, id)
}

// FetchEndpointModels 测试连接并拉取 /v1/models,结果缓存 models_cache,返回可用模型列表。
// 返回的每个模型以 {ID: true} 形式给出,供 select 使用。
func (s *Service) FetchEndpointModels(ctx context.Context, id string) ([]endpoint.ModelInfo, error) {
	return s.FetchEndpointModelsAs(ctx, id, "human:cli")
}

// FetchEndpointModelsAs 同 FetchEndpointModels,审计 actor 用传入值。
func (s *Service) FetchEndpointModelsAs(ctx context.Context, id, actor string) ([]endpoint.ModelInfo, error) {
	e, err := s.store.GetEndpoint(ctx, id)
	if err != nil {
		return nil, err
	}
	token, err := endpoint.OpenToken(e.TokenEnc)
	if err != nil {
		return nil, err
	}
	body, err := fetchModelsJSON(ctx, e.BaseURL, e.Proto, token)
	if err != nil {
		return nil, err
	}
	models, err := parseModelIDs(body)
	if err != nil {
		return nil, err
	}
	// 缓存原始返回;selected_model 原值保留(select 另设)。
	if _, err := s.store.SetEndpointModel(ctx, e.ID, e.SelectedModel, string(body)); err != nil {
		return nil, err
	}
	_, err = s.audit(ctx, "endpoint", e.ID, "models", actor, fmt.Sprintf("%d model(s) cached", len(models)))
	return models, err
}

// SelectEndpointModel 选定模型(可选同时指派 role / tier)。写操作落 Audit。
func (s *Service) SelectEndpointModel(ctx context.Context, id, model, role, tier string) (endpoint.Endpoint, error) {
	return s.SelectEndpointModelAs(ctx, id, model, role, tier, "human:cli")
}

// SelectEndpointModelAs 同 SelectEndpointModel,审计 actor 用传入值(如 human:console)。
func (s *Service) SelectEndpointModelAs(ctx context.Context, id, model, role, tier, actor string) (endpoint.Endpoint, error) {
	if model == "" {
		return endpoint.Endpoint{}, fmt.Errorf("--model is required")
	}
	// 先校验 role/tier,再落库(避免 model 已更新而 role/tier 非法导致半写)。
	if role != "" {
		switch role {
		case "pool", "planner", "standby":
		default:
			return endpoint.Endpoint{}, fmt.Errorf("--role must be pool|planner|standby (got %q)", role)
		}
	}
	if tier != "" {
		switch tier {
		case tierFrontier, tierStandard, tierCheap:
		default:
			return endpoint.Endpoint{}, fmt.Errorf("--tier must be frontier|standard|cheap (got %q)", tier)
		}
	}
	e, err := s.store.SetEndpointModel(ctx, id, model, "")
	if err != nil {
		return endpoint.Endpoint{}, err
	}
	if role != "" {
		e, err = s.store.SetEndpointRole(ctx, id, role)
		if err != nil {
			return endpoint.Endpoint{}, err
		}
	}
	if tier != "" {
		e, err = s.store.SetEndpointTier(ctx, id, tier)
		if err != nil {
			return endpoint.Endpoint{}, err
		}
	}
	_, err = s.audit(ctx, "endpoint", e.ID, "select", actor, model+" role="+e.Role+" tier="+e.Tier)
	return e, err
}

// sniffVendor 由域名自动识别厂商标识与默认协议。识别不了 → vendor 空、proto 维持 auto。
func sniffVendor(baseURL string) (vendor, proto string) {
	host := strings.ToLower(baseURL)
	switch {
	case strings.Contains(host, "anthropic.com"):
		return "anthropic", "anthropic"
	case strings.Contains(host, "openai.com"):
		return "openai", "openai"
	default:
		return "", "auto"
	}
}

// fetchModelsJSON 调 {base}/v1/models。auto 协议先试 Anthropic 头、再试 OpenAI 头。
func fetchModelsJSON(ctx context.Context, baseURL, proto, token string) ([]byte, error) {
	url := strings.TrimRight(baseURL, "/") + "/v1/models"
	attempts := []func() (*http.Request, error){}
	switch proto {
	case "anthropic":
		attempts = append(attempts, func() (*http.Request, error) { return newModelsReq(ctx, url, "anthropic", token) })
	case "openai":
		attempts = append(attempts, func() (*http.Request, error) { return newModelsReq(ctx, url, "openai", token) })
	default: // auto
		attempts = append(attempts,
			func() (*http.Request, error) { return newModelsReq(ctx, url, "anthropic", token) },
			func() (*http.Request, error) { return newModelsReq(ctx, url, "openai", token) },
		)
	}
	var lastErr error
	for _, mk := range attempts {
		req, err := mk()
		if err != nil {
			lastErr = err
			continue
		}
		client := &http.Client{Timeout: modelsTimeout}
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("%s -> HTTP %d: %s", url, resp.StatusCode, truncate(string(body), 200))
			continue
		}
		return body, nil
	}
	return nil, lastErr
}

func newModelsReq(ctx context.Context, url, proto, token string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	switch proto {
	case "anthropic":
		req.Header.Set("x-api-key", token)
		req.Header.Set("anthropic-version", "2023-06-01")
	default: // openai
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
	}
	return req, nil
}

// parseModelIDs 兼容 {data:[{id}]} 与 {models:[{name/model}]} 两种 /v1/models 返回。
func parseModelIDs(body []byte) ([]endpoint.ModelInfo, error) {
	var data struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
		Models []struct {
			Name  string `json:"name"`
			Model string `json:"model"`
		} `json:"models"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("parse /v1/models response: %w", err)
	}
	out := []endpoint.ModelInfo{}
	for _, m := range data.Data {
		if m.ID != "" {
			out = append(out, endpoint.ModelInfo{ID: m.ID})
		}
	}
	for _, m := range data.Models {
		id := m.Model
		if id == "" {
			id = m.Name
		}
		if id != "" {
			out = append(out, endpoint.ModelInfo{ID: id})
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("/v1/models returned no models: %s", truncate(string(body), 200))
	}
	return out, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
