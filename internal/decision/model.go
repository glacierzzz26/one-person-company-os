package decision

// Decision 人类/系统决策留痕(基线 §4.1 核心对象,项目目标「自动记录 Decision」)。
// kind: approval | goal | strategy | policy_change | capital | manual
type Decision struct {
	ID        string `json:"id"`
	CompanyID string `json:"company_id"`
	Title     string `json:"title"`
	Kind      string `json:"kind"`
	Status    string `json:"status"` // made | pending | executed
	Body      string `json:"body"`
	DecidedBy string `json:"decided_by"`
	Source    string `json:"source"` // 血缘:approval:<id> / manual
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}
