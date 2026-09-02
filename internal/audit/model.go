package audit

type Audit struct {
	ID         string `json:"id"`
	EntityType string `json:"entity_type"`
	EntityID   string `json:"entity_id"`
	Action     string `json:"action"`
	Actor      string `json:"actor"`
	Detail     string `json:"detail"`
	CreatedAt  int64  `json:"created_at"`
}
