package endpoint

// Endpoint 模型池接入端点(Phase 6.1)。Token 加密存储,明文不出库。
// proto:auto | anthropic | openai;role:pool | planner | standby;status:active | disabled。
type Endpoint struct {
	ID            string
	CompanyID     string
	Name          string
	BaseURL       string
	TokenEnc      string // aesgcm 密文(enc:v1:<b64>);空=无鉴权
	Proto         string // auto | anthropic | openai
	Vendor        string // 厂商标识(anthropic/openai/local/其他,域名自动识别可手改)
	SelectedModel string
	Role          string // pool | planner | standby
	Status        string // active | disabled
	ModelsCache   string // json:最近一次 /v1/models 原始返回(可空)
	CreatedAt     int64
	UpdatedAt     int64
}

// ModelInfo 是 /v1/models 返回中一个可用模型(用于 CLI 列表展示)。
type ModelInfo struct {
	ID string
}
