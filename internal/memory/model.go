package memory

// Memory 公司知识沉淀条目(基线 §4.10)。type 对应结构化类型:
// lesson | knowledge | project_context | decision_ref | architecture | convention | task_history
type Memory struct {
	ID        string `json:"id"`
	CompanyID string `json:"company_id"`
	Type      string `json:"type"`
	Title     string `json:"title"`
	Content   string `json:"content"`
	Source    string `json:"source"` // 血缘:workflow:<id> / task:<id> / approval:<id> / manual
	Tags      string `json:"tags"`   // 空格分隔
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}
