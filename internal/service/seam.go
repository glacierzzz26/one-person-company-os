package service

// env 测试 seam 反向门控(Phase 9.4,契约 cli-readonly.md §3.1,决策①)。
// envSeam 默认 false = 产品语义:产品进程(cmd/os 构造链,cli/root.go PersistentPreRunE 建 svc)恒不读
// OS_* 配置 env,纯 DB(company→global→默认)解析 —— 结构性兑现方向 config-governance.md §八.1
// 「除 --db/--config 外无任何产品配置经 env 注入」。仅 go test 在 TestMain 经 SetEnvSeam(true) 打开,
// 保留既有确定性 seam(600+ 用例零改造,方向 §五 D3)。非测试代码不得调用本函数(产品恒关)。
var envSeam = false

// SetEnvSeam 开关测试 seam 的 env 读取(runtime.go engineScripted/agentCLI/issueSourceFor、
// delegate.go agentCLIFromEnv 的 env 分支都收在 envSeam 门内)。
func SetEnvSeam(on bool) { envSeam = on }
