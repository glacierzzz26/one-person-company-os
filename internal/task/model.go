package task

type Task struct {
	ID            string
	CompanyID     string
	CapabilityID  *string
	WorkflowID    *string
	AgentID       *string
	Title         string
	Description   string
	ToolName      string // 执行该 Task 的 Tool 名(默认 shell;engineering = Engineering Driver)
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
	// Phase 6.2:Engineering Driver 回合状态(回合在驱动内部,不另建状态机)。
	ParentTaskID       *string // planner 拆解的子任务挂父请求;空 = 顶层任务
	RoundNo            int64   // 当前评审回合(写→测→审为一轮)
	ConflictCount      int64   // reviewer 驳回累计;test 失败不累计
	WriterEndpointID   *string // 写者模型端点;空 = agent 默认
	ReviewerEndpointID *string // 审阅模型端点;空 = agent 默认
	CreatedAt          int64
	UpdatedAt          int64
}
