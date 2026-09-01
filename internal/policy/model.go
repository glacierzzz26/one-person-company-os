package policy

type Policy struct {
	ID        string
	CompanyID string
	Name      string
	Kind      string // allow | deny
	Statement string
	Enabled   bool
	CreatedAt int64
	UpdatedAt int64
}
