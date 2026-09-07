package endpoint

// Phase 9.4(契约 cli-readonly.md §3.1,决策① env seam 反向门控)——seal_test 以 OS_ENDPOINT_KEY
// 造旧 key 端点,TestMain 打开本包 seam;产品(cmd/os)恒关(默认 false),不在此列。

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	SetEnvSeam(true)
	os.Exit(m.Run())
}
