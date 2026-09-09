package service

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Phase 10.6 — OS 机械扩 build·test(契约 docs/phase10/design/os-mechanical-verify.md)。
// os-accept 相位可在合成计划里显式声明 verify(go-build|go-test),OS 在验收时真跑该封闭命令,
// 作为 D4 的"真正裁判"(方向 declarative-pipelines.md §六#1:真裁判 = go test/build 跑一遍)。
// 触发唯一路径 = upfront 计划里 allocator=os 的 accept 相位携带 verify 标记行;默认 adaptive(grow)
// 与无 verify 标记的合成相位永不真跑 go ⟹ 600+ 用例零 body 改、真实 go 只在显式声明时被 OS 执行。
// 环境策略零改动(Q3 定稿门):命令继承宿主 go env 与 module cache,缺依赖/私有模块拉不到 → 如实失败。
// 残留对账(Q4):verify 只回收 pre→post 期间新出现的 untracked;改动 tracked → verify 判 fail。

const (
	// verifyMax OS 机械 verify 命令单次时间预算封顶(镜像既有 push 30s / pull 20s / delegate 20m 封顶惯例)。
	verifyMax = 5 * time.Minute
	// synthVerifyPrefix phase.note 的 verify 标记行前缀(与 acceptance/output 同族;读回 phaseVerify)。
	synthVerifyPrefix = "verify: "
)

// cmdRunner 执行一条宿主命令(OS 机械 verify 的唯一执行 seam;仿 delegator/prPub 注入范式)。
// Run 合并 stdout+stderr 返回;ctx 超时/命令失败 → 非 nil err。真实实现 realCmdRunner;测试注入 fake。
type cmdRunner interface {
	Run(ctx context.Context, dir, name string, args ...string) (string, error)
}

// realCmdRunner 默认命令执行器:继承宿主 env(不改网络策略 Q3),cwd=dir,stdout+stderr 合并。
type realCmdRunner struct{}

func (realCmdRunner) Run(ctx context.Context, dir, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return out.String(), err
	}
	return out.String(), nil
}

// verifyCommand 一条封闭 verify 命令(cmd + 固定 argv;无 shell、无模型注入 argv)。
type verifyCommand struct {
	cmd  string
	args []string
}

// verifyCommands OS 机械 verify 封闭命令集(Q2 定稿门)。go-test 隐含全量编译。新增键必须同步
// verifyCommandKeys(词表即接口:计划生成端 + 校验闭口),并把 go.mod 依赖语义写进契约 os-mechanical-verify.md。
var verifyCommands = map[string]verifyCommand{
	"go-build": {cmd: "go", args: []string{"build", "./..."}},
	"go-test":  {cmd: "go", args: []string{"test", "./..."}},
}

// verifyCommandKeys 返回允许的 verify 键(排序;校验词表与错误指引用)。
func verifyCommandKeys() []string {
	keys := make([]string, 0, len(verifyCommands))
	for k := range verifyCommands {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// verifyIsKey 键是否在封闭词表内(validateSynthPlan 词表校验)。
func verifyIsKey(k string) bool {
	_, ok := verifyCommands[k]
	return ok
}

// verifyArgv 解析键为可执行 + argv;未知 → ok=false(validate 应已拒;防御不静默跳过)。
func verifyArgv(key string) (string, []string, bool) {
	v, ok := verifyCommands[key]
	if !ok {
		return "", nil, false
	}
	return v.cmd, v.args, true
}

// runMechanicalVerify 执行一次 OS 机械 verify(Q1/Q3/Q4;契约 §3.3):
//  1. git 门(workspace 须 git;do 相位已 commit,防御);
//  2. go.mod 门(<ws>/go.mod 须存在 → 否则立即 fail,**不调 runner**;do 返工可把模块建出来 → 重 verify 过);
//  3. porcelain pre 快照(do 已 commit,通常净);
//  4. 封闭命令执行(s.runner;nil → realCmdRunner;verifyMax 预算;env 继承);
//  5. post−pre 对账(仅回收新增 untracked;改动 tracked → fail,保下相位 delegateBaseline 净起点);
//  6. 判定/证据单行:exit 0 → PASS(+回收计数);非零/命令错 → FAIL(输出首非空行);超时 → TIMEOUT。
//
// 返回 (pass, detail);detail 单行(evidence 上限 120 由 lgFinish 截;首段即有用信息),折回既有返工机。
func (s *Service) runMechanicalVerify(ctx context.Context, ws, key string) (bool, string) {
	if !wsIsGit(ctx, ws) {
		return false, fmt.Sprintf("verify %s: workspace is not a git repo", key)
	}
	if _, err := os.Stat(filepath.Join(ws, "go.mod")); err != nil {
		return false, fmt.Sprintf("verify %s: no go.mod in workspace (verify expects a Go module)", key)
	}
	cmd, argv, ok := verifyArgv(key)
	if !ok {
		return false, fmt.Sprintf("verify %s: unknown verify command", key)
	}
	pre, err := gitPorcelainSet(ctx, ws)
	if err != nil {
		return false, fmt.Sprintf("verify %s: porcelain snapshot: %s", key, firstLine(err.Error()))
	}
	runner := s.runner
	if runner == nil {
		runner = realCmdRunner{}
	}
	start := time.Now()
	vctx, cancel := context.WithTimeout(ctx, verifyMax)
	defer cancel()
	out, rerr := runner.Run(vctx, ws, cmd, argv...)
	elapsed := time.Since(start)

	dirtied, cleaned, rcErr := verifyReconcile(ctx, ws, pre)
	if rcErr != nil {
		return false, fmt.Sprintf("verify %s: reconcile: %s", key, firstLine(rcErr.Error()))
	}
	if dirtied != "" {
		return false, dirtied
	}
	if rerr != nil {
		if vctx.Err() == context.DeadlineExceeded {
			return false, fmt.Sprintf("verify %s: TIMEOUT (%.1fs)", key, elapsed.Seconds())
		}
		if line := firstNonEmptyLine(out); line != "" {
			return false, fmt.Sprintf("verify %s: FAIL: %s", key, line)
		}
		return false, fmt.Sprintf("verify %s: FAIL: %s", key, firstLine(rerr.Error()))
	}
	detail := fmt.Sprintf("verify %s: PASS (exit 0, %.1fs)", key, elapsed.Seconds())
	if cleaned > 0 {
		detail += fmt.Sprintf("; verify residue cleaned=%d", cleaned)
	}
	return true, detail
}

// verifyReconcile post−pre porcelain 对账(Q4):pre 已有/已提交行绝不碰;
//   - post 新增 `??` untracked → os.RemoveAll 回收(仅这批新出现路径;删失败 → err);
//   - post 新增 tracked 改动/删除(非 ?? 状态行)→ 返回首个 dirtied 描述(verify 判 fail,不自动回滚)。
func verifyReconcile(ctx context.Context, ws string, pre map[string]bool) (dirtied string, cleaned int, err error) {
	post, err := gitPorcelainSet(ctx, ws)
	if err != nil {
		return "", 0, err
	}
	var added []string
	for line := range post {
		if !pre[line] {
			added = append(added, line)
		}
	}
	sort.Strings(added)
	for _, line := range added {
		if strings.HasPrefix(line, "?? ") {
			p := porcelainPath(line)
			if rmErr := os.RemoveAll(filepath.Join(ws, p)); rmErr != nil {
				return "", cleaned, fmt.Errorf("remove verify residue %s: %w", p, rmErr)
			}
			cleaned++
			continue
		}
		return "verify dirtied tracked file: " + porcelainTrackedPath(line), cleaned, nil
	}
	return "", cleaned, nil
}

// firstNonEmptyLine 输出首非空行(verify FAIL 回显挑它;go-test 首个 --- FAIL / 编译错、go-build 首行编译错)。
func firstNonEmptyLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if t := strings.TrimSpace(line); t != "" {
			return t
		}
	}
	return ""
}

// porcelainTrackedPath 从 trim 后的非 ?? porcelain 行抽被改动/删除的相对路径。porcelain v1 行形为
// "XY path"(X=index 态可空格;gitPorcelainSet trim 掉前导空格 → 状态段余 1–2 字符),逐状态字符后切出路径;
// 重命名形 "R  old -> new" 取 old(被改对象)。仅作可读 fail 消息,容错保守。
func porcelainTrackedPath(line string) string {
	line = strings.TrimSpace(line)
	s := 0
	for s < len(line) && strings.ContainsRune("MADRCU", rune(line[s])) {
		s++
	}
	if s == 0 { // 无状态字符(理论不达)→ 保守退化为整行
		return line
	}
	rest := strings.TrimLeft(line[s:], " ")
	if i := strings.Index(rest, " -> "); i >= 0 {
		rest = rest[:i]
	}
	return rest
}
