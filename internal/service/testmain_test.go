package service

// Phase 9.4(契约 cli-readonly.md §3.1,决策① env seam 反向门控)——本包测试在 TestMain 打开
// service + endpoint 两 seam:service 侧 engineScripted/agentCLI/issueSourceFor/agentCLIFromEnv 读 env;
// endpoint 侧 envKey 读 OS_ENDPOINT_KEY(delegate_test:120、settings_test 以 env 造旧 key 端点)。
// 产品(cmd/os)恒关(默认 false),不在此列。

import (
	"os"
	"testing"

	"github.com/glacierzzz26/one-person-company-os/internal/endpoint"
)

func TestMain(m *testing.M) {
	endpoint.SetEnvSeam(true)
	SetEnvSeam(true)
	os.Exit(m.Run())
}
