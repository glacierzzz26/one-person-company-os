package approval

// Approval 是人工审批节点。方向基线 6.3:Agent 不得伪造 Approval,
// 状态仅由人工命令(human:cli)变更。
type Approval struct {
	ID           string `json:"id"`
	TaskID       string `json:"task_id"`
	Risk         string `json:"risk"`
	Reason       string `json:"reason"`
	Status       string `json:"status"` // pending | approved | rejected | changes
	RequestedBy  string `json:"requested_by"`
	DecidedBy    string `json:"decided_by"`
	DecisionNote string `json:"decision_note"`
	CreatedAt    int64  `json:"created_at"`
	DecidedAt    *int64 `json:"decided_at"`
}
