package workflow

type Workflow struct {
	ID          string
	CompanyID   string
	Name        string
	Description string
	Definition  string // JSON 文本，Phase 0 仅存储
	CreatedAt   int64
	UpdatedAt   int64
}
