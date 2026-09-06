package settings

// Phase 9 配置治理地基(方向 config-governance.md,9.1 契约 settings-foundation.md)。
// 领域模型对应三张配置表:app_setting(global 单行 id='self')/ company_setting(每公司覆盖行,
// 指针字段 NULL=继承 global)/ secret(company 机密,密文 enc:v2: 落 cipher 列)。

// AppSetting 全局配置单行。ConsoleTokenHash=” 表示未初始化(/setup 开放,9.2)。
type AppSetting struct {
	ID                string `json:"id"`                  // 'self'
	EngineModeDefault string `json:"engine_mode_default"` // live | scripted
	AgentCLIDefault   string `json:"agent_cli_default"`   // claude | codex
	ConsoleTokenHash  string `json:"console_token_hash"`  // '' = 未初始化
	DigestTime        string `json:"digest_time"`
	HTTPPort          int    `json:"http_port"`
	PollMin           int    `json:"poll_min"`
	QueueWork         bool   `json:"queue_work"` // 0|1(DB INTEGER;domain 用 bool)
	QueueIntervalSec  int    `json:"queue_interval_sec"`
	UpdatedAt         int64  `json:"updated_at"`
}

// 内置默认(运行时「无行」兜底,与迁移 0012 列默认一致)。
const (
	AppSettingID            = "self"
	DefaultEngineMode       = "live"
	DefaultAgentCLI         = "claude"
	DefaultDigestTime       = "09:00"
	DefaultHTTPPort         = 8787
	DefaultPollMin          = 5
	DefaultQueueIntervalSec = 10
)

// DefaultAppSetting 缺行时的内置默认(service 读 global 生效值用)。UpdatedAt=0 表示未落库。
func DefaultAppSetting() AppSetting {
	return AppSetting{
		ID:                AppSettingID,
		EngineModeDefault: DefaultEngineMode,
		AgentCLIDefault:   DefaultAgentCLI,
		DigestTime:        DefaultDigestTime,
		HTTPPort:          DefaultHTTPPort,
		PollMin:           DefaultPollMin,
		QueueIntervalSec:  DefaultQueueIntervalSec,
	}
}

// CompanySetting 每公司覆盖行。指针字段 nil=继承 global(空覆盖行 = 全继承)。
type CompanySetting struct {
	CompanyID        string  `json:"company_id"`
	EngineMode       *string `json:"engine_mode"` // NULL=继承 global
	AgentCLI         *string `json:"agent_cli"`
	IssueSource      *string `json:"issue_source"`
	IssueFixturePath *string `json:"issue_fixture_path"`
	UpdatedAt        int64   `json:"updated_at"`
}

// Secret company 机密。Cipher 为 enc:v2:<b64>(settings.SealSecret,主密钥);不回给前端(API 只出掩码,9.3)。
type Secret struct {
	CompanyID string `json:"company_id"`
	ID        string `json:"id"` // 'github_token' | 'feishu_webhook' | 'feishu_secret' …
	Cipher    string `json:"cipher"`
	UpdatedAt int64  `json:"updated_at"`
}
