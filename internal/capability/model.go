package capability

type Capability struct {
	ID          string `json:"id"`
	CompanyID   string `json:"company_id"`
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description"`
	CreatedAt   int64  `json:"created_at"`
	UpdatedAt   int64  `json:"updated_at"`
}
