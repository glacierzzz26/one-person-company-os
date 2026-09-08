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

// 计划策略 plan_policy(Phase 10.4;契约 run-synthesis.md §3.1/§3.2)。
//
//	adaptive(缺省)= 现状 grow 记账(engineering run 认领走 runEngineering);
//	synthesize = frontier 对意图合成计划(首次认领)→ 落 upfront 账本 → 先审后干 → runSynthesized。
//
// service 白名单校验(非法 → ErrInvalid),DB 不 CHECK;ops_patrol 恒例行模板(policy 存而不用,模板优先)。
const (
	PlanPolicyAdaptive   = "adaptive"
	PlanPolicySynthesize = "synthesize"
)

var ValidPlanPolicies = []string{PlanPolicyAdaptive, PlanPolicySynthesize}

// ValidPlanPolicy 报告 plan_policy 是否白名单内。空 → 缺省 adaptive。
func ValidPlanPolicy(p string) bool {
	for _, v := range ValidPlanPolicies {
		if p == v {
			return true
		}
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
	PlanPolicy  string `json:"plan_policy"` // 10.4:adaptive | synthesize(缺省 adaptive)
	CreatedAt   int64  `json:"created_at"`
	UpdatedAt   int64  `json:"updated_at"`
}

// ScheduledPipeline 调度器视角的流水线子集(仅含判下次命中/触发 run 所需字段)。
// Phase 10.2:ListScheduledPipelines 的返回类型 —— active 且 schedule 非空;整行 Pipeline 的其余
// 字段(description/risk/status/created_at 等)对调度不透明,不零值伪造整行。
type ScheduledPipeline struct {
	ID        string `json:"id"`
	ProjectID string `json:"project_id"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Schedule  string `json:"schedule"`
}
