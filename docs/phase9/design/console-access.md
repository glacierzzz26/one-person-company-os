# Phase 9.2 — 控制台访问治理 + 首启引导(/setup + console token 哈希鉴权 + 主密钥 runtime 注入 + 轮换)(实施契约)

> Phase 9 子阶段实施契约,承接方向 [config-governance.md](config-governance.md)(定稿,8 项决策)§六 9.2 行 + §五 D1/D2/D3;9.1 契约([settings-foundation.md](settings-foundation.md),已冻结)显式 defer 的「/setup / 控制台令牌 / 首启 re-key / 主密钥运行时注入」在本阶段落地。
> 定稿门(2026-09-06,AskUserQuestion 两项决策):**① 含最小 SPA**——/setup 向导页 + 壳内「控制台访问」卡(令牌轮换),Web 首启闭环本轮即用;② **endpoint 包级 holder**——`UseMasterKey` 注入,OpenToken/SealToken 主密钥优先、env 仅测试 seam。
> 代码现状立足点 = **9.1 落地后工作树**(migration 0012 三表 + settings 包 + repo/service settings + endpoint seal With 变体,未提交)。

## 一、范围与边界

**做(9.2 交付物):**

1. **endpoint seal 主密钥注入**:包级 holder `UseMasterKey(key []byte)`;`currentKey()` 解析 = holder(若有)→ env seam(无,测试兜底,语义不变);`SealToken/OpenToken` 内部改走 `currentKey()`;存量 enc:v1 前缀/空串语义不变。
2. **`/setup` 首启 API + 向导**:未初始化(`app_setting.console_token_hash=''`)→ `GET /api/v1/setup/status`(open)+ `POST /api/v1/setup` 完成一次初始化(服务端生成主密钥 → 落 `<db>.key`(0600)→ 可选旧 `OS_ENDPOINT_KEY` 一次性 re-key → 设 console 令牌哈希 → 返回主密钥**仅此一次**);已初始化 → 409。
3. **`/api/v1` 鉴权改 DB 哈希**:`apiAuth` 每次请求读 `global.console_token_hash`;空 = 未初始化 → 开放(与现空 token 语义一致,存量 server 测试零适配);非空 → `Bearer <token>` 的 `sha256hex` 与哈希 **constant-time** 比较,否则 401。删除 `apiToken` 常量字段与 `SetAPIToken`(env 不再权威,9.4 起 CLI 收口)。
4. **控制台令牌轮换 API**:`PUT /api/v1/settings/console-token`(Bearer 鉴权后,初始化才可)→ 换新哈希,**旧即失效**。
5. **CLI root 注入主密钥**:开库解析出最终 db path 后,`settings.LoadKey(KeyPath(path))` 存在 → 解码 → `endpoint.UseMasterKey`(CLI `os queue work`/task run live 也能解 re-key 后端点 token);keypath 存包变量供 `os server` 用(server `/setup` 需要落盘路径)。
6. **最小 SPA**:App 启动 `GET /api/v1/setup/status` → 未初始化 → 全屏 `/setup` 向导(令牌输入 + 旧 key 可选 + 完成);成功后回写令牌进 localStorage(复用 `os.token`)并显示主密钥一次;新增「设置」页(壳内 nav)含「控制台访问」卡 = 状态 + 令牌轮换。**不做** engine/agent/issue/digest/server 参数等旋钮设置页(9.3 全量铺)。
7. 配置写审计(settings):`SetConsoleTokenAs` + setup 落 audit(actor=`human:console`),补 9.1 顺延的 settings 写审计。
8. 测试 + SPA 构建 + 归档 + 进度总表登记。

**不做(留给后续/live,防越界):**

- **旋钮 Web 化 / server 零参数 / 通知按公司**:9.3(本轮 server 仍读 flag/env 起,digest/poll/queue 不动)。
- **CLI 写命令收口 / deprecated**:9.4。
- **「更换主密钥(re-key 全量机密)」设置页动作**:9.3 设置页铺全时一并(9.2 `/setup` 只覆盖首启一次性 re-key)。
- **多管理员/RBAC、会话过期、令牌审计历史**:单算子单令牌,方向既定不做。
- **新增迁移**:零迁移(0012 已含 `console_token_hash`;keyfile 在 DB 旁非库内)。无 go.mod 变更。

## 二、代码现状核实(立足点,9.1 落地后工作树)

| # | 事实 | 位置 |
|---|---|---|
| 1 | server auth = 启动注入常量 `apiToken`(env OS_API_TOKEN),`apiAuth` 空开 / constant-time compare(sha256 未用);`Server{svc,poll,digest,apiToken,queue}` | `internal/server/server.go:39,164-182`;`cli/server.go:41` `SetAPIToken(os.Getenv("OS_API_TOKEN"))` |
| 2 | `service.AppSetting(ctx)` 已回退内置默认且含 `ConsoleTokenHash`(0012 列,''=未初始化);`service.UpsertAppSetting` 可写 | `internal/service/settings.go`(9.1) |
| 3 | 主密钥文件模块齐:settings.`KeyPath/GenerateKey/SaveKey/LoadKey`(0600,64 hex);re-key 两遍 `service.RekeyEndpointTokens(ctx, oldKey, newKey) (int, error)` 已备 | `internal/settings/keyfile.go`;`internal/service/settings.go` |
| 4 | endpoint seal 已抽 With 变体但运行期调用点走 env:`SealToken/OpenToken` 内部 `envKey()`;engine.go:84 / delegate.go:356 / service/endpoint.go:79 `endpoint.OpenToken(e.TokenEnc)` | `internal/endpoint/seal.go`(9.1) |
| 5 | CLI root `PersistentPreRunE` 解析 `path := dbPath ‖ cfg.DBPath` → `storage.Open(path)` → `svc = service.New(...)`;dbPath 未回写给 server | `internal/cli/root.go:28-48` |
| 6 | `/api/v1` 路由注册 `r.Route("/api/v1", func(r){ r.Use(s.apiAuth); s.registerAPIRoutes(r)})`;SPA 兜底 `/*` console.Handler | `internal/server/server.go:154-160` |
| 7 | server 写 handler 审计 actor 常量 `consoleActor`;envelope `writeJSON` 已有 | `internal/server/api.go` |
| 8 | SPA:React18+TS+AntD+react-router;8 页挂 `Shell`;`AuthModal` 401 → 输入令牌存 `localStorage 'os.token'`;`client.ts` `TOKEN_KEY/unauthorizedHandler`;8 页无设置页 | `web/src/App.tsx`、`web/src/components/AuthModal.tsx`、`web/src/api/client.ts` |
| 9 | 唯一用 SetAPIToken 的测试 `TestAPIAuthToken`(无头 401 / 带头 200)→ 9.2 改走 DB 哈希初始化路径 | `internal/server/api_test.go:246-269` |
| 10 | web 构建管线:`make ui` = web npm ci+build → `cp -r web/dist/. internal/console/ui/`(assets gitignored,index.html 占位被覆盖 → 提交前须 git checkout) | `Makefile` |

## 三、契约设计

### 3.1 endpoint seal holder(§一.1)

```go
// internal/endpoint/seal.go 追加
// masterKey 进程级主密钥(server/CLI 开库后 LoadKey 注入;nil = 未初始化,退回 env 测试 seam)。
// Phase 9 方向 §3.4「OpenToken 密钥源 = 主密钥文件句柄(注入 endpoint 包)」;D3:env 仅测试 seam。
var ( masterKey []byte; masterKeyMu sync.RWMutex )
func UseMasterKey(key []byte)              // nil = 清除(回 env);非 nil 拷贝存(防调用方复用缓冲)
func CurrentKeySource() string             // "master-key" | "env"(日志/测试用,不泄 key)
func currentKey() ([]byte, error)          // holder → envKey()
// SealToken/OpenToken:key 解析改 currentKey();其余 AAD/前缀/空串逻辑不变。
```

语义:master 未设 → 与 9.1 完全一致(env seam,存量测试零影响);master 设了 → 产品路径用主密钥解 re-key 后 enc:v1。

### 3.2 settings hash helper

```go
// internal/settings/token.go
// HashConsoleToken console 访问令牌哈希(sha256 hex;DB 存哈希不存明文)。
func HashConsoleToken(plain string) string // sha256.Sum256([]byte(plain)) → hex
```

### 3.3 service 增方法(settings 写审计本轮落地)

```go
// internal/service/settings.go 追加
func (s *Service) ConsoleTokenHash(ctx) (string, error)      // AppSetting().ConsoleTokenHash
func (s *Service) SetConsoleTokenAs(ctx, plain, actor string) error
//   空 plain 拒绝;hash = settings.HashConsoleToken;读现 global(Upsert 前保留其余字段);
//   Upsert;audit("settings","global","set-console-token",actor,"")
func (s *Service) SetConsoleToken(ctx, plain string) error    // 便捷(=As, actor "human:cli"),测试用
```

re-key 复用既有 `RekeyEndpointTokens`;主密钥生成/落盘/注入属 server 编排(见 3.4),service 不持路径。

### 3.4 server:鉴权 DB 化 + setup/rotate API

```go
// Server struct:删 apiToken 字段;增 masterKeyPath string
func (s *Server) SetMasterKeyPath(p string)          // cli/server 注入(<db>.key 路径)
func (s *Server) Initialized(ctx) (bool, error)      // svc.ConsoleTokenHash()!=""
// apiAuth:每次请求 s.svc.AppSetting(r.Context()) → hash=="";
//   空 → 开放;非空 → Bearer sha256hex == hash(hex 字符串 constant-time)。
//   401 envelope code "unauthorized"(沿用)。
```

**路由**(挂 `/api/v1` 组内,复用 apiAuth 语义):
- `GET  /api/v1/setup/status` → `{ok,data:{initialized:bool}}`(未初始化 open;初始化后带 token 也能查)。
- `POST /api/v1/setup`(未初始化才可;handler 自守卫)
  body `{ "console_token": string, "old_endpoint_key": string(可选,64hex 或空) }`
  流程(原子,任一失败 → 400 且零持久化):
  1. `Initialized()` true → 409 `{"code":"already_initialized"}`。
  2. `console_token` 空/len<8 → 400;`old_endpoint_key` 非空须 64 hex(否则 400)。
  3. `master := settings.GenerateKey()`;`newKey,_ := hex.DecodeString(master)`。
  4. old_key 给了 → `svc.RekeyEndpointTokens(ctx, oldKey, newKey)`;失败 → 400 透传(零写回,两遍保证)。
  5. `settings.SaveKey(s.masterKeyPath, master)`(失败 → 500;masterKeyPath 空 → 500「server 未配 key 路径」)。
  6. `endpoint.UseMasterKey(newKey)`(进程内生效,免重启)。
  7. `svc.SetConsoleTokenAs(ctx, console_token, consoleActor)`。
  8. `svc.audit…` setup 记一条(经 service 内 audit? server 无 audit 助手 → 借 `svc.SetConsoleTokenAs` 已 audit;setup 额外 detail 记 re-key 数可省略,保持单 audit)。
  9. 200 `{ok,data:{initialized:true, master_key:master}}`(**仅此一次**;前端即显示)。
- `PUT /api/v1/settings/console-token`(bearer 后;未初始化 → 409)
  body `{ "new_token": string }` → 校验非空 → `svc.SetConsoleTokenAs(ctx, t, consoleActor)` → 200 `{ok,data:{rotated:true}}`。

### 3.5 CLI 注入

```go
// internal/cli/root.go PersistentPreRunE(open 之后):
//   path 已解析;masterKeyPath = settings.KeyPath(path)(包变量,serverCmd 读用)
//   if k,err := settings.LoadKey(masterKeyPath); err==nil {
//       kb,err := hex.DecodeString(strings.TrimSpace(k)); if err==nil { endpoint.UseMasterKey(kb) }
//   } // 无 key 文件 = 未初始化/env seam,静默
// internal/cli/server.go:删 SetAPIToken(os.Getenv("OS_API_TOKEN"));srv.SetMasterKeyPath(masterKeyPath);
//   /api/v1 auth 启动日志改读 srv 初始化态(Initialized 需 ctx → RunE 内查 svc)
```

### 3.6 SPA(最小)

- `web/src/api/types.ts/endpoints.ts`: `setupStatus()`(GET setup/status)、`runSetup({console_token, old_endpoint_key})`(POST)、`rotateConsoleToken(new_token)`(PUT)、类型 `SetupStatus{initialized}`,`SetupResult{initialized, master_key}`。
- `web/src/App.tsx`:App 挂载取 status;**未初始化 → 渲染 `<SetupWizard/>`**(不复用 Shell);已初始化 → 现有路由;新增 `/settings` 页(Shell 内,top nav + 图标)。
- `web/src/pages/SetupWizard.tsx`(新):步骤式(简介 → 令牌 + 旧 key 可选 → 完成页显示主密钥一次 + 一键复制 + 「保存并进入」)。成功 → `setToken(console_token)` 存 localStorage → 路由到 `/`。
- `web/src/pages/Settings.tsx`(新):「控制台访问」卡 = 已初始化徽标 + 新令牌输入 + 轮换按钮(旧即失效提示)+ 401 → AuthModal 现有流。
- 主密钥后续「更换/re-key」入口留 9.3(不在本轮 UI)。

## 四、文件落地清单(9.2)

| 文件 | 内容 |
|---|---|
| `internal/endpoint/seal.go`(改)+ `internal/endpoint/seal_test.go`(增) | UseMasterKey/currentKey + OpenToken/SealToken 改走 currentKey;holder 用例 |
| `internal/settings/token.go`(新) | HashConsoleToken |
| `internal/service/settings.go`(改) | ConsoleTokenHash / SetConsoleTokenAs / SetConsoleToken(audit) |
| `internal/server/server.go`(改) | 删 apiToken 字段+SetAPIToken+APITokenSet;apiAuth DB 化;SetupStatus/Setup/Rotate handler;SetMasterKeyPath |
| `internal/server/api.go`(改) | 注册 3 路由 |
| `internal/cli/root.go`(改)+ `internal/cli/server.go`(改) | masterKeyPath 解析 + UseMasterKey 注入;server 接 path、去 env token |
| `internal/server/api_test.go`(改)+ 新 `setup_test.go` | TestAPIAuthToken 改 DB 初始化路径;A1/A2/A3 用例 |
| `web/src/api/*` / `App.tsx` / `pages/SetupWizard.tsx` / `pages/Settings.tsx`(新/改) | status/setup/rotate 接线 + 向导 + 设置卡 |
| `docs/phase9/stages/2.md`(归档)+ `design/README.md` + `docs/进度总表.md` | 收口凭证 |

依赖:全 stdlib + 既有;零迁移;无 go.mod 变更。web 需 `npx tsc --noEmit` + `make ui` 重嵌(提交前恢复 index.html)。

## 五、用例清单(验收凭证)

| # | 用例 | 断言 |
|---|---|---|
| A1 | 首启初始化(httptest) | GET setup/status false → POST /setup{console_token} → 200,data.master_key=64hex;再 status true;`/api/v1/companies` 无头 401、带头(=console_token)200;重复 POST → 409 |
| A2 | setup re-key + 原子性 | env 造 enc:v1 端点(OS_ENDPOINT_KEY)→ POST /setup 带 old_endpoint_key → 200;该端点 `OpenTokenWith(masterKey)` 可解、旧 env key 失败;错 old key → 400 且仍未初始化(哈希未设、token 未动、零写回) |
| A3 | 轮换 | PUT /settings/console-token{new_token}(旧 bearer)→ 200;旧 bearer 401、新 200;未初始化 PUT → 409 |
| B1 | holder | UseMasterKey(k) 后 SealToken/OpenToken 用 k(OS_ENDPOINT_KEY 设了别的也解不出;roundtrip 用 k);UseMasterKey(nil) 回 env;env seam 未设 key 无 master → Seal 报错(语义不变) |
| B2 | hash | HashConsoleToken 稳定/不同 token 不同;空串拒(SetConsoleTokenAs 校验) |
| C1 | SPA | `npx tsc --noEmit` 干净;`vite build` 产出;make ui 重嵌后 go build 绿 |
| C3 | 回归 | 0–7/8.x + 9.1 全量绿(除 TestAPIAuthToken 按新契约改写);`-race service` 绿;gofmt/vet 干净 |

验收口径:gofmt 本次改动干净;`go vet`/`go build` 干净;`go test -count=1 ./...` 全绿;`-race ./internal/service/`、`./internal/server/` 绿;`npx tsc --noEmit`(web)干净。

## 六、风险与取舍

- **auth 每请求读 app_setting**:单行 PK 读,单算子可接受;不做内存缓存(避免初始化后需重启刷新)。
- **进程级 master holder**:server 与 CLI 各自进程注入各自 key 文件;测试各自 TempDir DB、不用 UseMasterKey → 无串扰;用 holder 的用例须在测试内复位(nil)。
- **setup 原子性**:失败任一步 → 400 且 key 文件/哈希/re-key 均未落(顺序:re-key 校验先行 → SaveKey → UseMasterKey → 设令牌);SaveKey 成功后才 UseMasterKey,避免「key 文件在但进程未注入」窗口。
- **旧 key 语义**:不输 old_endpoint_key = 按 D2「dev/scratch 跳过」——库里有存量 enc:v1 token 时须输旧 key,否则 runtime 解不开(fail-closed,启动日志明示)。
- **pre-init API 开放**:与今日「env 空 = 开放」姿态一致;方向接受单算子本地,暴露公网务必先 /setup。

## 七、登记

- 定稿后登记 `docs/phase9/design/README.md`(9.2 契约 row ✅)+ 落地后归档 `docs/phase9/stages/2.md` + `docs/进度总表.md`(Phase 9 行 + 子阶段明细)。
