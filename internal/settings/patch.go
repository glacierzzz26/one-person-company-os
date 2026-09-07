package settings

// 设置部分更新 DTO(Phase 9.3,契约 runtime-knobs-web.md §3.6)。指针字段:JSON 缺省/nil = 不改该维度;
// 显式空串 = 清该维度覆盖(company,回退继承)。校验在 service 层(Update*As),此包只做形状。

// AppSettingPatch 全局设置部分更新(engine_mode_default Web 只收 "live",scripted 仅 env 测试 seam)。
type AppSettingPatch struct {
	EngineModeDefault *string `json:"engine_mode_default"`
	AgentCLIDefault   *string `json:"agent_cli_default"`
	DigestTime        *string `json:"digest_time"`
	HTTPPort          *int    `json:"http_port"`
	PollMin           *int    `json:"poll_min"`
	QueueWork         *bool   `json:"queue_work"`
	QueueIntervalSec  *int    `json:"queue_interval_sec"`
}

// CompanySettingPatch 公司覆盖部分更新。空串 = 清该维度(继承 global/默认);IssueSource=fixture 需
// 同时(或既有)给 issue_fixture_path。engine_mode 同上只收 "live"。
type CompanySettingPatch struct {
	EngineMode       *string `json:"engine_mode"`
	AgentCLI         *string `json:"agent_cli"`
	IssueSource      *string `json:"issue_source"`
	IssueFixturePath *string `json:"issue_fixture_path"`
}
