package agent

type Agent struct {
	ID           string `json:"id"`
	CapabilityID string `json:"capability_id"`
	Name         string `json:"name"`
	Role         string `json:"role"`
	ModelHint    string `json:"model_hint"`
	CreatedAt    int64  `json:"created_at"`
	UpdatedAt    int64  `json:"updated_at"`
}
