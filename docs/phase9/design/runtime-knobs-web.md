# Phase 9.3 — 运行时旋钮 Web 化 + 全量设置页 + 通知按公司(实施契约)

> Phase 9 子阶段实施契约,承接方向 [config-governance.md](config-governance.md)(定稿)§六 9.3 行 + §三 3.1 全量 env 旋钮清单 + §五 D1/D2/D3;9.1([settings-foundation.md](settings-foundation.md))/9.2([console-access.md](console-access.md),均已冻结)显式 defer 的「运行时消费点 DB 化 / server 零参数 / 通知按公司 / 全量设置页 / 换主密钥动作 / settings 写审计」在本阶段落地。
> 定稿门(2026-09-07,AskUserQuestion 三项决策):**① 摘要路由 = 按公司 fan-out 独立日报**——GET/定时 `SendDailyDigest` 对每个配了 `feishu_webhook` 机密的公司各自聚合**该公司数据**并独立推送,未配公司跳过;② **通知移除 env**——`OS_FEISHU_*` 不再读取,`service.notify` 单例/`SetNotifier`/`NotifyEnabled` 删除,事件点即时通知按任务归属公司机密路由;③ **engine_mode Web 仅 live**——设置页只出现 live(scripted 仅保留 env 测试 seam,运行期判据 = env==scripted 或 DB 生效值==scripted)。
> 代码现状立足点 = **9.2 落地后工作树**(migration 0012 三表 + settings 包 + repo/service settings + `/setup` 首启 + console 令牌哈希鉴权 + 主密钥 endpoint holder,已提交)。

## 一、范围与边界

**做(9.3 交付物):**

1. **运行时 env 消费点 DB 化**(11 旋钮逐行收编,方向 §3.1):engine_mode / agent_cli / issue_source+failure 路径 / github_token(secret)统一改走 company→global→default 生效解析,env 只作测试 seam(D3)。
2. **settings 包主密钥 holder**:与 endpoint holder 平行(`UseMasterKey`/`MasterKey`/`CurrentKeySource`),CLI root 与 `/setup` 两处注入;service 机密运行期读写经 holder(不解明文出 service)。
3. **通知按公司(决策 ②)**:删 env 注入(notify 包 `NewFromEnv` + `service.notify` 单例);`notifyCompany(ctx, companyID, text)` 用公司机密 `feishu_webhook`/`feishu_secret` 构造 notifier;审批即时通知路由到任务归属公司。
4. **摘要按公司 fan-out(决策 ①)**:`SendDailyDigest` 遍历公司,每个有 `feishu_webhook` 的公司聚合**本公司窗口数据**独立推送(approval 无 company 列 → 经本公司任务 id 集过滤)。
5. **server 零参数**:`os server` 默认从 DB(`app_setting`)读 `http_port/poll_min/queue_work/queue_interval_sec/digest_time` 起;flag 保留但标记 deprecated、仅显式传参时覆盖;删 `OS_FEISHU_DIGEST` 读取。
6. **settings 写审计 + 全量设置 API**:GET/PUT `/api/v1/settings`(global)、GET/PUT/DELETE `/companies/{id}/settings`(company 覆盖)、GET/PUT/DELETE `/companies/{id}/secrets/{secretID}`(白名单 github_token/feishu_webhook/feishu_secret,audit 只记掩码)、POST `/api/v1/settings/rotate-master-key`(端点 token + 公司机密双 re-key,新主密钥仅此一次返回)。
7. **Web 全量设置页**:保留「控制台访问」卡,新增 全局默认 / server 参数 / 公司覆盖+机密(随当前选择公司)/ 更换主密钥(re-key 动作,确认弹窗 + 结果一次展示)卡。
8. 测试 + 归档 + 进度总表登记。

**不做(留给后续/live,防越界):**

- **CLI 写命令收口 / env 读取残余删除 / flag 彻底下线**:9.4(本阶段 server flag 只标 deprecated,保留覆盖能力)。
- **多管理员/RBAC、会话过期、secret 历史审计值**:方向既定不做。
- **server 参数热加载**:boot 读一次,重启生效(单算子常驻,方向 §3.2 未要求热更)。
- **`/setup` 旧 `OS_ENDPOINT_KEY` re-key 语义改动**:仍走 9.2 既有 `RekeyEndpointTokens`(首启时 secret 表恒空)。
- **新增迁移 / sqlc**:零迁移、零 sqlc 重生成(repo 查询面 9.1 已齐:ListSecrets/DeleteSecret 等俱在)。

## 二、代码现状核实(立足点,9.2 落地后工作树)

### 2.1 运行时 env 消费点(逐行核实,全部 `os.Getenv` 在 internal 非测试代码)

| # | 事实 | 位置 |
|---|---|---|
| 1 | 模式分界 `OS_ENGINE_MODE`:**engCall** live/scripted 首行判定 | `internal/service/engine.go:48` |
| 2 | 模式分界:`isScriptedEngine()` 包函数(唯一实现 `tier.go:25`;唯一调用 `task.go:54` `if p.ToolName=="engineering" && !isScriptedEngine()`) | `internal/service/tier.go:25`、`task.go:54` |
| 3 | 模式分界:**delegateBaseline** scripted 跳过首行 | `internal/service/delegate.go:326` |
| 4 | 模式分界:**planCall** scripted 首行 | `internal/service/planner.go:49` |
| 5 | 模式分界:**triageIssue** scripted 首行 + **intakeMode()**(audit actor `"intake:"+intakeMode()`) | `internal/service/intake.go:213`、`:277`、`:170` |
| 6 | agent_cli 旋钮:`agentCLIFromEnv()`(OS_AGENT_CLI,缺省 claude);delegateWriter `family := agentCLIFromEnv()` | `internal/service/delegate.go:49`、`:282` |
| 7 | issue_source 旋钮:`SyncRepos` 首行 `src, err := issueSource()` **单源全局一次**;`issueSource()`:fixture→OS_FIXTURE_ISSUES、github→OS_GITHUB_TOKEN(fail-closed) | `internal/service/intake.go:59`、`:256-275` |
| 8 | github token:`github.NewClient(token)`;osrepo.Repo 带 `CompanyID` | `internal/github/github.go:33`;`internal/repo/model.go` |
| 9 | notify env:`notify.NewFromEnv()` 读 OS_FEISHU_WEBHOOK/SECRET;`service.Service{... notify *notify.Notifier}` + `SetNotifier` + `NotifyEnabled` | `internal/notify/notify.go:44-55`;`internal/service/service.go:16,27,30` |
| 10 | 审批即时通知(唯一事件点):`approval.go:59 notifyApproval(ctx,t,id,reason)` → `service/notify.go` `s.NotifyEnabled()` + `s.notify.PostText` | `internal/service/approval.go:59`;`internal/service/notify.go:23-27` |
| 11 | 摘要:`SendDailyDigest` `!s.NotifyEnabled()` 跳过 → `s.notify.PostText`;`gatherDigest` 全公司聚合;approval 只带 TaskID **无 company 列** | `internal/service/digest.go:37-53`;`internal/approval/model.go` |
| 12 | server digest env 读取:`--digest > OS_FEISHU_DIGEST > 09:00` 优先级 | `internal/cli/server.go:29-32` |
| 13 | server flags:`--port 8787 --poll 5 --queue-interval 10 --queue-work`;boot 日志 `feishu on (OS_FEISHU_WEBHOOK)`;`svc.NotifyEnabled()` 判定 | `internal/cli/server.go` 全(flags 定义 + RunE) |
| 14 | CLI root 注入:`notify.NewFromEnv()` → `SetNotifier`;`LoadKey(masterKeyPath)` → 仅 `endpoint.UseMasterKey(kb)`(settings holder 尚未设) | `internal/cli/root.go:52-53,61-66` |
| 15 | `/setup`:GenerateKey → 可选旧 key `RekeyEndpointTokens` → `SaveKey` → `endpoint.UseMasterKey`;settings holder 未设 | `internal/server/setup.go` |

### 2.2 可复用的既有能力(9.1/9.2 已立,不改接口)

| # | 事实 | 位置 |
|---|---|---|
| 16 | service 设置读:`AppSetting`(缺行→Default)、`UpsertAppSetting`(整行)、`CompanySetting(ctx,id)(ok)`、`UpsertCompanySetting`、`DeleteCompanySetting`、`EngineModeFor`(company→global→"live")、`SetSecret(ctx,co,id,plain,key)`、`OpenSecret(ctx,co,id,key)(plain,ok,err)`、`RekeyEndpointTokens`(两遍零写坏)、`ConsoleTokenHash`/`SetConsoleTokenAs` | `internal/service/settings.go` |
| 17 | repo 读面齐:Get/List/DeleteSecret、ListTasks(companyID,…)、ListDecisions(companyID,kind)、ListIssueSync(companyID)、ListCompanies、HasAuditAction | `internal/storage/repository/settings.go`、`task.go`、`decision.go`、`repo.go`、`company.go`、`audit.go` |
| 18 | 主密钥文件模块:KeyPath/GenerateKey/SaveKey/LoadKey(0600,64hex);settings 包 seal:SealSecret/OpenSecret(enc:v2) | `internal/settings/keyfile.go`;`internal/settings/seal.go` |
| 19 | notify 测试直构 `&Notifier{webhook: "file://"+path}`(离线邮箱);`notify_test.go:119-122` 测 `NewFromEnv` 空 webhook→nil(本阶段按决策②改写/删除) | `internal/notify/notify_test.go` |
| 20 | server:`Server{svc,poll,digestH/M,masterKeyPath,queueInterval}`;`New(svc,pollMin)`;`SetDigestTime("HH:MM"/"off")`;`SetMasterKeyPath`;`DigestLoop/RunDigestNow`;`/api/v1` 挂 `registerAPIRoutes`(现有路由无 /settings GET、/companies/{id}/settings、/secrets、rotate) | `internal/server/server.go`;`internal/server/api.go:70-109` |
| 21 | `/api/v1` 写操作审计 actor 常量 `human:console`;信封/`apiOK/apiErr/decodeJSON/pathParam` helper;404/409/400 语义与 9.2 一致 | `internal/server/api.go`、`setup.go` |
| 22 | Web:设置页现状 = 仅「控制台访问」卡 + 主密钥信息卡;`AppContext` 提供 `companyId/selectCompany`;endpoints.ts 现有 helper 风格(setupStatus/rotateConsoleToken 等) | `web/src/pages/Settings.tsx`;`web/src/store/AppContext.tsx`;`web/src/api/endpoints.ts` |

## 三、契约设计

### 3.1 运行时生效解析(service 新增一组方法;env 只作测试 seam)

新增 `internal/service/runtime.go`(包级注释说明 D3 seam 与调用点):

```go
// engineScripted 生效判定:D3 seam 优先(env==scripted,离线测试线)→ 否则 DB 生效值
// (EngineModeFor:company 覆盖 → global 默认 → 内置 live)。Web 只写 live,scripted 只可能来自 env/手工 DB。
func (s *Service) engineScripted(ctx context.Context, companyID string) bool {
	if strings.EqualFold(os.Getenv("OS_ENGINE_MODE"), "scripted") { return true }
	m, err := s.EngineModeFor(ctx, companyID)
	if err != nil { return false } // 解析失败按 live 处理(fail-closed 端点语义在各自调用点,与现状一致)
	return m == "scripted"
}

// agentCLI 生效委派族:env seam(OS_AGENT_CLI,agentCLIFromEnv 原样)→ company.agent_cli →
// global.agent_cli_default → "claude"。
func (s *Service) agentCLI(ctx context.Context, companyID string) (string, error)

// issueSourceFor 按公司解析通道 B issue 源:company.issue_source(fixture|github,缺省 github)→
// github = 需 secret github_token(fail-closed,缺省清晰报错);fixture = 需 company.issue_fixture_path。
// env seam:OS_ISSUE_SOURCE 非空 → 原 env 分支(离线冒烟,OS_FIXTURE_ISSUES/OS_GITHUB_TOKEN 语义不变)。
func (s *Service) issueSourceFor(ctx context.Context, companyID string) (github.Source, error)
```

**消费点逐个替换(env 读取从 internal 业务路径消失):**

| 调用点(现状) | 替换为 |
|---|---|
| `engine.go:48` engCall `os.Getenv(OS_ENGINE_MODE)` | `if s.engineScripted(ctx, t.CompanyID)` |
| `tier.go:25` `isScriptedEngine()`(删包函数)→ `task.go:54` | `s.engineScripted(ctx, p.CompanyID)`(createTask 已是 s 方法) |
| `delegate.go:326` delegateBaseline | `s.engineScripted(ctx, t.CompanyID)` |
| `planner.go:49` planCall | `s.engineScripted(ctx, t.CompanyID)` |
| `intake.go:213` triageIssue | `s.engineScripted(ctx, companyID)` |
| `intake.go:277` `intakeMode()`(删)→ `intake.go:170` audit | actor = `"intake:live"`/`"intake:scripted"` 由 `s.engineScripted(ctx, r.CompanyID)` 推导(IntakeIssues 已持 `r.CompanyID`) |
| `delegate.go:282` `agentCLIFromEnv()` | `s.agentCLI(ctx, t.CompanyID)`(delegateWriter 收 ctx+t;合法性命 delegatorFor 判,错误文案去 OS_AGENT_CLI 字样改通用) |
| `intake.go:59` `SyncRepos` 单源 `issueSource()` | 循环内按仓库归属公司 `s.issueSourceFor(ctx, r.CompanyID)`,同公司源缓存(map[string]github.Source);单仓库失败不中断语义保留 |

保留 env seam 原语:`agentCLIFromEnv()`(delegate_test.go:595 直测 + 3.1 agentCLI 方法内部作 env 分支);删 `isScriptedEngine`/`intakeMode` 包函数(tier.go 内该函数无其他调用)。

### 3.2 settings 包主密钥 holder + 机密运行期读写

**keyfile.go 追加**(与 endpoint/seal.go holder 平行,同注释体例):

```go
// masterKey 进程级主密钥(settings 机密解密/加密用;CLI root 与 /setup 开库后注入;nil = 未注入)。
var (
	masterKey   []byte
	masterKeyMu sync.RWMutex
)
func UseMasterKey(key []byte)                 // nil = 清除;非 nil 拷贝存(防调用方复用缓冲)
func MasterKey() ([]byte, bool)               // 当前 key + 是否已注入(service 机密读写 + rotate 用)
func CurrentKeySource() string                // "master-key" | "none"(日志/测试)
```

**注入点两处补齐**(与 endpoint holder 同 site):
- `internal/cli/root.go` LoadKey 成功后:`settings.UseMasterKey(kb)`(并列现有 endpoint.UseMasterKey)。
- `internal/server/setup.go` SaveKey 后:`settings.UseMasterKey(newKey)`。

**service 机密方法(持 key 版本保留供测试;新增 Current 族走 holder):**

```go
const (
	SecretGitHubToken   = "github_token"   // 通道 B issue 源(GitHub REST)
	SecretFeishuWebhook = "feishu_webhook" // 通知/摘要 sink
	SecretFeishuSecret  = "feishu_secret"  // 飞书加签(可选)
)
// KnownSecretIDs 白名单(API set/delete 只收这些;审计只记 id+掩码,不记明文)。
var KnownSecretIDs = []string{SecretGitHubToken, SecretFeishuWebhook, SecretFeishuSecret}

func (s *Service) SetSecretCurrent(ctx, companyID, id, plain string) error // key = MasterKey();未注入 → error
func (s *Service) OpenSecretCurrent(ctx, companyID, id string) (string, bool, error) // 同上,key 内部取
func (s *Service) DeleteSecret(ctx, companyID, id string) error                     // wrap store
func (s *Service) RekeySecrets(ctx, oldKey, newKey []byte) (int, error)             // 两遍零写坏(collect→write),enc:v2
```

- SetSecret/OpenSecret(带 key 参数)签名不动(存量单测用);Current 族内部取 `settings.MasterKey()` 后复用同一路径。
- 换主密钥要同时 re-key 端点 token 与 secret 两表、且希望「先全量校验、任一失败零写」→ 新增 `RekeyAll(ctx, oldKey, newKey) (epN, secN int, err error)`:先遍历端点(collect+解密校验)再遍历 secret(collect+解密校验),全部校验通过后才开始写端点在先、secrets 在后(校验错误 → 零写;写阶段错误仍按两遍语义尽量收口,残余风险见 §六)。`/setup` 的 `RekeyEndpointTokens` 保留不动(首启时 secret 表恒空,无需改)。

### 3.3 通知按公司(决策 ②:删 env,只走公司机密)

- **notify 包**:新增 `func New(webhook, secret string) *Notifier`(webhook 空 → 返回 nil,与 nil 安全语义一致);`NewFromEnv` 删除、`os` import 移除;`Enabled/PostText/writeMailbox/postFeishu` 不动。`notify_test.go:119-122` 的 `NewFromEnv` 空 webhook→nil 用例改写为 `New("", "") == nil`。
- **service.go**:删 `notify` 字段、`SetNotifier`、`NotifyEnabled`、notify import(所有调用点见 2.1 #9-#11)。
- **`notifyCompany(ctx, companyID, text) error`**(new,放 service/notify.go):companyID 空 → nil(无归属任务无通道);`OpenSecretCurrent(feishu_webhook)` 未配 → nil + log skip;配了 → `notify.New(webhook, feishu_secret 可选).PostText(ctx, text)`。
- **`notifyApproval(ctx, t, approvalID, reason)`** 改走 `s.notifyCompany(ctx, t.CompanyID, ...)`(best-effort 语义不变)。

### 3.4 摘要按公司 fan-out(决策 ①)

```go
// SendDailyDigest 为每个配了 feishu_webhook 机密的公司各发一份独立日报(仅该公司数据)。
// 返回聚合错误前先尽量发完(单公司失败记日志不中断);无任何公司配 webhook → log 跳过返回 nil。
func (s *Service) SendDailyDigest(ctx context.Context) error {
	companies := s.store.ListCompanies(ctx)
	reported := 0
	for _, c := range companies {
		wh, ok, err := s.OpenSecretCurrent(ctx, c.ID, SecretFeishuWebhook); if err != nil { return err }
		if !ok { log.Printf("daily digest: company %s has no feishu webhook secret, skipped", short8(c.ID)); continue }
		st, err := s.gatherDigestFor(ctx, c.ID, windowStart, windowEnd)
		if err != nil { log...; continue }
		sec, _, _ := s.OpenSecretCurrent(ctx, c.ID, SecretFeishuSecret)
		if err := notify.New(wh, sec).PostText(ctx, formatDigest(st)); err != nil { log...; continue }
		reported++
	}
	log.Printf("daily digest sent to %d company/ies", reported)
	return nil
}
```

- `gatherDigestFor(ctx, companyID, start, end)`:改造现 `gatherDigest`(原逻辑 9.2 前「全公司一锅」,现按公司):
  - tasks = `ListTasks(companyID, …)` → 计数 + 建**本公司任务 id 集**(approval 无 company 列,过滤必经 task 集)。
  - approvals = `ListApprovals("")` 过滤:decided 在窗口 **且** TaskID ∈ 本公司任务集。
  - decisions = `ListDecisions(companyID, "")`;issue 账本 = `ListIssueSync(companyID)`。
  - `st.Companies = 1; st.CompanyID = companyID`(formatDigest 尾部 `os overview --company <id>` 提示保持)。
  - digestStats/formatDigest 结构不动(formatDigest 纯函数 + 既有单测零适配)。
- digest 时刻仍全局单 tick(`app_setting.digest_time`);窗口/标题不变。

### 3.5 server 零参数(决策 §3.2 落地)

**`internal/cli/serverconfig.go`**(纯函数,无 cobra 依赖,可单测):

```go
// serverConfig 是 os server 生效配置:DB app_setting 为默认,显式 flag 覆盖。
type serverConfig struct {
	HTTPPort, PollMin, QueueIntervalSec int
	QueueWork                           bool
	Digest                              string // "off" 或 "" = 关;否则 "HH:MM"
}
// flagsChanged(name) 由 cmd.Flags().Changed 提供;flag 值未显式传时用零值。
func effectiveServerConfig(app settings.AppSetting, f flagVals, changed func(name string) bool) serverConfig
```

规则:
- `http_port`:DB `app.HTTPPort`(缺省 8787);`--port` Changed → flag。
- `poll_min`:DB(缺省 5);`--poll` Changed → flag。
- `queue_work`:DB `app.QueueWork`(缺省 false);`--queue-work` Changed → flag。
- `queue_interval_sec`:DB(缺省 10);`--queue-interval` Changed → flag。
- `digest_time`:DB(缺省 09:00);`--digest` Changed **且 flag 非空** → flag(`"off"`=关);DB 值为空/`"off"` → 关。
- 范围防护:port∈[1,65535]、poll≥1、interval≥1、digest 交给 `SetDigestTime` 判(`"HH:MM"`/off);越界 → error(boot 清晰报错不静默)。

**`internal/cli/server.go` RunE 改造**:
- `app, err := svc.AppSetting(ctx)`(缺行即内置默认 → 未配置 DB 时与旧 flag 默认完全一致,存量零参数行为不变)。
- 求 `effectiveServerConfig` → `port/pollMin/queueInterval/digest`;queue_work 开时 `SetQueueWork(interval)`。
- digest:`SetDigestTime(cfg.Digest)`(off/空 = 关)。**删 OS_FEISHU_DIGEST 读取**(digestFlag 保留,仅 Changed 覆盖)。
- boot 日志:删 `feishu := "on (OS_FEISHU_WEBHOOK)"` + `svc.NotifyEnabled()` 判定,改打 `feishu notify: per-company (company secret feishu_webhook)`,并保留 digest/api-auth/queue 行。
- flag help 追加 `(deprecated: 已 DB 化,Web 设置页为权威源;显式传此 flag 仅作覆盖)`(`--port/--poll/--digest/--queue-work/--queue-interval`)。

### 3.6 Settings / company 设置 / secrets / rotate API

路由(加在 `registerAPIRoutes` 现有 `/settings/console-token` 旁):

```
GET    /api/v1/settings                       → global 生效行(AppSetting 缺行默认),输出含 console_token_set(bool,掩码),无 hash
PUT    /api/v1/settings                       → 部分更新(字段可选),见下;audit settings.update
POST   /api/v1/settings/rotate-master-key     → 换主密钥(端点 token + 公司机密双 re-key);新 master_key 仅此一次;audit settings.rotate_master_key
GET    /api/v1/companies/{id}/settings        → company 覆盖行(指针字段;无行 → 全 null + ok)
PUT    /api/v1/companies/{id}/settings        → 部分覆盖(null 字段 = 清该维度覆盖继承 global);audit company_settings.update
DELETE /api/v1/companies/{id}/settings        → 重置整行继承;audit company_settings.reset
GET    /api/v1/companies/{id}/secrets         → 白名单×set/updated_at(值永不出,只掩码);audit 不记
PUT    /api/v1/companies/{id}/secrets/{secretID} → body {value};白名单校验;SetSecretCurrent;audit secret.set(detail 只 id+set)
DELETE /api/v1/companies/{id}/secrets/{secretID} → 白名单校验;DeleteSecret;audit secret.delete
```

**校验规则(service 层,API 复用):**
- global PUT:engine_mode_default 只收 `"live"`(scripted → 400,决策③);agent_cli_default ∈ {claude,codex};digest_time = ""/`"HH:MM"`(parse);http_port∈[1,65535];poll_min/queue_interval_sec≥1。空 patch → 400。
- **必须保留 `console_token_hash`**:handler 先读当前 `AppSetting` 取 hash,合并 patch 后整行 Upsert(9.1 Upsert = 全列覆写,漏传即清空 hash 致鉴权回落开放——红线,写入专门 guard 测试)。
- global 生效行即 `s.AppSetting`;DB 无行时 PUT 生成首行(含默认列 + hash 空 → 不改变初始化态)。
- company PUT:engine_mode 只收 `"live"`/null(scripted → 400);agent_cli ∈ {claude,codex}/null;issue_source ∈ {github,fixture}/null;**fixture 必配 issue_fixture_path**(同 patch 或已存在);issue_fixture_path 可独立设;至少一个字段非空否则 400。全 null patch → 400(要清空走 DELETE)。
- secrets:secretID ∈ KnownSecretIDs;value trim 非空;SetSecretCurrent 需主密钥已注入(未注入 → 500「master key not loaded」)。

**rotate-master-key handler(放 `internal/server/settings_api.go`,需 masterKeyPath,与 setup.go 同文件族):**
1. `Initialized` 否 → 400(先 /setup)。
2. `oldKey, ok := settings.MasterKey()`;!ok → 400(进程未注入主密钥)。
3. `newHex := settings.GenerateKey(); newKey, _ := hex.DecodeString(newHex)`。
4. `epN, secN, err := s.svc.RekeyAll(ctx, oldKey, newKey)`;err → 4xx(bad old key / 任一行不可解 → 已零写)。
5. `settings.SaveKey(s.masterKeyPath, newHex)`(0600;masterKeyPath 空 → 500,先于写校验)。
6. `endpoint.UseMasterKey(newKey); settings.UseMasterKey(newKey)`。
7. audit;返回 `{master_key: newHex, endpoints_rekeyed: epN, secrets_rekeyed: secN}`。

**service 新方法(带 actor 写审计,放 service/settings.go):**
```go
func (s *Service) UpdateGlobalSettingsAs(ctx, patch settings.AppSettingPatch, actor string) (settings.AppSetting, error)
func (s *Service) UpdateCompanySettingsAs(ctx, companyID string, patch settings.CompanySettingPatch, actor string) (settings.CompanySetting, error)
func (s *Service) ResetCompanySettingsAs(ctx, companyID, actor string) error
func (s *Service) SetCompanySecretAs(ctx, companyID, id, value, actor string) error
func (s *Service) DeleteCompanySecretAs(ctx, companyID, id, actor string) error
func (s *Service) SecretMeta(ctx, companyID string) ([]SecretMeta, error) // [{id,set,updated_at}],白名单顺序
```
patch 类型用指针(DTO 对齐 Web JSON 部分可选):`settings.AppSettingPatch{EngineModeDefault *string; AgentCLIDefault *string; DigestTime *string; HTTPPort *int; PollMin *int; QueueWork *bool; QueueIntervalSec *int}`、`settings.CompanySettingPatch{EngineMode *string; AgentCLI *string; IssueSource *string; IssueFixturePath *string}`(放在 settings 包,json tag 照 3.6 输出形状)。audit entity:settings/company_settings/secret。

### 3.7 Web 全量设置页 + re-key 动作

`web/src/pages/Settings.tsx` 重构(保留现壳结构 + PageHead + useData 轮询模式),新增卡(自上而下):

1. **控制台访问**(现有,不动)。
2. **全局默认 / 引擎**:`engine_mode_default` 只读展示 `live`(Tag + 说明「Web 仅 live;scripted 是 CLI/env 离线测试线」,决策③);`agent_cli_default` Select(claude/codex)。→ PUT /settings。
3. **Server 参数**:http_port(InputNumber)、poll_min、queue_interval_sec、queue_work(Switch)、digest_time(Input,占位 09:00;空/off=关)+ 说明「重启 os server 生效」。→ PUT /settings。
4. **公司覆盖(作用于当前选择公司)**(companyId 变化重拉;无公司时禁用):agent_cli(Select + 清空=继承)、issue_source(Select: 继承默认 / GitHub / Fixture 需路径)、issue_fixture_path(Input);另设「重置公司设置(全继承)」危险按钮。→ GET/PUT/DELETE /companies/{id}/settings。公司引擎模式只读提示 live(随全局)。
5. **公司机密(当前公司)**:KnownSecretIDs 三行卡片 —— github_token(GitHub 通道 B 用)、feishu_webhook / feishu_secret(通知/摘要用);每行状态 Tag(未设置/已设置 · HH:MM 更新于)+ 「设置/更换」(Modal 输 value,Input.Password)+ 「移除」。→ GET/PUT/DELETE /companies/{id}/secrets。
6. **更换主密钥**:危险区卡——说明(re-key 全部端点 token + 公司机密;服务端无取回通道,成功后新密钥仅展示一次,`<db>.key` 0600 覆盖)+ Modal.confirm(输入确认短语或二次确认)→ POST /settings/rotate-master-key → 结果 Modal 展示新 master_key(一次)与 re-key 计数。

数据与 API:`web/src/api/endpoints.ts` 加 getGlobalSettings/updateGlobalSettings/rotateMasterKey/getCompanySettings/updateCompanySettings/resetCompanySettings/listSecrets/setSecret/deleteSecret(fn 入参带 companyId/secretID 走既有风格)。页面写操作后刷新对应 useData + message.success/error;401 走既有 AuthModal。引擎只读字段不进 PUT 请求。

## 四、文件落地清单

| 动作 | 路径 | 内容 |
|---|---|---|
| 新增 | `internal/service/runtime.go` | 3.1 engineScripted/agentCLI/issueSourceFor + 头注 |
| 改 | `internal/service/engine.go` | :48 换 engineScripted |
| 改 | `internal/service/tier.go` | 删 isScriptedEngine 包函数 |
| 改 | `internal/service/task.go` | :54 换 s.engineScripted(ctx, p.CompanyID) |
| 改 | `internal/service/delegate.go` | :326 换 engineScripted;:282 换 s.agentCLI;错误文案去 env 字样 |
| 改 | `internal/service/planner.go` | :49 换 engineScripted |
| 改 | `internal/service/intake.go` | :213/:170/:277 换 engineScripted 推导 actor;:59 改 per-company source(缓存);删 issueSource()/intakeMode() |
| 改 | `internal/service/notify.go` | notifyApproval → notifyCompany |
| 新增 | `internal/service/notify.go`(追加) | notifyCompany 实现 |
| 改 | `internal/service/digest.go` | SendDailyDigest 按公司 fan-out;gatherDigest → gatherDigestFor(companyID) |
| 改 | `internal/service/service.go` | 删 notify 字段/SetNotifier/NotifyEnabled/import |
| 改 | `internal/service/settings.go` | 3.2 secret Current/Delete/RekeySecrets/RekeyAll + 3.6 As 审计方法 + SecretMeta + 白名单 const;patch 类型(settings 包或 service 内定义,见 3.6) |
| 改 | `internal/notify/notify.go` | 加 New(webhook,secret);删 NewFromEnv/os import |
| 改 | `internal/settings/keyfile.go` | 3.2 holder UseMasterKey/MasterKey/CurrentKeySource |
| 改 | `internal/cli/root.go` | LoadKey 后补 settings.UseMasterKey(kb) |
| 改 | `internal/server/setup.go` | SaveKey 后补 settings.UseMasterKey |
| 新增 | `internal/server/settings_api.go` | 3.6 全部 handler + rotate-master-key |
| 改 | `internal/server/api.go` | registerAPIRoutes 注册新路由 |
| 新增 | `internal/cli/serverconfig.go` | 3.5 effectiveServerConfig + flagVals |
| 改 | `internal/cli/server.go` | RunE 走 DB 生效配置;删 OS_FEISHU_DIGEST;flag help deprecated;boot 日志改 |
| 改 | `internal/service/digest_test.go`、`internal/notify/notify_test.go`、相关 service 测试 | 按决策②/零参数适配 |
| 新增 | `internal/service/runtime_test.go`、`settings_secret_test.go`(或并入 settings_test.go) | A/B/C 用例(见 §五) |
| 新增 | `internal/server/settings_api_test.go` | E 用例 |
| 新增 | `internal/cli/serverconfig_test.go` | D 用例 |
| 改 | `internal/github/*`(如需) | NewClient 消费侧只换调用点,不改包 |
| 改 | `web/src/api/endpoints.ts` | 3.7 新 API helper + 类型 |
| 改 | `web/src/pages/Settings.tsx` | 3.7 全量设置页(卡 1-6) |
| 改 | `docs/进度总表.md` / `docs/phase9/design/README.md` / 归档 stage | 登记(§七) |
| 新增 | `docs/phase9/stages/3.md` | 9.3 归档 |

## 五、用例清单(定稿后逐条落地,命名 A/B/C/D/E)

**Go service:**
- A1 `engineScripted`:env scripted=true(seam);company 覆盖 scripted=true;global 默认 scripted=true;缺省(无 company/无行)=false;DB 解析错误=false。
- A2 `agentCLI`:env codex 覆盖(seam);company 覆盖优先 global;global 默认;缺省 claude。
- A3 `issueSourceFor`:company github + secret github_token → 构造成功;fixture + issue_fixture_path → LoadFixture;缺省 github 无 token → fail-closed 报错;env seam(OS_ISSUE_SOURCE=fixture+OS_FIXTURE_ISSUES,无 company 行)保持旧行为。
- B1 审批即时通知:task 归属公司配 feishu_webhook(file:// 邮箱)→ 邮箱出现告警文本;未配公司 → 静默无错误;task 无公司 → 静默。
- B2 摘要 fan-out:公司 A/B 各配 feishu(file:// 两邮箱 或 一邮箱两公司顺序)→ A 报告只含 A 任务/审批/决策计数、B 只含 B;公司 C 未配 → 跳过;SendDailyDigest 返回 nil。
- C1 secret Current 族:settings.UseMasterKey 后 SetSecretCurrent/OpenSecretCurrent roundtrip;未注入 key → 报错。
- C2 `RekeyAll`:old 解 endpoints+secrets 全成功 → 换新后新解成功旧解失败、计数正确;oldKey 错误 → 零写(endpoints token 与 secret 均仍可被旧解、不可被错解)。
- C3 白名单:set/delete 未知 secretID 拒绝;audit detail 无明文。
- D1 `effectiveServerConfig`:DB 行值生效;port/poll/queue/digest 各自 Changed 覆盖;DB digest off → 关;flag `--digest off` → 关。
- D2(可并入 D1)越界 port → error。
- 适配:原 digest skip(未配 webhook)→ 新 SendDailyDigest 无公司配 webhook = log skip nil;notify_test NewFromEnv 用例改写;`gatherDigest` 单测改 `gatherDigestFor`。

**Go API(httptest):**
- E1 GET /settings 默认;PUT 部分更新生效;PUT engine_mode_default=scripted → 400;**初始化(设 console hash)后 PUT /settings(不带 hash 字段)→ console 令牌仍有效**(hash 保留 guard)。
- E2 company settings PUT null=继承/DELETE 重置;fixture 无 path → 400;空 patch → 400。
- E3 secrets set/list/delete:list 只出掩码{set,updated_at}无明文;DELETE 后 set=false。
- E4 rotate-master-key(temp masterKeyPath + 注入 old holder):成功后新 key 可解 endpoint token 与 secret、旧 key 不可解;key 文件 0600 更新;新 master_key 仅响应一次。
- E5 已有 auth 用例回归(空 hash 开放 / bearer 401+200 不变)。

**Web / 构建 / 二进制冒烟:**
- `npx tsc --noEmit`(web)干净;`make ui` 重嵌 + `go build ./...` 干净;`go vet` 干净;`go test -count=1 ./...` 全绿;`-race` service/server/settings 绿。
- 二进制冒烟(scratch db):/setup 初始化 → os endpoint add(enc:v1) → curl PUT /settings(http_port=8899、poll_min=3、queue_work=true、digest_time=off)→ 无参 `os server` 起在 :8899(boot 日志显示 digest off / queue on / per-company notify)→ 公司配 feishu_webhook=file://邮箱 → 触发一次待审批(os 建任务走 approval)→ 邮箱有告警;`--digest-now` 邮箱出现该公司独立日报 → POST rotate-master-key → 新 key 一次返回、旧 key 解不开、`<db>.key` 0600 → PUT /settings 校验 scripted 400 与 console token 仍 200。

## 六、风险与取舍

1. **Web 设不了 scripted(决策③)**:离线确定性只经 env;DB 层 engine_mode 列仍会解析(company/global 为 scripted 时 engineScripted=true),仅 Web/API 拒写——防 Web 误把生产推进 scripted,同时不破坏既存 scripted 测试线。
2. **通知/摘要只走公司机密(决策②)**:公司未配 feishu_webhook = 无即时告警/无日报(不再有单 env 全局兜底);无公司归属任务(空 CompanyID)不路由。这是 company 隔离的预期代价,契约明确告知。
3. **approval 无 company 列**:摘要按公司统计 approval 必须经「本公司任务 id 集」过滤(ListTasks(companyID)→ set → ListApprovals(""))——正确但多一次全表 approvals 读,单算子量级可接受。
4. **server 参数 boot 读一次**:Web 改了参数须重启 os server 生效(非热更);缺行回内置默认与旧 flag 默认一致,存量启动行为不变。
5. **AppSetting 整行 Upsert 的 hash 守卫**:3.6 红线——PUT /settings 漏带 console_token_hash 会清空致鉴权回落开放;handler 内先读当前行保 hash,并有 E1 guard 测试。同理 engine_mode_default/agent_cli_default 等均合并部分 patch。
6. **re-key 崩溃窗口**:rotate 在「RekeyAll 双表写完成后、SaveKey 前」若进程崩溃,DB 已新 key、文件仍旧 key → 需人工恢复(与 9.2 `/setup` 接受同类单次风险;校验错误零写已消除「错 key 破坏存量」大类)。单算子单机 + 低频操作,文档明示。
7. **OpenSecretCurrent 每调用一次 DB 读 + 解密**:事件点/摘要低频,可接受;不引入缓存(机密轮换即时生效优先)。
8. **删 OS_FEISHU_* 波及**:notify_test 的 NewFromEnv 用例与任何依赖 env 注入的调用点需改写;历史 shell 脚本若直接设 env 通知,改为公司机密配置(本阶段文档化)。

## 七、登记

- 设计索引 `docs/phase9/design/README.md` 增本契约行;定稿后按 3.1-3.7 实施;落地归档 `docs/phase9/stages/3.md`;`docs/进度总表.md` Phase 9 行记 9.3 完成。
