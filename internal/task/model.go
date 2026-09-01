package task

type Task struct {
	ID           string
	CompanyID    string
	CapabilityID *string
	WorkflowID   *string
	AgentID      *string
	Title        string
	Description  string
	Status       string // pending | running | waiting_approval | completed | failed
	Priority     int64
	Attempt      int64
	Risk         string // low | medium | high
	CreatedAt    int64
	UpdatedAt    int64
}
