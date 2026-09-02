package company

type Company struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Vision    string `json:"vision"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}
