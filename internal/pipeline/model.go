package pipeline

// 声明式流水线形态(Phase 10 方向 D6;service 白名单校验,DB 不 CHECK)。
// 首期仅定义三形态;run 语义统一走 Engineering Driver(10.1),各形态差异自 10.2 起展开。
const (
	KindBugfix    = "bugfix"     // 缺陷修复:意图 → engineering driver(写→测→审→批)
	KindDevelop   = "develop"    // 特性开发(本期同 bugfix 执行语义,预留差异化)
	KindOpsPatrol = "ops_patrol" // 运维巡检(10.2 才真正展开:只读巡检 + 报告 + 发现处置)
)

var ValidKinds = []string{KindBugfix, KindDevelop, KindOpsPatrol}

// ValidKind 报告 kind 是否白名单内。首期接受全部三形态,但仅 bugfix/develop 能真正跑通 driver。
func ValidKind(k string) bool {
	for _, v := range ValidKinds {
		if k == v {
			return true
		}
	}
	return false
}

// 流水线状态:active | disabled。建即 active;run 前置要求 active(disabled 命中 409)。
const (
	StatusActive   = "active"
	StatusDisabled = "disabled"
)

// 风险护栏(与 task.Risk 同枚举;approval.go 按 risk=high 走审批门)。
const (
	RiskLow    = "low"
	RiskMedium = "medium"
	RiskHigh   = "high"
)

func ValidRisk(r string) bool {
	switch r {
	case RiskLow, RiskMedium, RiskHigh:
		return true
	}
	return false
}

type Pipeline struct {
	ID          string `json:"id"`
	ProjectID   string `json:"project_id"`
	Name        string `json:"name"`
	Kind        string `json:"kind"`        // bugfix | develop | ops_patrol
	Description string `json:"description"` // 意图(自然语言;run 无 request 时即 writer 请求)
	Risk        string `json:"risk"`        // low | medium | high
	Status      string `json:"status"`      // active | disabled
	Schedule    string `json:"schedule"`    // 10.2 才解析;本期恒空
	CreatedAt   int64  `json:"created_at"`
	UpdatedAt   int64  `json:"updated_at"`
}
