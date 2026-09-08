package task

type Task struct {
	ID            string  `json:"id"`
	CompanyID     string  `json:"company_id"`
	CapabilityID  *string `json:"capability_id"`
	WorkflowID    *string `json:"workflow_id"`
	AgentID       *string `json:"agent_id"`
	Title         string  `json:"title"`
	Description   string  `json:"description"`
	ToolName      string  `json:"tool_name"` // 执行该 Task 的 Tool 名(默认 shell;engineering = Engineering Driver)
	Status        string  `json:"status"`    // 业务状态: pending | running | waiting_approval | completed | failed
	Priority      int64   `json:"priority"`
	Attempt       int64   `json:"attempt"`
	Risk          string  `json:"risk"`     // low | medium | high
	QStatus       string  `json:"q_status"` // 队列状态: ready | leased | running | waiting_approval | completed | failed
	LeaseWorkerID string  `json:"lease_worker_id"`
	LeaseUntil    int64   `json:"lease_until"`
	MaxAttempts   int64   `json:"max_attempts"`
	TimeoutSec    int64   `json:"timeout_sec"`
	LastError     string  `json:"last_error"`
	Result        string  `json:"result"`
	WorkspacePath string  `json:"workspace_path"`
	// Phase 6.2:Engineering Driver 回合状态(回合在驱动内部,不另建状态机)。
	ParentTaskID       *string `json:"parent_task_id"`       // planner 拆解的子任务挂父请求;空 = 顶层任务
	RoundNo            int64   `json:"round_no"`             // 当前评审回合(写→测→审为一轮)
	ConflictCount      int64   `json:"conflict_count"`       // reviewer 驳回累计;test 失败不累计
	WriterEndpointID   *string `json:"writer_endpoint_id"`   // 写者模型端点;空 = agent 默认
	ReviewerEndpointID *string `json:"reviewer_endpoint_id"` // 审阅模型端点;空 = agent 默认
	TestEndpointID     *string `json:"test_endpoint_id"`     // test 判读端点(8.4;默认 standard 档,与 review 分槽);空 = 回退 reviewer/writer
	ProjectID          *string `json:"project_id"`           // Phase 10.1:流水线 run 产物挂项目;空 = 非流水线任务
	PipelineID         *string `json:"pipeline_id"`          // Phase 10.2:run 反链流水线(认领判 kind 分流 / Web 显形态);空 = 非流水线 run 产物
	CreatedAt          int64   `json:"created_at"`
	UpdatedAt          int64   `json:"updated_at"`
}
