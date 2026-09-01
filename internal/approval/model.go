package approval

// Approval 是人工审批节点。方向基线 6.3:Agent 不得伪造 Approval,
// 状态仅由人工命令(human:cli)变更。
type Approval struct {
	ID           string
	TaskID       string
	Risk         string
	Reason       string
	Status       string // pending | approved | rejected | changes
	RequestedBy  string
	DecidedBy    string
	DecisionNote string
	CreatedAt    int64
	DecidedAt    *int64
}
