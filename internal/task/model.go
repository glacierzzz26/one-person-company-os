package task

type Task struct {
	ID            string
	CompanyID     string
	CapabilityID  *string
	WorkflowID    *string
	AgentID       *string
	Title         string
	Description   string
	Status        string // 业务状态: pending | running | waiting_approval | completed | failed
	Priority      int64
	Attempt       int64
	Risk          string // low | medium | high
	QStatus       string // 队列状态: ready | leased | running | waiting_approval | completed | failed
	LeaseWorkerID string
	LeaseUntil    int64
	MaxAttempts   int64
	TimeoutSec    int64
	LastError     string
	Result        string
	WorkspacePath string
	CreatedAt     int64
	UpdatedAt     int64
}
