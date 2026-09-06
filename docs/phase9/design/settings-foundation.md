# Phase 9.1 — 配置存储地基(app_setting/company_setting/secret + 主密钥文件 + seal 密钥注入 + re-key)(实施契约)

> Phase 9 子阶段实施契约,承接方向 [config-governance.md](config-governance.md)(定稿,8 项决策)§六 9.1。
> 定稿门:延续方向定稿 + 用户「继续」(2026-09-06)。定稿冻结,按它写码;落地归档 `stages/1.md` 并在进度总表登记。
> 代码现状全部经仓库核实(HEAD 06bfc22);类型/签名照抄真实代码,不凭想象。

## 一、范围与边界

**做(9.1 交付物,纯地基,不接运行时/不接 Web 页面):**

1. **迁移 0012_config_governance**(只增不改):三张表 `app_setting`(global 单行 `id='self'`)/`company_setting`(每公司覆盖行)/`secret`(company 机密,密文列 `cipher`,PRIMARY KEY(company_id,id));`db/schema.sql` 同步(sqlc schema 源),sqlc 重生成。
2. **新查询**:`db/queries/settings.sql`(app_setting/company_setting/secret 的 Get/Upsert/Delete,company_setting 支持删行回退继承)+ `db/queries/endpoint.sql` 增 `UpdateEndpointToken`(re-key 写回)。
3. **领域模型 `internal/settings`**(新包):`AppSetting` / `CompanySetting` / `Secret` + 内置默认常量 + 密钥文件模块(`Generate/Save/Load/KeyPath`,0600)+ 通用 AES-GCM(显式 key,`enc:v2:` 前缀,供 secret 表)。
4. **endpoint seal 密钥注入**:`endpoint.SealToken/OpenToken`(env 测试 seam,语义不变)重构为内部 `sealWith/openWith(key)`;新增导出 `SealTokenWith(key, plain)`/`OpenTokenWith(key, enc)`(re-key 与未来主密钥路径用)。
5. **repo 层**:`internal/storage/repository/settings.go`(三表 CRUD 映射)+ `repository/endpoint.go` 增 `SetEndpointToken`(raw cipher 写回,不落明文)。
6. **service 层**:`internal/service/settings.go` —— 缺省行解析(Get 不到 → 内置默认)、company 覆盖读写、`EngineModeFor(companyID)` 生效解析(company → global 默认 → `live`)、`SetSecretAs/GetSecret`、`RekeyEndpointTokens(ctx, oldKey, newKey)`(两遍:先全量解密→再全量写回,失败不动库)。
7. 测试 + 归档 + 进度总表登记。

**不做(留给后续子阶段,防越界):**

- **不接运行时消费点**:engine.go/intake.go/planner.go/delegate.go 的 `OS_ENGINE_MODE`/`OS_AGENT_CLI` 等 env 读取**本轮不动**(9.3 统一切 DB;9.1 只把「读得到的设置」与「生效解析函数」立起来)。
- **不做 `/setup`/Web 页/控制台令牌**:9.2 落地(首启引导 + `/api/v1` 鉴权哈希化 + 设置页)。
- **不做 secret 的 Web 读写 API/审计**:9.2/9.3 落地(本轮仅 store/service 能力 + 单测)。
- **不改已冻结 0–7/8.x 语义**:seal 仅加 With 变体,env 路径与密文格式不变 → 既有测试零改动。

## 二、代码现状核实(立足点,全部经仓库核实)

| # | 事实 | 位置 |
|---|---|---|
| 1 | 最新迁移版本 = 11;runner `//go:embed migrations/*.sql` 整数升序;0012 是下一连续号 | `internal/storage/migrate.go`、`migrations/` |
| 2 | 无 settings/secret/auth 表(greenfield);repo 模型仅 9 表,`Repo` 无逐 repo 配置 | `db/schema.sql`、`internal/repo/model.go` |
| 3 | sqlc v1.31.1 `/home/rguo/go_workspace/bin/sqlc`;schema=`db/schema.sql` queries=`db/queries` gen→`internal/storage/query`(json tags) | `sqlc.yaml` |
| 4 | 域模型 `endpoint.Endpoint.TokenEnc json:"token_enc"`(**密文在模型内**,List/Get 随行回读)—— re-key 只需读行 + 写回 cipher,不需明文出库 | `internal/endpoint/model.go:12` |
| 5 | seal 调用点:写入 `service/endpoint.go:36 SealToken`;运行期解密 `engine.go:84`、`delegate.go:356`、`service/endpoint.go:79 OpenToken`;密钥来源 `envKey()` 读 `OS_ENDPOINT_KEY`(fail-closed,空 token 跳过) | `internal/endpoint/seal.go:20-29` |
| 6 | Store 包装 `*sql.DB`+`query.New`;repo 方法同名同签名风格;`now()=time.Now().Unix()` | `internal/storage/repository/store.go` |
| 7 | service 测试 harness:`newSvc(t) (*Service,*repository.Store)`、`seedCompanyID(t, st) string` | `internal/service/delegate_test.go:26,36` |
| 8 | 端点 token 查询无 Update;全查询 `SELECT *` 自动带新列;`ListCompanies :many` 存在(re-key 遍历用) | `db/queries/endpoint.sql`、`company.sql` |

## 三、契约设计

### 3.1 迁移 0012(up/down,只增不改)

```sql
-- up
CREATE TABLE app_setting (
  id TEXT PRIMARY KEY,                 -- 'self' 单行(global)
  engine_mode_default TEXT NOT NULL DEFAULT 'live',
  agent_cli_default  TEXT NOT NULL DEFAULT 'claude',
  console_token_hash TEXT NOT NULL DEFAULT '',
  digest_time TEXT NOT NULL DEFAULT '09:00',
  http_port INTEGER NOT NULL DEFAULT 8787,
  poll_min  INTEGER NOT NULL DEFAULT 5,
  queue_work INTEGER NOT NULL DEFAULT 0,          -- 0|1
  queue_interval_sec INTEGER NOT NULL DEFAULT 10,
  updated_at INTEGER NOT NULL
);
CREATE TABLE company_setting (
  company_id TEXT PRIMARY KEY REFERENCES company(id),
  engine_mode TEXT,                    -- NULL=继承 global
  agent_cli TEXT,
  issue_source TEXT,
  issue_fixture_path TEXT,
  updated_at INTEGER NOT NULL
);
CREATE TABLE secret (
  company_id TEXT NOT NULL REFERENCES company(id),
  id TEXT NOT NULL,                    -- 'github_token' | 'feishu_webhook' | 'feishu_secret' …
  cipher TEXT NOT NULL,                -- enc:v2:<b64>(settings.AES-GCM,主密钥)
  updated_at INTEGER NOT NULL,
  PRIMARY KEY (company_id, id)
);
-- down: DROP TABLE secret; DROP TABLE company_setting; DROP TABLE app_setting;
```

### 3.2 领域 `internal/settings`

```go
// model.go
type AppSetting struct {
  ID string `json:"id"`                 // 'self'
  EngineModeDefault string `json:"engine_mode_default"` // live|scripted
  AgentCLIDefault  string `json:"agent_cli_default"`    // claude|codex
  ConsoleTokenHash string `json:"console_token_hash"`   // '' = 未初始化(/setup 开放)
  DigestTime string `json:"digest_time"`
  HTTPPort int `json:"http_port"`
  PollMin int `json:"poll_min"`
  QueueWork bool `json:"queue_work"`
  QueueIntervalSec int `json:"queue_interval_sec"`
  UpdatedAt int64 `json:"updated_at"`
}
type CompanySetting struct {
  CompanyID string `json:"company_id"`
  EngineMode *string `json:"engine_mode"`      // NULL=继承 global
  AgentCLI *string `json:"agent_cli"`
  IssueSource *string `json:"issue_source"`
  IssueFixturePath *string `json:"issue_fixture_path"`
  UpdatedAt int64 `json:"updated_at"`
}
type Secret struct {
  CompanyID string `json:"company_id"`
  ID string `json:"id"`
  Cipher string `json:"cipher"`               // 不回给前端;API 只出掩码(9.3)
  UpdatedAt int64 `json:"updated_at"`
}
// defaults: DefaultEngineMode="live"; DefaultAgentCLI="claude"; DefaultDigestTime="09:00";
//           DefaultHTTPPort=8787; DefaultPollMin=5; DefaultQueueIntervalSec=10.

// keyfile.go  —— 主密钥(64 hex)文件,路径 = <dbPath> + ".key"
func KeyPath(dbPath string) string          // dbPath+".key"
func GenerateKey() string                   // hex(rand32)
func SaveKey(path, key string) error        // os.WriteFile 0600(创建/覆盖)
func LoadKey(path string) (string, error)   // trim;空 → error

// seal.go   —— secret 表密文(显式 key,enc:v2:)
const SecretPrefix = "enc:v2:"
func SealSecret(key []byte, plain string) (string, error)   // 空 plain → 空
func OpenSecret(key []byte, enc string) (string, error)     // 空 enc → 空
```

### 3.3 endpoint seal 注入(既有语义不变)

`SealToken/OpenToken` 抽出内部 `sealWith(key, plain)/openWith(key, enc)`;env 两函数改调 `SealTokenWith(key,..)` 等新导出变体(参数 `key []byte`)。密文前缀与格式不变 → 存量 enc:v1 兼容。

### 3.4 repo 层(mapper 逐字段,循现有风格)

`repository/settings.go` + `endpoint.go` 增:
`GetAppSetting / UpsertAppSetting / GetCompanySetting / UpsertCompanySetting / DeleteCompanySetting / UpsertSecret / GetSecret / ListSecrets / DeleteSecret / SetEndpointToken(id, cipher)`;sqlc 生成后 diff 仅新表/新查询。

### 3.5 service 层

```go
func (s *Service) AppSetting(ctx) (settings.AppSetting, error)        // 缺行 → DefaultAppSetting()
func (s *Service) UpsertAppSetting(ctx, a) error
func (s *Service) CompanySetting(ctx, companyID) (settings.CompanySetting, bool, error) // ok=false 无行
func (s *Service) UpsertCompanySetting(ctx, cs) error
func (s *Service) DeleteCompanySetting(ctx, companyID) error
func (s *Service) SetSecret(ctx, companyID, id, plain string, key []byte) error   // SealSecret→UpsertSecret
func (s *Service) OpenSecret(ctx, companyID, id string, key []byte) (string, bool, error)
func (s *Service) EngineModeFor(ctx, companyID) (string, error)       // cs.EngineMode → global.EngineModeDefault → "live"
func (s *Service) RekeyEndpointTokens(ctx, oldKey, newKey []byte) (int, error)
// Rekey:两遍——ListCompanies→ListEndpoints 收集 (id, old cipher)→OpenTokenWith(oldKey);
//      全量成功后第二遍 SealTokenWith(newKey)+SetEndpointToken 写回;任何一处失败 → error 且零写回。
```

## 四、文件落地清单(9.1)

| 文件 | 内容 |
|---|---|
| `internal/storage/migrations/0012_config_governance.up/down.sql`(新) | 三表(3.1) |
| `db/schema.sql`(改) | 追加三表(与迁移终态一致) |
| `db/queries/settings.sql`(新)+ `db/queries/endpoint.sql`(改) | 三表 CRUD + `UpdateEndpointToken` |
| `internal/storage/query/*`(sqlc 再生成) | diff 仅新增表/查询 |
| `internal/settings/model.go` / `keyfile.go` / `seal.go`(新包) | 3.2 |
| `internal/endpoint/seal.go`(改) | 抽 sealWith/openWith + 新增 `SealTokenWith/OpenTokenWith`(env 语义不变) |
| `internal/storage/repository/settings.go`(新)+ `repository/endpoint.go`(改) | 3.4 |
| `internal/service/settings.go`(新) | 3.5 |
| 测试(§五)+ `docs/phase9/stages/1.md`(落地后归档)+ `design/README.md` + `docs/进度总表.md` | 收口凭证 |

依赖:全 stdlib + 既有 sqlc;一迁移;无 go.mod 变更。

## 五、用例清单(验收凭证)

| # | 用例 | 断言 |
|---|---|---|
| A1 | 迁移 | `storage.Open(tmp)` 应用 0012;三表存在、默认列回填正确;down 可逆(0009 旧库升级路径) |
| A2 | sqlc/repo 全链 | AppSetting Upsert→Get 往返;CompanySetting upsert→get / Delete→Get 消失(继承语义);Secret upsert→get / List 按 company 隔离 / delete;SetEndpointToken 改 cipher 后 GetEndpoint 回读新值 |
| B1 | 密钥文件 | `GenerateKey`=64 hex;Save 落 0600;Load 往返一致;KeyPath(db) = db+".key" |
| B2 | seal 注入 | `SealTokenWith(key)/OpenTokenWith(key)` 往返;错 key Open 失败;env `SealToken/OpenToken` 行为不变(OS_ENDPOINT_KEY 测试 seam) |
| C1 | 生效解析 | 无 app 行/无 company 行 → "live";app 默认 scripted → 全局生效;company 覆盖 live→"scripted" 优先;删 company 行 → 回落全局 |
| C2 | re-key | 用 env key 造 enc:v1 端点 token → `RekeyEndpointTokens(oldEnvKey, newKey)` → OpenTokenWith(newKey) 成功、oldKey 失败;计数正确;失败路径(错 old key)零写回 |
| C3 | 回归 | 0–7/8.x 全量测试零改动(seal env 语义不变 → 端点既有用例不触) |

验收口径:`gofmt` 干净;`go vet`/`go build` 干净;`go test -count=1 ./...` 全绿;`-race ./internal/service/` 绿;sqlc diff 仅新表/查询。

## 六、风险与取舍

- **迁移最小化**:只增表,不迁数据(secret 表本轮空;endpoint 密文列原样)。存量 enc:v1 token 待 9.2 `/setup` re-key(或手动 `RekeyEndpointTokens`)。
- **seal 双密钥源**:env 测试 seam 保留(D3 定稿);主密钥注入点在 9.2 server/setup 注入,9.1 先备 With 变体 + re-key。
- **两遍 re-key**:第一遍只读/解密收集,全部成功才第二遍写,避免半写坏库;失败报错不动库。
- **`queue_work` int↔bool**:domain 用 bool(json 友好),repo mapper 转 int64(0/1)。

## 七、登记

- 定稿后登记 `docs/phase9/design/README.md`(9.1 契约 row ✅)+ 落地后归档 `docs/phase9/stages/1.md` + `docs/进度总表.md`(Phase 9 行 + 子阶段明细)。
