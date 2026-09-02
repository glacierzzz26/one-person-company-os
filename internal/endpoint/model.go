package endpoint

// Endpoint 模型池接入端点(Phase 6.1)。Token 加密存储,明文不出库。
// proto:auto | anthropic | openai;role:pool | planner | standby;status:active | disabled。
type Endpoint struct {
	ID            string `json:"id"`
	CompanyID     string `json:"company_id"`
	Name          string `json:"name"`
	BaseURL       string `json:"base_url"`
	TokenEnc      string `json:"token_enc"` // aesgcm 密文(enc:v1:<b64>);空=无鉴权
	Proto         string `json:"proto"`     // auto | anthropic | openai
	Vendor        string `json:"vendor"`    // 厂商标识(anthropic/openai/local/其他,域名自动识别可手改)
	SelectedModel string `json:"selected_model"`
	Role          string `json:"role"`         // pool | planner | standby
	Status        string `json:"status"`       // active | disabled
	ModelsCache   string `json:"models_cache"` // json:最近一次 /v1/models 原始返回(可空)
	CreatedAt     int64  `json:"created_at"`
	UpdatedAt     int64  `json:"updated_at"`
}

// ModelInfo 是 /v1/models 返回中一个可用模型(用于 CLI 列表展示)。
type ModelInfo struct {
	ID string `json:"id"`
}
