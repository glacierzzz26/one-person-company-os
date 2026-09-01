package memory

// Memory 公司知识沉淀条目(基线 §4.10)。type 对应结构化类型:
// lesson | knowledge | project_context | decision_ref | architecture | convention | task_history
type Memory struct {
	ID        string
	CompanyID string
	Type      string
	Title     string
	Content   string
	Source    string // 血缘:workflow:<id> / task:<id> / approval:<id> / manual
	Tags      string // 空格分隔
	CreatedAt int64
	UpdatedAt int64
}
