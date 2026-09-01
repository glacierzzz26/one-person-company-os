package decision

// Decision 人类/系统决策留痕(基线 §4.1 核心对象,项目目标「自动记录 Decision」)。
// kind: approval | goal | strategy | policy_change | capital | manual
type Decision struct {
	ID        string
	CompanyID string
	Title     string
	Kind      string
	Status    string // made | pending | executed
	Body      string
	DecidedBy string
	Source    string // 血缘:approval:<id> / manual
	CreatedAt int64
	UpdatedAt int64
}
