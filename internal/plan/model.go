package plan

// run 计划账本领域模型(Phase 10.3;契约 phase-plan-contract.md §3.1)。
// 每条流水线 run(task_id)一份 RunPlan + 有序 RunPlanPhase 列表;service 层只写读 + 驱动内部翻状态,
// 无外部写口。计划整体状态 = task.status 单一来源(plan 表不重复存总状态)。

// 计划形态 kind(与驱动分流同源 —— pipeline.kind:ops_patrol → patrol;bugfix/develop → engineering)。
const (
	PlanKindPatrol      = "patrol"
	PlanKindEngineering = "engineering"
)

// materialized:计划落账时机。upfront = 建单即铺全(可先审后干;ops_patrol);grow = 执行中 append(engineering)。
const (
	MaterializedUpfront = "upfront"
	MaterializedGrow    = "grow"
)

// phase.kind:阶段类型。
const (
	PhaseKindDo      = "do"      // 执行产出(委派写 / OS 提交报告)
	PhaseKindAccept  = "accept"  // 验收判读(机械预检 / frontier/scripted 判读)
	PhaseKindDispose = "dispose" // 处置(发现链拉 / 手动留人)
)

// phase.status:单阶段状态(驱动内部翻;pending → running → ok|fail|skipped)。
const (
	PhaseStatusPending = "pending"
	PhaseStatusRunning = "running"
	PhaseStatusOK      = "ok"
	PhaseStatusFail    = "fail"
	PhaseStatusSkipped = "skipped"
)

// phase.allocator:阶段执行者归属(Web 展示用)。
const (
	AllocatorDelegate = "delegate" // 委派 agent 干(写/巡检)
	AllocatorOS       = "os"       // OS 直跑(提交/机械预检)
	AllocatorJudge    = "judge"    // 判读端点(测试/评审/巡检 verdict)
	AllocatorPlanner  = "planner"  // planner 序曲(拆解)
	AllocatorManual   = "manual"   // 人工(处置留人)
)

// RunPlan 一份流水线 run 的计划账本行(一条 run 一份;task_id UNIQUE)。
type RunPlan struct {
	ID           string `json:"id"`
	TaskID       string `json:"task_id"`
	Kind         string `json:"kind"`         // patrol | engineering
	Materialized string `json:"materialized"` // upfront | grow
	CreatedAt    int64  `json:"created_at"`
	UpdatedAt    int64  `json:"updated_at"`
}

// RunPlanPhase 有序阶段行(seq 升序;grow 计划 append 递增)。
type RunPlanPhase struct {
	ID         string `json:"id"`
	PlanID     string `json:"plan_id"`
	Seq        int64  `json:"seq"`
	Kind       string `json:"kind"`      // do | accept | dispose
	Title      string `json:"title"`     // 人类可读阶段目标(round/报告名等)
	Allocator  string `json:"allocator"` // delegate | os | judge | planner | manual
	Status     string `json:"status"`    // pending | running | ok | fail | skipped
	Evidence   string `json:"evidence"`  // 产出引用/摘要(报告 rel/verdict/exec id/commit;不整存大产出)
	Note       string `json:"note"`
	StartedAt  *int64 `json:"started_at"`
	FinishedAt *int64 `json:"finished_at"`
}
