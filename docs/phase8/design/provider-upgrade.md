# Phase 8.1 — Provider 升级:OpenAI 兼容网关的 messages+tools 契约(实施契约)

> Phase 8 子阶段实施契约,承接方向设计 [model-runtime.md](model-runtime.md) §六 8.1 行与 §四 D1(Provider 契约升级),含 **修订 A**。
> 修订 A(2026-09-04,用户方向补充):**自建 OpenAI 兼容 API 网关,单 key 聚合多厂商模型,OS 不再按厂商适配**;
> 8.1 由「anthropic/openai 双原生 HTTP 实现」改为「**单一 Chat Completions 兼容适配器对接网关**」;模型目录由网关给(选模型=池里挑)。
> 本文件是 8.1 的执行蓝本:代码现状全部经仓库核实,类型/用例照抄真实代码风格,不凭想象。
> 定稿冻结,按它写代码;落地后归档 `stages/1.md` 并在进度总表登记。

## 一、范围与边界

**做(8.1 交付物,全在 `internal/provider`,纯新增 + 标注,不加语义):**

1. **messages+tools 契约**:`Chatter` 接口 + OpenAI Chat Completions 方言的消息/工具类型,支持**多轮原生 tool-call**。契约直接以网关协议为唯一方言 —— 网关即厂商,不再保留双厂商中性抽象(见 §六 风险)。工具调用经网关**透传**到底层模型(已与用户确认网关支持 function calling)。
2. **网关 HTTP 实现**:`openai.go` 一个 Chat Completions 兼容客户端(`Authorization: Bearer` 单 key、`{base}/v1/chat/completions`)。纯 stdlib `net/http`+`encoding/json`,**零新依赖**。路径/鉴权沿用 `service/endpoint.go` models 请求已确立的约定。
3. **scripted 保真**:provider 层就位确定性 `ChatStub`(离线可复现,供 8.2 tool 回合假驱动);现有**服务层 scripted**(`engScripted`/`engScriptedPlan`)一行不改。
4. **claude CLI 标废弃保留**:`ClaudeCLI` 头注标 `Deprecated`,代码不动、0-7 路径继续可用(真实 claude Code agent 委派归 8.5,是另一条独立路径)。
5. **httptest 假网关服务器**:请求形状 + `tool_calls` 回传两轮 + 错误路径,provider 包全绿。

**不做(留给后续子阶段,防越界):**

- **不改 service 层**:`modelCall`/`engCall`/`planCall`/intake triage 仍走 `Generate`→`ClaudeCLI`;生产路径切网关(文本 + 工具都经单轮/多轮 Chat)属 **8.2**(首个 tool 闭环一起切)。8.1 的网关适配是**先行地基**,经 httptest 独立验收,不与 live 路径强绑。
- **不做工具执行**:Go 侧工具执行/工作区落盘/权限归 8.2–8.3;8.1 只定义"模型怎么收发工具调用"。
- **不改 `Provider` 接口、不加迁移/不加 sqlc、不动 go.mod**。`endpoint` 表无新字段(tier 属 8.4 迁移 0010;**选模型机制保留 tier 档位原案**,8.1 不涉及)。
- **不做第二厂商方言**(anthropic /v1/messages 等):将来若绕过网关直连厂商,再按 §六 增补方言,不在 8.1 预支。

## 二、代码现状核实(8.1 的立足点,全部属实)

| # | 事实 | 位置 |
|---|---|---|
| 1 | `Provider` 接口 = `Generate(ctx, Request{Prompt})→(Response{Content}, error)`,单次文本补全 | `internal/provider/provider.go:21-27` |
| 2 | 实现只有 `Stub`(mock 回显)与 `ClaudeCLI`(`Type()` 恒 `"claude"`,真后端 `exec claude -p --output-format text`,30min 超时,env 注入 ANTHROPIC_*);`New(typ,…)` 非 claude 回退 Stub | `internal/provider/claudecli.go:13-80` |
| 3 | 唯一生产调用点:service `modelCall` 先 `endpoint.OpenToken(TokenEnc)` 解密,再 `NewClaudeEndpoint(name, SelectedModel, BaseURL, token).Generate` | `internal/service/engine.go:67-78` |
| 4 | 工程回合三阶段(writer/test/review)各自一次 `engCall`;scripted → 确定性文本,live → `engEndpointFor` 解析端点 → `modelCall` | `internal/service/engine.go:43-63, 82-93, 96-133` |
| 5 | planner 拆解(`planCall`)与 intake triage 共用同一 `modelCall` 真调用路径 | `internal/service/planner.go:50-72`、`internal/service/intake.go:214-254` |
| 6 | 产出收敛靠文本正则:`parseReviewVerdict`/`parseTestPass`/`extractDiff`/`parsePlan`(8.2-8.3 用结构化收敛替换,8.1 不动) | `internal/service/engine.go:148-195` |
| 7 | `endpoint.Endpoint` 已含 `BaseURL/Proto/Vendor/SelectedModel/TokenEnc/Role/Status`,无 tier 字段 | `internal/endpoint/model.go:5-19` |
| 8 | **网关模型目录已能拉取**:`FetchEndpointModels` 走 `{base}/v1/models`,`data:[{id}]`(openai 形状)+ `Bearer`(proto=openai 时),缓存 `models_cache` —— 「获取模型/选模型」的目录能力 6.1 已具备;8.1 只补 **chat(含 tool-use)调用** | `internal/service/endpoint.go:141-230` |
| 9 | token 落库 AES-GCM(`enc:v1:`,`OS_ENDPOINT_KEY` fail-closed),明文不出库 | `internal/endpoint/seal.go` |
| 10 | 迁移 runner 内嵌 `migrations/*.sql`,最大 0009(0007_endpoint 是端点表);8.1 不新增 | `internal/storage/migrate.go` |
| 11 | httptest 请求形状断言风格(`httptest.NewServer` 捕获 method/header/body)已在 notify 测试确立 | `internal/notify/notify_test.go` |

**端点接入约定(网关世界)**:给池子加模型 = 对同一网关 base_url 建多条 endpoint 行,每条 `proto=openai` + 选定的 `selected_model`;多档(tier)端点分法属 8.4。8.1 阶段端点行只用于 future live 路径,httptest 以假网关独立验收。

## 三、契约设计(OpenAI Chat Completions 为唯一方言)

### 3.1 会话形态:一次"逻辑回合"的推进

```
Go 侧(8.2 起的调用方)持会话 []Message:
  ① 组装 [system(可选)] + 历史 → Chat(req)
  ② resp = assistant 回复:Content 文本 与/或 ToolCalls[](一次可多个并行)
  ③ 有 ToolCalls → Go 逐一执行工具(权限/工作区归 8.2)→ 每条结果追加一条
        Message{Role:"tool", ToolCallID:<对应 id>, Content:<输出>}
     → 再 Chat,循环
  ④ 无 ToolCalls → 阶段收敛(文本或结构化信号由 8.2/8.3 判读)
```

规则:assistant 的 `tool_calls` 与紧随的 `role=tool` 回填消息永不拆散跨 `Chat` 边界 —— 调用方把"上一轮 assistant + 各工具结果消息"整体放进下一次 `Chat` 的历史。网关负责把 function calling 翻译给底层模型(含 Claude/GPT 等),OS 只讲 OpenAI 方言。

### 3.2 类型定义(json tag 即网关线格式,实施照抄)

```go
// Message 是一条会话消息。字段与 OpenAI Chat Completions wire 对齐:
//   role=system|user    → Content 文本
//   role=assistant      → Content 文本(可空);若本轮调用工具则带 ToolCalls
//   role=tool           → ToolCallID 引用某次调用 + Content 输出
type Message struct {
    Role        string     `json:"role"`                  // system | user | assistant | tool
    Content     string     `json:"content,omitempty"`     // 文本;assistant 纯工具轮可空
    ToolCallID  string     `json:"tool_call_id,omitempty"` // role=tool 回填引用
    ToolCalls   []ToolCall `json:"tool_calls,omitempty"`  // role=assistant
}

type ToolCall struct {
    ID       string       `json:"id"`
    Type     string       `json:"type"`     // 恒 "function"(8.1 只支持 function)
    Function FunctionCall `json:"function"`
}

type FunctionCall struct {
    Name      string `json:"name"`
    Arguments string `json:"arguments"` // 参数 JSON 的序列化字符串(网关协议如此)
}

// Tool 是注入模型的能力描述;parameters = JSON Schema 对象原样转发。
type Tool struct {
    Type     string         `json:"type"`     // "function"
    Function ToolFunction   `json:"function"`
}
type ToolFunction struct {
    Name        string          `json:"name"`
    Description string          `json:"description"`
    Parameters  json.RawMessage `json:"parameters"` // JSON Schema
}

type ChatRequest struct {
    Model     string    `json:"model"`               // 缺省回退 Provider 构造时 model
    Messages  []Message `json:"messages"`
    Tools     []Tool    `json:"tools,omitempty"`
    MaxTokens int       `json:"max_tokens,omitempty"` // ≤0 不发送(部分网关模型需给,见 §五 用例 1)
}

type ChatResponse struct {
    Content      string     `json:"content"`        // assistant 文本(可能为空)
    ToolCalls    []ToolCall `json:"tool_calls"`     // 非空 → 需执行工具后回填再 Chat
    FinishReason string     `json:"finish_reason"`  // 中性:"stop" | "tool_calls" | "length"
}

// Chatter 是 Phase 8 起的模型生成契约(多轮、原生工具调用)。
// 取代 Generate 的"单次文本补全"形态;Provider/Generate 为 0-7 保留路径,标注废弃但不删。
type Chatter interface {
    Model() string
    Chat(ctx context.Context, req ChatRequest) (ChatResponse, error)
}
```

构造助手(实现细节):`Sys(text)` / `User(text)` / `AssistantText(text)` / `AssistantWithTools(toolCalls)` / `ToolResult(callID, output)`。`Arguments` 与 `json.RawMessage` 互转助手:进模型给字符串、Go 侧解析用 raw。

### 3.3 请求/响应组装要点(实现规格)

| 面 | 规则 |
|---|---|
| URL | `{TrimRight(base,"/")}/v1/chat/completions`(与 `fetchModels` 的 `{base}/v1/models` 同根约定) |
| 鉴权 | `Authorization: Bearer <key>`;空 key 不发该头(本地无鉴权网关) |
| system | 首条 `{role:"system", content}` |
| tools | 无则不发;发则 `[{type:"function", function:{name,description,parameters}}]` |
| assistant 工具轮 | `Content` 空 + `ToolCalls` 原样(含上轮 id),进历史 |
| tool 回填 | 每条独立 `{role:"tool", tool_call_id, content}`;一条 tool_calls 的多条结果按序跟随 |
| 超时 | 构造时 `http.Client{Timeout: 5 * time.Minute}` 默认,可注入(测试用短超时);`NewRequestWithContext` 尊重 ctx |
| 错误 | 非 2xx:读响应体截 200 字符,报 `fmt.Errorf("%s -> HTTP %d: %s", url, code, excerpt)`(与 models 拉取同风格) |

### 3.4 中性收敛信号(8.2 依赖)

`FinishReason` 归一:`tool_calls`(原生)`→ "tool_calls"`;`stop` → `"stop"`;`length` → `"length"`(截断,8.2 起按阶段兜底);`content_filter` 视同 `"stop"`(内容过滤罕见,先记日志不断言)。**收敛判据**:有 `ToolCalls` 即未收敛(执行回填后继续),否则以 `Content` 收敛。

## 四、文件落地清单(8.1)

全部在 `internal/provider/`,纯新增(一处标注):

| 文件 | 内容要点 |
|---|---|
| `messages.go`(新) | §3.2 全部类型 + 消息构造助手 + `Arguments`/raw 互转 |
| `chatter.go`(新) | `Chatter` 接口 + 设计说明头注(取代 Generate、0-7 兼容口径、网关单方言) |
| `chatstub.go`(新) | 确定性 `ChatStub`:构造参数定 canned 文本 / canned 工具序列(先吐 ToolCalls 再吐文本)+ 错误注入;同输入同输出(8.2 scripted tool 回合的假后端) |
| `openai.go`(新) | `NewOpenAI(base, key, model string, opts…)` + `Chat`:`{base}/v1/chat/completions`,`Bearer`,§3.3 组装;`Content`/`ToolCalls`/`FinishReason` 归一 |
| `claudecli.go`(改) | 头注标 `Deprecated`(legacy 文本后端,0-7 保留;模型文本/工具走网关,claude Code agent 委派属 8.5)——代码零改动 |
| `openai_test.go`(新) | §五 用例 1-3 |
| `chatstub_test.go`(新) | §五 用例 4 |
| `mapping_test.go`(新) | §五 用例 5 |

依赖:`net/http`/`encoding/json`/`context`/`fmt`/`strings`/`time`,无 go.mod 变更、无 sqlc、无迁移。

## 五、httptest 用例清单(验收凭证)

假网关 = `httptest.NewServer`,捕获 method/path/header/body 断言(沿用 notify_test.go 风格)。服务器内按调用序号切换应答,模拟"先 tool_calls、回填后收尾"两轮。

| # | 用例 | 断言 |
|---|---|---|
| 1 | 请求形状(文本) | POST `{base}/v1/chat/completions`;头 `Authorization: Bearer <key>`、`content-type: application/json`;body:`model`/首条 system/`max_tokens` 存在或省略按 §3.3;返回 `content` + `finish_reason:"stop"` → `Content` 一致、`FinishReason=stop`、`ToolCalls` 空 |
| 2 | tool 回传(两轮) | 一轮应答 `message.tool_calls:[{id,type:"function",function:{name,arguments}}]` + `finish_reason:"tool_calls"` → `ToolCalls` 带 ID/Name/Arguments、`FinishReason=tool_calls`;携 assistant 工具轮 + `role=tool` 回填再 Chat → 假网关断言第二请求 body:assistant 消息原样 + 逐条 `{role:"tool", tool_call_id:<id>, content:<输出>}` → 收 text `stop` |
| 3 | 错误路径 | 401 + 错误体 → error 含 `HTTP 401` 与体摘要;`Bearer` 头仍发出;空 key 不发鉴权头 |
| 4 | ChatStub 确定性 | 同输入两次调用逐字节相同;canned tool_calls→再 canned text 序列可复现;错误注入路径 |
| 5 | 映射对拍(表驱动) | 网关原生 JSON 样例(含并行 tool_calls、空 content、`arguments` 转义)→ 解码到中性类型关键字段无损;反向构造请求体一致 |

验收口径:
1. `go build ./...`、`go test ./...` 全绿;**0-7 测试零改动**(8.1 只新增 provider 文件 + claudecli 注释)。
2. provider 包新增测试全绿;`go vet ./internal/provider` 干净。
3. 无迁移文件落地、无 go.mod 变更、service/endpoint 包零改动。
4. `FetchEndpointModels` 拉网关目录 → 池子模型可见(6.1 既有能力,回归验证一遍即可,不改)。

## 六、风险与取舍

- **契约耦合网关单方言** → 有意为之:自建网关是唯一外部依赖,厂商差异归它;OS 层不再付双厂商抽象税。若将来绕过网关直连 anthropic/openai,再增补方言层,8.1 不预支。
- **底层模型工具能力参差** → 网关能透传 function calling,但个别模型可能不支持/质量差;由 8.4 档位默认(writer=cheap 由 frontier review 兜底)与显式选模型规避,不在 8.1 判能力。
- **`length` 截断** → 8.1 只如实上报 `FinishReason=length`,8.2 起由阶段兜底(重试/降请求),不静默当成功。
- **空 key 本地网关** → 不发鉴权头,与现有 models 拉取一致。
- **8.1 新网关适配暂不被生产调用** → 有意为之:live 路由切换与首个 tool 闭环同属 8.2,8.1 的 httptest 独立证明契约正确,避免"没被用到的代码"与"改了没测的路径"两个风险同时出现。

## 七、留给 8.2 的接线点(备忘,不在 8.1 做)

- service 层 `modelCall` 由 `ClaudeCLI.Generate` 切到网关 Chat:endpoint 行(BaseURL/token/selected_model,proto=openai)→ `NewOpenAI(...).Chat`;纯文本请求 = 不带 tools 的单轮 Chat。
- writer/test 之一先跑"模型调工具 → Go 执行 → 回填 → 收敛"真实 loop;`ChatStub` 充当 scripted 假后端。
- 收敛信号结构化替换对应正则(§二 #6);`FinishReason=length` 的兜底策略随阶段落地。
- claude Code agent 委派(8.5)与网关模型调用正交:委派工具在回合内被模型当作一个 function tool 调用,Go 在任务 workspace 起 claude agent 执行并回传。
