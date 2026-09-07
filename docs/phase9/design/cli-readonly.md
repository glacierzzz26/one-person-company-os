# Phase 9.4 — CLI 只读收口:env seam 反向门控 + server 业务 flag 下线 + 残留文案清理(实施契约)

> Phase 9 收尾子阶段实施契约,承接方向 [config-governance.md](config-governance.md)(定稿)§六 9.4 行 + §五 D3/D4 + §二.6/§八.1 验收;9.3([runtime-knobs-web.md](runtime-knobs-web.md),已冻结,见 [stage 3](../stages/3.md))显式 defer 的「CLI 写命令收口 / env 读取残余删除 / flag 彻底下线」在本阶段落地。
> 定稿门(2026-09-07,AskUserQuestion 三项):**① env seam 门控 = 反向默认(测试开、产品恒关)**——service/endpoint 包级 `envSeam` 默认 false,经 `SetEnvSeam(true)` 在测试打开;产品(cmd/os 构造链)零改动即天然不读 env,结构性兑现「除 --db/--config 外无任何产品配置经 env 注入」(方向 §八.1);**② CLI 收口深度 = 收 env/flag + 文案清理,领域写命令全保留**——仓库核实(见 §二 2.3)配置收编面从无独立 CLI 写命令(全是 env/flag 渠道,9.1-9.3 已 DB 化),现存 CLI 写命令全是领域/执行对象,非 Phase 9 收编面;**③ server 业务 flag = 删除**——`--port/--poll/--digest/--queue-work/--queue-interval` 全下,保留 `--db/--config`(数据定位)+ `--digest-now`(一次性触发,非配置)。
> 代码现状立足点 = **9.3 落地后工作树**(commit `a84ecce`:runtime 旋钮 DB 化 + service/endpoint/settings 三 holder + `os server` 零参数 + settings 全 API + 6 卡设置页 + rotate-master-key,已提交)。

## 一、范围与边界

**做(9.4 交付物):**

1. **env seam 反向门控(决策 ①)**:
   - `internal/service` 新增包级 `envSeam = false` + `SetEnvSeam(bool)`(非 nil 语义守卫);`runtime.go` 三个生效解析的 env 分支(engineScripted / agentCLI / issueSourceFor)加 `envSeam &&` 门 —— seam 关(产品)→ env 永不读取、纯 DB(company→global→默认)解析;seam 开(测试)→ 现行为不变(env 优先)。
   - `internal/endpoint/seal.go` 平行门控 `envKey()`(主密钥缺失兜底):seam 关 → 不再读 `OS_ENDPOINT_KEY`,报清晰错误(文案不含 env 名,指 `<db>.key` / `/setup`);seam 开 → 现行为不变。
   - `internal/service/delegate.go` `agentCLIFromEnv()`(产品死代码,生产已走 delegate.go:283 `s.agentCLI`)门控保留供测试直测,产品语义 = 缺省 claude。
   - **OS_SCRIPT_* 读取不动**(engine.go:128/144、planner.go:83、intake.go:290):它们全在 `engScripted*` 确定性桩函数内、仅 scripted 分支可达,而 scripted 的唯一 env 入口 = `engineScripted` 的 `OS_ENGINE_MODE`(已被 ① 门住)——门住 OS_ENGINE_MODE 后 OS_SCRIPT_* 产品路径结构性不可达;env 为空时本就有确定默认内容。只补注释说明(见 3.4)。
2. **两个 TestMain(决策 ① 配套,测试零改造)**:`internal/service/testmain_test.go`(开 service + endpoint 两 seam,因其 OS_ENDPOINT_KEY 用例经 endpoint env 兜底)、`internal/endpoint/testmain_test.go`(开 endpoint seam)。两包现均无 TestMain(核实),可安全新增。`internal/server` 无需 TestMain(经 `/setup` 注入真主密钥,零 env 依赖,见 §二 2.2)。
3. **server 业务 flag 删除(决策 ③)**:`internal/cli/server.go` 删 `--port/--poll/--digest/--queue-work/--queue-interval` 定义与 `effectiveServerConfig` 调用 → RunE 直接 `serverConfigFromApp(app)`(纯 app_setting → config + bounds);`--digest-now` 保留(一次性触发,非配置);`--db/--config` 保留(数据定位,root.go)。`internal/cli/serverconfig.go` 删 `flagVals`/`changed` 参数,函数改 `serverConfigFromApp(app settings.AppSetting) (serverConfig, error)`,零值填默认 + bounds 校验保留。
4. **残留文案 / help 清理(决策 ②)**:凡把 env 当配置写的注释与 help 去除或改写为「DB/Web 权威;env 仅测试 seam」——见 §三 3.3 逐条。
5. 测试 + 归档 + 进度总表 + design README 登记。

**不做(留给后续/live,防越界):**

- **CLI 领域写命令全保留**(company create / task create / decision add / memory add / endpoint add+select / repo add / capability·agent·workflow add / approval decide / queue work / intake sync):非 Phase 9 收编面(方向 §二.1 决策③收编面 = engine/agent/issue 源/通知/控制台/server 运行参数,全 env/flag,已由 9.1-9.3 + 本阶段 ①③ 收净),Web 未接管端点池/repo/公司登记,禁写即阉割。
- **provider ANTHROPIC_\* 不收编**:engine 活路径走 endpoint 解密 token(`provider.NewOpenAI(e.BaseURL, token, …)`,engine.go:81-89);`internal/provider/claudecli.go` 的 ANTHROPIC_\* 是「已解密凭据 → 子进程 env 传递」机制(Claude Code CLI 自身读 ANTHROPIC_\*),非 OS 配置通道,方向 §3.1 权威清单亦未收录。
- **`internal/server/setup.go` 的 `old_endpoint_key` JSON 字段**(D2 引导页 re-key 输入)不动:是 Web 页面输入,非进程 env 读取。
- **Web / API 零新面**;零迁移 / 零 sqlc / 零 go.mod 变更。
- **OS_ENDPOINT_KEY 历史 DB 迁移语义不动**:enc:v1 存量经 `/setup` D2 一次性 re-key(9.2 已立);本阶段只关产品 env 兜底通道。

## 二、代码现状核实(立足点,9.3 落地后 commit a84ecce)

### 2.1 产品 env 读取点(逐行核实,9.4 门控对象)

| # | 读取点 | env | 现状语义 | 9.4 处置 |
|---|---|---|---|---|
| 1 | `internal/service/runtime.go:21` `engineScripted` | `OS_ENGINE_MODE` | seam 分支前置:env==scripted 短接 true(压过 DB) | 加 `envSeam &&` 门;seam 关 → 纯 DB(company→global→live) |
| 2 | `internal/service/runtime.go:34` `agentCLI` | `OS_AGENT_CLI` | seam 分支前置:env 覆盖全 DB | 加 `envSeam &&` 门;seam 关 → company→global→claude |
| 3 | `internal/service/runtime.go:57` `issueSourceFor` | `OS_ISSUE_SOURCE` | seam 分支前置:非空 → `issueSourceFromEnv()` | 加 `envSeam &&` 门;seam 关 → company github/fixture |
| 4 | `internal/service/runtime.go:94-102` `issueSourceFromEnv` | `OS_FIXTURE_ISSUES` / `OS_GITHUB_TOKEN` | 仅被 3 的 seam 分支独占调用 | 随 3 门控闭环(seam 关不可达);函数保留 |
| 5 | `internal/service/delegate.go:49` `agentCLIFromEnv` | `OS_AGENT_CLI` | **产品死代码**(生产选型 = delegate.go:283 `s.agentCLI(ctx, t.CompanyID)`;此函数仅 delegate_test 直测) | 加 `envSeam &&` 门(测试零改);seam 关 = 缺省 claude |
| 6 | `internal/endpoint/seal.go:63` `envKey` | `OS_ENDPOINT_KEY` | 主密钥缺失兜底(enc:v1/测试);产品 `/setup` 后已注入 master,此路径产品不可达 | 加门 + 错误文案改「master key not loaded(<db>.key;run /setup)」 |
| 7 | `internal/service/engine.go:128,144`、`planner.go:83`、`intake.go:290` | `OS_SCRIPT_TEST/REVIEW/PLAN/TRIAGE` | 全在 `engScripted*` 确定性桩内,**仅 scripted 分支可达**;env 空 → 确定默认内容 | **不动**(scripted env 入口已被 1 门住 → 产品不可达);补注释 |
| — | `internal/notify/notify.go` | — | 已无 env 读取(9.3 删 `NewFromEnv`) | 不涉 |
| — | `internal/provider/claudecli.go:71` | `ANTHROPIC_*`(apiKeyEnv) | 子进程凭据传递,非 OS 配置(见 §一 不做) | 不涉 |

### 2.2 测试 env 依赖(门控改造面 → 决定 TestMain 放置;均为 package 级 `t.Setenv`)

| 包 | 依赖 env 的测试文件 | seam 需开 |
|---|---|---|
| `internal/service` | delegate_test(OS_AGENT_CLI/OS_ENGINE_MODE/OS_ENDPOINT_KEY/OS_SCRIPT_\*)、runtime_test(OS_AGENT_CLI/OS_ENGINE_MODE/OS_ISSUE_SOURCE/OS_FIXTURE_ISSUES)、planner_test(OS_SCRIPT_PLAN)、tier_test/driver_test(OS_ENGINE_MODE/OS_SCRIPT_\*)、settings_test(OS_ENDPOINT_KEY) | service + endpoint |
| `internal/endpoint` | seal_test(OS_ENDPOINT_KEY) | endpoint |
| `internal/server` | **无真 env 读取**(setup_test.go:144 仅是注释;用例用 `endpoint.SealTokenWith(显式 key)` 或经 `/setup` 注入 master;E4 全链注入) | 不需要 |

核实:两依赖包现均**无 `TestMain`** → 可各新增一个。仓库其余包测试均不设这些 env(逐包扫过)。

### 2.3 产品构造链 / CLI 写命令现状(决定 ①② 边界)

| # | 事实 | 位置 |
|---|---|---|
| 1 | 产品唯一 Service 构造点 = `NewRootCmd` 的 `PersistentPreRunE`:`svc = service.New(repository.NewStore(db))`,随后 LoadKey 注入 endpoint+settings holder | `internal/cli/root.go:50,54-60` |
| 2 | **cli 无任何 settings/secret 写方法调用**(grep `UpdateGlobalSettingsAs / UpdateCompanySettingsAs / SetCompanySecretAs / DeleteCompanySecretAs / ResetCompanySettingsAs / UpsertAppSetting / SetConsoleToken / RekeyAll / SetSecret` → 零命中);cli 也不直接持 repo 写表(仅 root.go 构造 store 传 service) | 全 `internal/cli/*.go` |
| 3 | CLI 写命令全为领域/执行对象:company create、task create/run、queue work、approval decide、intake sync、decision add、memory add、endpoint add/select、repo add、capability/agent/workflow add、workflow run | `internal/cli/*.go`(Use 表,见 §一 不做) |
| 4 | `os server` deprecated 业务 flag 5 个 + `effectiveServerConfig(app, flagVals, Changed)`(Changed 覆盖分支) | `internal/cli/server.go:93-99`;`internal/cli/serverconfig.go:23-86` |
| 5 | `svc.AppSetting` 缺行回内置默认(`settings.Default*`:8787/5/10/09:00)→ serverconfig 零填为防御冗余 | `internal/service/settings.go`(9.1);`internal/settings` |
| 6 | `--digest-now` 是 boot 一次性触发(调 `srv.RunDigestNow`),非持久配置 | `internal/cli/server.go:72-77,97` |

## 三、契约设计

### 3.1 env seam 反向门控(决策 ①)

**`internal/service/seam.go`(新增,包级):**

```go
// envSeam 反向门控(9.4,契约 cli-readonly.md §3.1,决策①):默认 false = 产品语义 —— 产品进程
// (cmd/os 构造链,root.go PersistentPreRunE)恒不读 OS_* 配置 env,纯 DB(company→global→默认)解析;
// 仅 go test 在 TestMain 经 SetEnvSeam(true) 打开,保留既有确定性 seam(600+ 用例零改造,方向 D3)。
var envSeam = false

// SetEnvSeam 开关测试 seam 的 env 读取。非测试代码不得调用(产品恒关)。
func SetEnvSeam(on bool) { envSeam = on }
```

**门控改造点(seam 关时 env 分支直接跳过):**

```go
// runtime.go engineScripted —— env 分支收进 envSeam 门(D3 seam 语义仅测试存在)。
func (s *Service) engineScripted(ctx context.Context, companyID string) bool {
	if envSeam && strings.EqualFold(os.Getenv("OS_ENGINE_MODE"), "scripted") {
		return true
	}
	// …DB 生效(EngineModeFor)不变
}

// runtime.go agentCLI —— env 分支同收门;seam 关 = company→global→claude。
func (s *Service) agentCLI(ctx context.Context, companyID string) (string, error) {
	if envSeam {
		if v := strings.TrimSpace(os.Getenv("OS_AGENT_CLI")); v != "" {
			return strings.ToLower(v), nil
		}
	}
	// …company → global → 内置 claude 不变
}

// runtime.go issueSourceFor —— env seam 分支收门(OS_FIXTURE_ISSUES/OS_GITHUB_TOKEN 经 issueSourceFromEnv 随此闭环)。
func (s *Service) issueSourceFor(ctx context.Context, companyID string) (github.Source, error) {
	if envSeam {
		if v := strings.TrimSpace(os.Getenv("OS_ISSUE_SOURCE")); v != "" {
			return issueSourceFromEnv()
		}
	}
	// …company issue_source(github|fixture)不变
}

// delegate.go agentCLIFromEnv(产品死代码,仅测试直测)—— 同门。
func agentCLIFromEnv() string {
	if envSeam {
		if v := strings.TrimSpace(os.Getenv("OS_AGENT_CLI")); v != "" {
			return strings.ToLower(v)
		}
	}
	return agentCLIClaude
}
```

**`internal/endpoint/seal.go`(平行门控 + 文案):**

```go
// 包级:var envSeam = false + func SetEnvSeam(on bool)  —— 同 service 体例(seal.go 顶部,挨着 holder)。
func envKey() ([]byte, error) {
	if !envSeam {
		// 产品语义:OS_ENDPOINT_KEY 不再兜底(决策①/D2 迁移后作废);须经 /setup 注入主密钥(<db>.key)。
		return nil, fmt.Errorf("endpoint master key not loaded (<db>.key); run /setup to initialize or re-key")
	}
	// …测试 seam:读 OS_ENDPOINT_KEY 不变
}
```

> 说明:endpoint 的 `envSeam` 与 service 各自独立(两包互不 import);TestMain 各开各的。service TestMain 需同时开 endpoint seam(其 OS_ENDPOINT_KEY 用例走 endpoint envKey)。

**TestMain(决策 ① 配套,新增两文件,现有测试零改):**

```go
// internal/service/testmain_test.go
func TestMain(m *testing.M) {
	endpoint.SetEnvSeam(true) // service 用例经 endpoint envKey 造旧 key 端点(OS_ENDPOINT_KEY)
	SetEnvSeam(true)           // 本包 engineScripted/agentCLI/issueSourceFor seam
	os.Exit(m.Run())
}

// internal/endpoint/testmain_test.go
func TestMain(m *testing.M) {
	SetEnvSeam(true) // seal_test OS_ENDPOINT_KEY 用例
	os.Exit(m.Run())
}
```

### 3.2 server 业务 flag 删除(决策 ③)

**`internal/cli/serverconfig.go` 简化(删 flagVals/Changed;函数名改 `serverConfigFromApp`):**

```go
// serverConfigFromApp 由 DB app_setting 行求 os server 生效配置(9.4 决策③:业务 flag 全删,
// 权威源 = Web 设置页;--db/--config 数据定位在 root,--digest-now 一次性触发在 RunE)。
// svc.AppSetting 缺行已回内置默认(9.1),零值填默认保留为防御;范围防护不变。
func serverConfigFromApp(app settings.AppSetting) (serverConfig, error) {
	cfg := serverConfig{
		HTTPPort:         app.HTTPPort,         // 0 → DefaultHTTPPort
		PollMin:          app.PollMin,          // 0 → DefaultPollMin
		QueueWork:        app.QueueWork,
		QueueIntervalSec: app.QueueIntervalSec, // 0 → DefaultQueueIntervalSec
		Digest:           app.DigestTime,       // "" / "off" → 归一 ""
	}
	if cfg.HTTPPort == 0 { cfg.HTTPPort = settings.DefaultHTTPPort }
	if cfg.PollMin == 0 { cfg.PollMin = settings.DefaultPollMin }
	if cfg.QueueIntervalSec == 0 { cfg.QueueIntervalSec = settings.DefaultQueueIntervalSec }
	if cfg.Digest == "" || strings.EqualFold(cfg.Digest, "off") { cfg.Digest = "" }
	if cfg.HTTPPort < 1 || cfg.HTTPPort > 65535 { return serverConfig{}, fmt.Errorf("http_port must be 1..65535 (got %d)", cfg.HTTPPort) }
	if cfg.PollMin < 1 { return serverConfig{}, fmt.Errorf("poll_min must be >= 1 (got %d)", cfg.PollMin) }
	if cfg.QueueIntervalSec < 1 { return serverConfig{}, fmt.Errorf("queue_interval_sec must be >= 1 (got %d)", cfg.QueueIntervalSec) }
	return cfg, nil
}
```

**`internal/cli/server.go` RunE 改造:**

```go
// 头注更新:9.4(决策③)业务 flag 全删,os server 只剩 --db/--config(数据定位)+ --digest-now(触发)。
app, err := svc.AppSetting(cmd.Context())
if err != nil { return fmt.Errorf("read app settings: %w", err) }
cfg, err := serverConfigFromApp(app)
if err != nil { return err }
// …srv := server.New(svc, cfg.PollMin);SetDigestTime(cfg.Digest);SetMasterKeyPath(masterKeyPath);
//   cfg.QueueWork → SetQueueWork(interval);RunE 其余(信号/boot 日志/PollLoop/DigestLoop/QueueLoop/digest-now)不变
```

- 删 flag 定义 5 个(server.go:94-99)与 `var port, pollMin int; var digestFlag string; var queueWork bool; var queueIntervalSec int` 声明;`digestNow` 保留。
- flag help 的 `(deprecated: …)` 文案随 flag 一起消失,不再需要。
- boot 日志行(server.go:60-67)保留(读 DB 权威状态),仅删注释里 `OS_API_TOKEN 不再是…(9.4 CLI 收口删)` 中「9.4 收口删」的时间注(收口完成,注释改陈述句,见 3.3)。

### 3.3 残留文案 / help 清理(决策 ②;逐条,只改注释/help 文本,零语义)

| # | 位置 | 现状 | 改为 |
|---|---|---|---|
| 1 | `internal/cli/server.go:19` 头注 | 「flags 保留仅显式覆盖,标记 deprecated(9.4 CLI 收口删)」 | 「权威源 = Web 设置页;业务 flag 已删(9.4),os server 零业务参数」 |
| 2 | `internal/cli/server.go:49` 注释 | 「…(9.4 CLI 收口删)」 | 收口完成 → 删时间注,陈述「鉴权已 DB 化,console token 哈希读 app_setting」 |
| 3 | `internal/cli/endpoint.go:36` flag help | `"auth token (encrypted; needs OS_ENDPOINT_KEY)"` | `"auth token (encrypted; needs master key: run /setup to create <db>.key)"`(env 名从产品文案消失) |
| 4 | `internal/cli/intake.go:16` 注释 | 「用 OS_ISSUE_SOURCE=fixture + OS_FIXTURE_ISSUES 离线冒烟」 | 「离线冒烟经公司 issue_source=fixture + issue_fixture_path(Web/API 配),env 仅测试 seam」 |
| 5 | `internal/github/github.go:33`、`fixture.go:10` 注释 | 仍写「token 经 env OS_GITHUB_TOKEN 注入」/「OS_ISSUE_SOURCE=fixture…」 | 改陈述构造语义(NewClient(token) 收参;fixture 经 LoadFixture(path)),去 env 字样 |
| 6 | `internal/service/delegate.go:47,268` 注释 | 「agentCLIFromEnv…OS_AGENT_CLI」/「family=agentCLIFromEnv」 | 改「family=s.agentCLI(ctx, t.CompanyID)(DB 生效)」;agentCLIFromEnv 标注测试专用 |
| 7 | `internal/service/engine.go:47,53`、`task.go:53,55`、`planner.go:48,50`、`intake.go:221,223,268` 注释 | 「env 仅测试 seam」 | 统一「env 仅测试 seam(9.4:产品恒关,DB 权威)」——体例统一,不改逻辑 |
| 8 | `internal/endpoint/seal.go` 相关注释 | — | 头注补 envKey 产品关闭说明(随 3.1 代码) |
| 9 | 方向 §3.3/§三 2.1 所涉其余 `OS_` 字样(如 config 文案) | — | 见 §六 风险 3(方向/归档文档里 env 残留措辞逐一核对) |

### 3.4 边界裁定说明(写入头注的「不做」依据,供实施与 review 对照)

- **OS_SCRIPT_\* 不门控的原因**:scripted 入口唯一 = `engineScripted`(1 号门),产品 seam 关后 engineScripted 恒只读 DB 且 Web 只写 live → scripted 分支仅可能来自「手工改 DB engine_mode」的越权操作;即便该情形发生,OS_SCRIPT_* env 为空 → 确定默认内容(engRoleTest→pass、plan→direct、triage→direct_work),不产生配置面。补注释:「测试桩,产品经 OS_ENGINE_MODE 门不可达」。
- **领域写命令保留的原因**(决策 ②):配置收编面(engine/agent/issue/notify/console/server 参数)自始无独立 CLI 写命令(env/flag 渠道),9.1-9.3 + 本阶段 ①③ 收净后即「配置唯一写入口 = Web」成立;领域写命令操作的是业务对象(公司/任务/端点/仓库/能力/决策/记忆),Web 未接管且单算子需 CLI 高效录入 → 保留。**验收锚点从「写命令报『请用控制台』」修正为**:「无任何 CLI/env 渠道可写 Web 已收编配置 + seam-off 时产品读 env 恒空」(见 §五 F/G),诚实反映代码现实。

## 四、文件落地清单

| 动作 | 路径 | 内容 |
|---|---|---|
| 新增 | `internal/service/seam.go` | `envSeam` 包级 + `SetEnvSeam` + 头注(决策①体例) |
| 新增 | `internal/service/testmain_test.go` | TestMain:开 service + endpoint seam |
| 新增 | `internal/endpoint/testmain_test.go` | TestMain:开 endpoint seam |
| 改 | `internal/service/runtime.go` | engineScripted/agentCLI/issueSourceFor 的 env 分支收进 `envSeam` 门(§3.1);头注更新 |
| 改 | `internal/service/delegate.go` | agentCLIFromEnv 加门;:47/:268 注释改写 |
| 改 | `internal/endpoint/seal.go` | `envSeam` + `SetEnvSeam` + envKey 门控 + 文案(§3.1) |
| 改 | `internal/cli/serverconfig.go` | 删 flagVals/changed;`effectiveServerConfig` → `serverConfigFromApp`(零填默认 + bounds 保留) |
| 改 | `internal/cli/server.go` | 删 5 业务 flag 定义与声明;RunE 改 `serverConfigFromApp`;头注/注释更新(§3.2/3.3) |
| 改 | `internal/cli/endpoint.go` | `--token` help 去 OS_ENDPOINT_KEY(§3.3 #3) |
| 改 | `internal/cli/intake.go`、`internal/github/github.go`、`internal/github/fixture.go` | 注释清理(§3.3 #4/#5) |
| 改 | `internal/service/engine.go`、`task.go`、`planner.go`、`intake.go` | 既有「env 仅测试 seam」注释体例统一(§3.3 #7);OS_SCRIPT_* 补「产品不可达」注(3.4) |
| 改 | `internal/cli/serverconfig_test.go` | D 用例适配新签名(§五) |
| 新增 | `internal/service/seam_test.go`(或并入 runtime_test) | F1-F4 seam-off 产品语义 + seam-on 对照(§五) |
| 新增 | `internal/endpoint/seal_test.go` 追加 | F5 envKey seam-off(§五) |
| 改 | `docs/进度总表.md` / `docs/phase9/design/README.md` / 归档 stage | 登记(§七) |
| 新增 | `docs/phase9/stages/4.md` | 9.4 归档 |

## 五、用例清单(定稿后逐条落地;命名 F/D/G)

**Go service / endpoint(seam 门控语义):**
- **F1 `engineScripted` 产品语义(seam 关;`SetEnvSeam(false)` + `t.Setenv` + `t.Cleanup` 恢复 true)**:env `OS_ENGINE_MODE=scripted` 已设 → 仍走 DB:company scripted=true → true;company live=false 压过 global scripted;无 company 默认 live → false。(对照 seam-on 既有 A1 断言 env 短接,两态并存证明门控成立)
- **F2 `agentCLI` 产品语义**:env `OS_AGENT_CLI=codex` 已设 → 忽略;company codex > global codex;缺省 claude。
- **F3 `issueSourceFor` 产品语义**:env `OS_ISSUE_SOURCE=fixture`+`OS_FIXTURE_ISSUES=<path>` 已设 → 忽略;company github 无 secret → fail-closed 报 `no github_token`(非 fixture 路径错);company fixture+path → LoadFixture。
- **F4 `agentCLIFromEnv` 产品语义**:env 已设 → 仍返回 claude(缺省);seam-on 直测(既有 delegate_test 用例,经 TestMain 零改)。
- **F5 endpoint `envKey` 产品语义**(endpoint 包,`SetEnvSeam(false)` 局部):master nil + `OS_ENDPOINT_KEY` 已设 → `SealToken`/`OpenToken` 报「master key not loaded…」且错误**不含 `OS_ENDPOINT_KEY` 字样**;seam-on(seal_test 既有用例,经 endpoint TestMain)零改。
- **既有 seam-on 全量回归(零改)**:runtime_test A1-A3、delegate/planner/tier/driver 的 OS_ENGINE_MODE/OS_SCRIPT_\*/OS_AGENT_CLI 用例、settings_test/delegate_test 的 OS_ENDPOINT_KEY 用例 —— 经两个 TestMain 自动开 seam,逐条原样绿。
- **D1 `serverConfigFromApp`**:app 行生效(port 8790 / poll 3 / queue true / interval 30 / digest "off"→"" 归一;digest "08:30" 保留);空 app → 内置默认(8787/5/10,digest ""→关语义由 SetDigestTime 判,函数只归一)。(原 effectiveServerConfig 的 Changed 覆盖分支用例删除)
- **D2(并入 D1)`serverConfigFromApp` bounds**:显式 app 值 port 0/65536、poll 0、interval 0 → error(不再经 flag Changed 可达)。

**二进制冒烟(临时脚本,不入库;seam-off 行为权威在 F1-F5,binary 只做污染 env 回归):**
- **G1 污染 env 零参数冒烟**:scratch db;导出 `OS_ENGINE_MODE=scripted`、`OS_AGENT_CLI=codex`、`OS_ISSUE_SOURCE=fixture`(无效路径);`os init` → `/setup`(Web)建 console + master key → 建公司 → Web/API 设 agent_cli=claude、issue_source=github(或无 secret 验证 fail-closed 文案)→ **零参数 `os server` 按 DB 权威起**(端口 8787、digest/queue 日志状态照 app_setting;污染 env 被忽略,不因 OS_ISSUE_SOURCE=fixture 去读无效 fixture);`GET /settings` 断言 default live / agent claude。→ 9.3 smoke 回归子集:PUT /settings、公司机密、rotate-master-key 全经 Web 断言仍绿。
- **G2 CLI 只读/写面冒烟**:读命令族(`company list`、`task list`、`approval list`、`endpoint list`、`audit list`、`overview`)零 env 零参数跑通;**领域写命令仍可用**(`company create` 建一家 → `task create` → `approval list` 可见 → `approval decide`)——决策②边界实证;无 CLI/env 路径能改 Web 已收编配置(§二 2.3 已静态核实)。

**Web / 构建:**
- `go build ./...`、`go vet ./...`、`go test -count=1 ./...` 全绿;`go test -count=1 -race ./internal/service/ ./internal/endpoint/ ./internal/cli/ ./internal/server/` 全绿。
- 本阶段零 Web diff → 不跑 tsc/vite;`make ui` 不涉(无 assets 变更)。

## 六、风险与取舍

1. **反向默认的「未来新测试包」约定**:任何未来包若在测试里设这些 OS_* env 且驱动 service/endpoint 代码,须自加 TestMain(或调 service.SetEnvSeam(true)/endpoint.SetEnvSeam(true))——文档(seam.go 头注)写明;当前全仓仅 service/endpoint 两包依赖(核实)。
2. **F 用例须显式局部关 seam**:suite 默认(TestMain)开;F 用例内 `SetEnvSeam(false)` 必须 `t.Cleanup` 恢复 true,防污染同包后续用例——写 helper(`withSeamOff(t)`),§五 用例统一走它。
3. **方向/归档文档里的 env 措辞残留**:9.3 归档与 design 文档若干处仍以 env 描述离线冒烟姿势(如「OS_ISSUE_SOURCE=fixture 离线冒烟」);9.4 文档(§3.3 #9)逐处核对,产品侧姿势改为「公司 issue_source=fixture + path(Web/API 配)」。存量 stage 3 归档为历史记录不改(只读),设计 README 仅新增 9.4 行。
4. **`os server` 排障改参数姿势变化(决策③代价)**:不再能 `os server --port 9999` 临时覆盖;排障改 http_port/poll/queue 等 = Web 设置页 或 `--db` 指向后手工改 app_setting 值。方向 D4 本留「deprecated 排障覆盖」余地,本阶段按 9.3 注释与 9.4 行选定删除;若日后排障频发再回补 `--port`(届时单 flag,不进契约配置面)。
5. **endpoint envKey 关门的波及**:任何「未 /setup 就想用 CLI 建端点」的产品姿势改为报「master key not loaded,run /setup」——与 9.2 初始化引导一致(fail-closed);dev/scratch 库清空 token 经 Web 重录(D2)不受影响。
6. **OS_SCRIPT_* 维持现状的残留**:手工把 DB engine_mode 改 scripted 的产品进程仍会读 OS_SCRIPT_*(若设了)→ 确定性内容生效。此属越权手工操作,文档标注(3.4),不视为配置通道。
7. **`agentCLIFromEnv` 保留为死代码 + 门控**:为免改 delegate_test 直测;注释明确「仅测试直测,产品走 s.agentCLI」;若日后重排可并入 runtime.go 删除。

## 七、登记

- 设计索引 `docs/phase9/design/README.md` 增本契约行(定稿);实施完成归档 `docs/phase9/stages/4.md` 后状态改「已冻结,见 stage 4」。
- `docs/进度总表.md` Phase 9 行记 9.4 完成(收口后),并更新「下一子阶段」为 Phase 10(未立项)或「Phase 9 完结」。
