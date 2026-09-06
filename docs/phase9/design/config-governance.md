# Phase 9 — 配置治理中心:Web 唯一配置入口 + company 隔离 + 机密治理(方向设计)

> **状态:已定稿(2026-09-06,定稿门 8 项决策通过)。** 定稿冻结,按它写代码;每子阶段另立实施契约(循 Phase 8 惯例)后落地归档 `stages/<n>.md` 并在进度总表登记。
> 触发:2026-09-06 用户要求 —— **不能通过命令行/env 注入任何配置参数,必须全部通过 Web 配置;token 每项目独立**(项目 = company)。
> **定稿门八项决策(2026-09-06,AskUserQuestion):** ① 隔离边界 = 按 company(company=项目,配置/机密全落 company 级);② 主密钥 = Web 首启 `/setup` 生成-显示一次-服务端落 0600 密钥文件;③ 范围 = 一次性全量 Web 化(engine/agent CLI/issue 源/通知/控制台访问/server 运行参数全收编);④ CLI = 只读/排障,Web 为唯一配置写入口;D1 = Web 生成-显示一次-落盘;D2 = 存量 enc:v1 引导页输旧 key 一次性 re-key;D3 = 保留 go test env seam,产品面 DB 优先;D4 = server 零参数为主,业务 flag 保留但 deprecated(仅排障临时覆盖)。
> 同目录 `declarative-pipelines.md`(声明式流水线)为**未立项存档草案,不受本文件影响,仍不入索引**(立项时按 Phase 10 起排)。
> 代码现状全部经仓库核实(2026-09-06,HEAD 06bfc22);文件/行号照抄真实代码,不凭想象。

## 一、问题与目标

**现状**:OS 的配置与机密散落在 **进程环境变量** 与 **CLI 启动参数** 里,service 代码 `os.Getenv` 即取即用(§三清单);控制台 `/api/v1` 访问令牌也是启动 env(`OS_API_TOKEN`)。这带来:

1. **配置无法用产品方式表达**:没有配置页、没有设置模型、没有历史与审计。
2. **机密无隔离**:GitHub token / 端点 token 主密钥 / 飞书 secret 全是进程级,company 之间天然共享同一进程 env,无法做到「每项目独立」。
3. **运维姿势靠 shell**:换 token/切 issue 源/切 engine 模式都要改启动 env 并重启进程 —— 正是本次要求否决的姿势。
4. **无加密密钥治理**:端点 token 加密主密钥 `OS_ENDPOINT_KEY` 靠 env 进进程,与「Web 输入并保存」矛盾。

**目标**:配置与机密**唯一写入口 = Web 控制台**;**company 为配置隔离边界**(机密/端点/issue 源/token 各公司独立);服务端与运行时**默认零 env/零启动参数**即可运行(参数收进库内设置);CLI 退化为只读/排障;机密 **AES-GCM 加密落库**,主密钥经 Web 首启引导输入并落本机 0600 密钥文件。

## 二、范围与不做

**做(本方向):**

1. **配置模型两档三层**:`global`(服务自身:控制台访问、主密钥落位、engine 默认、agent CLI 默认、server 运行参数、digest/通知全局默认)与 `company`(每公司:GitHub token、issue 源、端点 token 早已 company、engine 覆盖、飞书 webhook、委派后端端点引用);company 覆盖 global 默认(仅对覆盖项)。
2. **机密治理**:主密钥 Web 首启引导(输入或生成 → 落本机 `0600` 密钥文件);全量机密(token/github/飞书 secret/委派凭据)AES-GCM 加密落 `secret` 表,读取即时解密;现有 `enc:v1`(OS_ENDPOINT_KEY 加密的端点 token)在引导页**一次性 re-key** 进新主密钥。
3. **运行时旋钮全部 Web 化**(engine 模式、agent CLI、issue 源、digest、通知、控制台访问、server 运行参数:端口/轮询/队列/digest 时刻)。
4. **server 可零参数启动**:`os server` 读库内设置;命令行 `--port` 等降为只读排障时的临时覆盖(或移除,见 §五决策点 D4)。
5. **控制台鉴权首启引导**:未初始化(无访问令牌/无管理员)→ 开放首启页 `/setup`;初始化后 `/api/v1` 全量要求 Bearer(令牌哈希落库,设置页可轮换)。
6. **CLI 只读/排障收口**:写操作(配置面)从 CLI 命令摘除或标注 deprecated,保留 `show/list/run/sync/approve` 等执行与读;§四列触达面。
7. 迁移 0012+ 与 sqlc/schema 同步;**无 go.mod 变更**(沿用全 stdlib + 既有依赖)。

**不做(留给后续/live,防越界):**

- **多管理员/角色权限体系**:仍是单算子系统;只保证「每个 company 机密互不可见」与「配置写入口收敛到 Web」,不做 RBAC/多人审批。
- **config 外部同步/导出**(如 gitops 化)、加密备份导出。
- **gateway(自建 OpenAI 兼容网关)本身的管理面**:端点 token 治理在 OS 内做,网关进程是外部前置。
- **不为 go test 确定性测试做产品化配置**(§五 D3:env 仅内部测试 seam,非配置面)。
- **声明式流水线(Phase 10 候选)不并入本阶段**。

## 三、现状触点清单(全量,经仓库核实)

### 3.1 env 旋钮(生产面,本次全部收编)

| # | 变量 | 读取点 | 语义 | 新归属(建议) |
|---|---|---|---|---|
| 1 | `OS_ENGINE_MODE` | engine.go:48 / intake.go:213,277 / planner.go:49 / delegate.go:326 / tier.go:25 | live \| scripted(判读/委派门 + scripted 确定性分界) | `global.engine_mode` 默认 + `company.engine_mode` 覆盖(live 语义) |
| 2 | `OS_AGENT_CLI` | delegate.go:49 | 委派工具族 claude \| codex | `global.agent_cli` 默认 + `company.agent_cli` 覆盖 |
| 3 | `OS_GITHUB_TOKEN` | intake.go:266 | 通道 B GitHub issue 源鉴权 | `company.secret.github_token`(**机密**) |
| 4 | `OS_ISSUE_SOURCE` | intake.go:258 | github \| fixture(issue 源选型) | `company.issue_source` |
| 5 | `OS_FIXTURE_ISSUES` | intake.go:260 | fixture JSON 路径(离线冒烟) | `company.issue_fixture_path`(company 级,可指本地文件) |
| 6 | `OS_ENDPOINT_KEY` | endpoint/seal.go:20 | 端点 token AES-GCM 主密钥(64 hex) | 废除;改 Web 首启引导 → 本机 0600 密钥文件(§五 D1/D2) |
| 7 | `OS_FEISHU_WEBHOOK` | notify/notify.go:45 | 飞书通知 webhook(file:// 双 sink) | `company.secret.feishu_webhook`(**机密**,支持 file:// ) |
| 8 | `OS_FEISHU_SECRET` | notify/notify.go:51 | 飞书签名 secret(可选) | `company.secret.feishu_secret`(**机密**) |
| 9 | `OS_API_TOKEN` | cli/server.go:41 → server.go SetAPIToken | `/api/v1` Bearer(空=开放) | `global.console_token_hash`(首启 `/setup` 生成;API 常量时比较改哈希) |
| 10 | `OS_FEISHU_DIGEST` | cli/server.go:32 | 每日摘要时刻 env 覆盖 | `global.digest_time`(web 设置) |

### 3.2 server 启动参数(CLI flag,同样收编 `global.server_*`)

| flag | 位置 | 语义 |
|---|---|---|
| `--port`(默认 8787) | cli/server.go:86 | 监听端口 |
| `--poll`(默认 5) | cli/server.go:87 | GitHub 轮询间隔分钟 |
| `--queue-work` / `--queue-interval` | cli/server.go:90-91 | server 自消费任务队列开关/间隔 |
| `--digest HH:MM` / `--digest-now` / `--digest off` | cli/server.go:88-89 | 每日摘要时刻/立即摘要 |
| `--db` / `--config`(持久层定位) | root.go | 数据目录:保留为**唯一允许的启动参数**(定位 DB/密钥文件,非业务配置) |

### 3.3 scripted 确定性内容(OS_SCRIPT_*,非配置面)

`OS_SCRIPT_TEST/REVIEW/TRIAGE/PLAN`(engine.go:127,143 / intake.go:299 / planner.go:82)= 离线冒烟给模型判读的**确定性桩输出**,是 go test 内部测试 seam,**不是运营配置,不进 Web**(§五 D3)。与 3.1 表 1 的 env 读取同理收进 runtime 解析函数。

### 3.4 结构性触点(非 env,但 Web 化要改架构)

| 触点 | 现状 | 改造方向 |
|---|---|---|
| notify 注入 | `service.SetNotifier(notify.NewFromEnv())` 启动注入**单例**(root.go:46);事件点(approval/digest)用该单例 | 通知按 company 解析:事件发生时按 company 读 secret.feishu_webhook 构造/取缓存通知器;无 → 跳过。`NotifyEnabled` 语义改为「存在任一 company 或 global 配了 webhook」或按 company 判 |
| 控制台鉴权 | `srv.apiToken` 启动注入常量,constant-time 比较(server.go:164-177);空=开放 | 令牌哈希落库:请求时查 `global.console_token_hash`,比较 `sha256(bearer)`;首启 `/setup` 建令牌 |
| service 构造 | `service.New(store)` + root.go 注入 env 派生对象 | `service.New(store, settings)` 注入 settings 读取器;或 service 内惰性查 store(单例缓存) |
| 判读/委派端点模型 | 端点 token 已 company 级,`seal.go` OpenToken 即时解密(密钥 env) | OpenToken 密钥源 = 主密钥文件句柄(server 首启加载注入 endpoint 包 or 传 key);company 级不变 |

## 四、配置模型与数据模型(草案)

**层级**:`global`(1 行服务自身)< `company`(每公司一行 + N 条机密)。repo 级暂不新增独立配置(仓库继承公司 issue 源与 token;workspace 已在 repo 行)。

**新表(migration 0012,只增不改):**

```sql
-- global 设置:服务自身(key/value 或类型化列)。单行表。
CREATE TABLE app_setting (
  id TEXT PRIMARY KEY,            -- 'self' 单行
  engine_mode_default TEXT NOT NULL DEFAULT 'live',  -- live|scripted
  agent_cli_default  TEXT NOT NULL DEFAULT 'claude', -- claude|codex
  console_token_hash TEXT NOT NULL DEFAULT '',       -- ''=未初始化(首启开放 /setup)
  digest_time  TEXT NOT NULL DEFAULT '09:00',
  http_port    INTEGER NOT NULL DEFAULT 8787,
  poll_min     INTEGER NOT NULL DEFAULT 5,
  queue_work   INTEGER NOT NULL DEFAULT 0,   -- 0|1
  queue_interval_sec INTEGER NOT NULL DEFAULT 10,
  updated_at INTEGER NOT NULL
);

-- company 覆盖 + 非机密配置
CREATE TABLE company_setting (
  company_id TEXT PRIMARY KEY REFERENCES company(id),
  engine_mode TEXT,              -- NULL=继承 global
  agent_cli   TEXT,              -- NULL=继承 global
  issue_source TEXT,             -- NULL=继承 global(github);显式 github|fixture
  issue_fixture_path TEXT,       -- NULL=无(fixture 源必填)
  updated_at INTEGER NOT NULL
);

-- 机密:company 级;值 AES-GCM 密文(enc:v2:),主密钥=首启密钥文件
CREATE TABLE secret (
  id TEXT PRIMARY KEY,           -- 'github_token' | 'feishu_webhook' | 'feishu_secret'
  company_id TEXT NOT NULL REFERENCES company(id),
  cipher TEXT NOT NULL,          -- enc:v2:<b64>
  updated_at INTEGER NOT NULL,
  UNIQUE(company_id, id)
);
```

端点 token 沿用 `endpoint.token`(现 `enc:v1:`)列,re-key 后密文格式不变、密钥源切换为新主密钥文件(前缀可升 `enc:v2:`,见 §五 D1)。

**配置解析(运行时确定性,服务单例):**
```
effective = company_setting.X ?? app_setting.X_default ?? 内置默认
env 仅在「未初始化/无 DB 值」时作内部测试兜底(§五 D3),产品面不读取、不宣传。
```

**Web 面(新增页面,均以 company 上下文;无 company 的选择 global 页):**
- `/settings`(global):engine 默认、agent CLI 默认、digest、server 运行参数(端口/轮询/队列)、控制台访问令牌轮换、主密钥查看(re-key)入口。
- 每 company `设置/集成` 页:GitHub token、issue 源(fixture 路径)、飞书 webhook/secret、engine 覆盖;端点池 token 已在此(补「解密验证/重输」即可)。
- 首启:`/setup`(未初始化)建 console 令牌 + 输入/生成主密钥(0600 落盘)+ 可选旧 `OS_ENDPOINT_KEY` re-key。

**审计**:配置写操作(`app_setting/company_setting/secret`)落 audit,actor=`human:console`(循 7.2 惯例);机密字段 detail 只记 `*` 或 id,不记明文。

## 五、定稿决策记录(定稿门 2026-09-06)

- **D1 主密钥形态(定稿:Web 生成-显示一次-落盘)**:首启 `/setup` 页生成 64 hex 主密钥,显示一次,服务端立即落密钥文件(chmod 0600,路径随 DB 旁 `<db>.key`);设置页提供「更换主密钥(re-key 全量机密)」。免手输、防弱密钥。
- **D2 旧 `enc:v1` 迁移(定稿:引导页输旧 key 一次性 re-key)**:`/setup` re-key 步骤输入旧 `OS_ENDPOINT_KEY` → 全量解 enc:v1 → 新主密钥重加密写回;迁移后旧 env key 作废。dev/scratch 库可跳过(清空 token 经 Web 重录)。
- **D3 env 兜底边界(定稿:保留 go test seam,产品面 DB 优先)**:运行时解析 DB(company→global→内置默认)优先;`OS_ENGINE_MODE`/`OS_SCRIPT_*` 的 env 读取仅保留为 go test 确定性 seam(600+ 测试零改造),文档标注「非配置面、产品不读」。不采用彻底零 env(需大改测试种子)。
- **D4 server 启动参数(定稿:零参数为主,flag deprecated 排障覆盖)**:`os server` 零参数读库(端口/轮询/队列/digest 全 global 设置);`--db`/`--config` 为数据定位唯一正式参数;`--port`/`--poll`/`--queue-*`/`--digest` 保留但标注 deprecated,仅排障临时覆盖(不构成配置入口)。
- **D5 Phase 编号**:本方向占 **Phase 9**(声明式流水线草案移 Phase 10 候选,未立项不入索引,同目录共存)。

## 六、实施阶段划分(每子阶段独立可验收)

| 子阶段 | 交付物(地基) | 验证口径 |
|---|---|---|
| **9.1 配置存储地基** | migration 0012(app_setting/company_setting/secret)+ sqlc/schema + repo CRUD + service settings 解析(DB 优先 + env 测试 seam)+ 主密钥密钥文件模块(seal 改造:密钥源注入) | 迁移上/下可逆;sqlc diff 仅新表;单测:解析优先级/密钥文件读写/re-key(v1→v2) |
| **9.2 控制台访问治理** | `/setup` 首启 + console token 哈希落库 + `/api/v1` 鉴权改哈希(全量 Bearer)+ 设置页轮换 | httptest:未初始化开放 /setup→初始化后 401/200;轮换即失效旧 |
| **9.3 运行时旋钮 Web 化** | engine/agent_cli/issue_source/fixture/digest/server 参数读库;`os server` 零参数可起;notification 按 company 解析(secret.feishu);端点 token 落 secret 治理(或并入 9.1) | server 零参数冒烟;每 company issue 源/engine 独立生效;通知按公司路由 |
| **9.4 CLI 只读收口 + 归档** | 配置面 CLI 写命令 deprecated/移除;show/list 保留;文档 + 进度总表 + stage 归档 | CLI 只读冒烟;配置全经 Web 断言(写命令报「请用控制台」) |

每子阶段落地写 `docs/phase9/stages/<n>.md`,定稿契约单独立档(循 Phase 8 惯例:方向 + 每子阶段实施契约)。

## 七、风险与取舍

- **机密以 DB 为主存储**:密钥文件 0600 是唯一明文薄弱点;失密钥文件=全部机密不可解密(fail-closed,与现 seal 同哲学)。备份策略 = 密钥文件 + DB 一并备份(单机)。
- **单算子局限**:不引入多用户会话;`/setup` 竞态(双进程同时首启)以「已初始化即锁死 /setup」处理。
- **环境变量残留**:9.3 若遗漏某 `os.Getenv` 生产触点,会静默退回旧姿势 —— 验收以「§三 3.1 表逐行销号」为准。
- **go test 改造面**:seal 密钥源注入会动 endpoint 包签名/测试;`OpenToken/SealToken` 改接收密钥(或包级 key holder),涉及既有端点测试适配 —— 属 9.1 既定面。
- **通知单例→按公司**:`requestApproval`/digest 事件点需能取 company 通知器,是 service 层结构性改动(§3.4),9.3 集中做。

## 八、验收口径(总)

1. **配置唯一写入口 = Web**:除 `--db/--config`(数据定位)外,无任何产品配置经命令行/env 注入;CLI 写配置命令不可用(报「请用控制台」或 deprecated 标注)。
2. **company 隔离**:A 公司的 GitHub token/issue 源/飞书/engine 覆盖不影响 B 公司;API 无法跨 company 读他人机密(机密不随 list 下发,仅掩码)。
3. **机密治理**:token 均密文落库;主密钥经 Web 首启输入/保存(0600);`enc:v1` 可一次性 re-key;无密钥 → 机密相关写 fail-closed。
4. **零参数 server**:`os server` 按库内设置起(端口/轮询/队列/digest);`/setup`→登录→各配置页可改即时生效(无需重启)。
5. `gofmt`/`go vet`/`go build`/`go test -count=1 ./...` 全绿;0–7/8.x 语义零破坏;scripted 确定性保持;`-race` service 绿。
6. 离线冒烟 + **live 验收归用户**(真实 GitHub token、真实飞书 webhook 经 Web 录入后跑通道 B/通知)。
