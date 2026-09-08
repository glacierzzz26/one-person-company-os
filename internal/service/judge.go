package service

import (
	"encoding/json"
	"strings"
)

// 判读结构化(Phase 8.3,A)。各判读角色(test/review/planner/triage)输出**恰好一个 JSON 对象**,
// 解析层统一收在这里:JSON 主契约 + 旧单行标记兜底(防模型不守约,不是执行通道 —— 执行语义不依赖正则)。
// 每角色 validator 语义与 8.2/6.x 逐字对齐(approve/needs_changes、pass/失败、disposition 集合)。

// decodeJudgeJSON 尝试把模型输出解成结构化信号:
//  1. 去掉 ``` 围栏/前后缀,直接 json.Unmarshal;
//  2. 失败 → 截出首个 { 到末个 } 的子串再解一次(容忍模型夹带 prose);
//
// 返回 true 仅当成功解出合法 JSON。字段校验(如 test 的 "pass" 键必须在)由调用方做。
func decodeJudgeJSON(out string, v any) bool {
	raw := strings.TrimSpace(out)
	if i := strings.Index(raw, "```"); i >= 0 {
		rest := raw[i+3:]
		rest = strings.TrimPrefix(rest, "json\n")
		rest = strings.TrimPrefix(rest, "json\r\n")
		if j := strings.Index(rest, "```"); j >= 0 {
			rest = rest[:j]
		}
		raw = strings.TrimSpace(rest)
	}
	raw = strings.Trim(raw, "` \t\r\n")
	if raw == "" {
		return false
	}
	if err := json.Unmarshal([]byte(raw), v); err == nil {
		return true
	}
	if i := strings.IndexByte(raw, '{'); i >= 0 {
		if j := strings.LastIndexByte(raw, '}'); j > i {
			if err := json.Unmarshal([]byte(raw[i:j+1]), v); err == nil {
				return true
			}
		}
	}
	return false
}

// ---- test 判读 ----

// parseTest 判定测试是否通过 + 失败摘要(summary 供返工喂 writer)。
// JSON 主:`{"pass":true,"summary":".."}` / `{"pass":false,"summary":"<失败摘要>"}`;
// 旧标记兜底:显式 TEST OK/PASS → 过;显式 TEST FAIL/FAIL/ERROR → 不过;两者皆无 → 过(8.2 启发式)。
func parseTest(out string) (pass bool, summary string) {
	var m map[string]json.RawMessage
	if decodeJudgeJSON(out, &m) {
		if raw, ok := m["pass"]; ok {
			var p bool
			if err := json.Unmarshal(raw, &p); err == nil {
				if s, ok := m["summary"]; ok {
					_ = json.Unmarshal(s, &summary)
				}
				return p, strings.TrimSpace(summary)
			}
		}
	}
	return legacyTest(out)
}

// legacyTest 单行标记兜底(8.2 parseTestPass 语义,紧凑包含扫描,非正则)。
func legacyTest(out string) (bool, string) {
	low := strings.ToLower(out)
	for _, ok := range []string{"test ok", "test pass", "test passed", "tests pass", "all tests pass"} {
		if strings.Contains(low, ok) {
			return true, firstLine(strings.TrimSpace(out))
		}
	}
	for _, fail := range []string{"test fail", "tests fail", "test failure", "failed", "failure", "error"} {
		if strings.Contains(low, fail) {
			return false, firstLine(strings.TrimSpace(out))
		}
	}
	return true, "" // 无失败信号 → 过(与 8.2 parseTestPass 一致)
}

// ---- review 判读 ----

// parseReview 提取裁决 + 理由。JSON 主:`{"verdict":"approve"}` /
// `{"verdict":"needs_changes","reason":"<理由>"}`;找不到可判裁决 → ("","")。
func parseReview(out string) (verdict, reason string) {
	var m map[string]json.RawMessage
	if decodeJudgeJSON(out, &m) {
		if raw, ok := m["verdict"]; ok {
			var v string
			if err := json.Unmarshal(raw, &v); err == nil {
				if norm := normalizeReviewVerdict(v); norm != "" {
					if r, ok := m["reason"]; ok {
						_ = json.Unmarshal(r, &reason)
					}
					return norm, strings.TrimSpace(reason)
				}
				return "", ""
			}
		}
	}
	return legacyReview(out)
}

// normalizeReviewVerdict 把裁决归一为 approve / needs_changes;未知 → ""。
func normalizeReviewVerdict(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "approve", "approved":
		return "approve"
	case "needs_changes", "changes", "reject", "rejected":
		return "needs_changes"
	}
	return ""
}

// legacyReview 单行标记兜底("VERDICT[:=] approve|needs_changes[:<reason>]")。
func legacyReview(out string) (string, string) {
	val, note := findKeyVal(out, "VERDICT")
	if val == "" {
		return "", ""
	}
	return normalizeReviewVerdict(val), note
}

// ---- patrol 判读(Phase 10.2;D4:只对 OS 读回的巡检报告证据文本裁决,不信 agent 自述) ----

// patrolVerdict 巡检裁决契约(3.5):{"ok":bool,"severity":"low|medium|high","action":"none"|"fix",
// "summary":"..","findings":[..]}。ok=true → 绿;非 ok 才看 severity/action(3.6 处置链)。
type patrolVerdict struct {
	Ok       bool     `json:"ok"`
	Severity string   `json:"severity"`
	Action   string   `json:"action"`
	Summary  string   `json:"summary"`
	Findings []string `json:"findings"`
}

// parsePatrolVerdict 提取巡检裁决。JSON 主契约;无 ok 键 / 整体不可解析 → ok=false(调用方 engFail,不默认绿)。
// severity 归一 low|medium|high(缺省 low);action 归一 none|fix(缺省 none);summary/findings 原样。
func parsePatrolVerdict(out string) (patrolVerdict, bool) {
	var m map[string]json.RawMessage
	if !decodeJudgeJSON(out, &m) {
		return patrolVerdict{}, false
	}
	raw, hasOK := m["ok"]
	if !hasOK {
		return patrolVerdict{}, false
	}
	var ok bool
	if err := json.Unmarshal(raw, &ok); err != nil {
		return patrolVerdict{}, false
	}
	v := patrolVerdict{Ok: ok, Severity: "low", Action: "none"}
	if s, ok := m["severity"]; ok {
		var x string
		if err := json.Unmarshal(s, &x); err == nil {
			v.Severity = normalizePatrolSeverity(x)
		}
	}
	if a, ok := m["action"]; ok {
		var x string
		if err := json.Unmarshal(a, &x); err == nil {
			v.Action = normalizePatrolAction(x)
		}
	}
	if s, ok := m["summary"]; ok {
		_ = json.Unmarshal(s, &v.Summary)
	}
	if f, ok := m["findings"]; ok {
		_ = json.Unmarshal(f, &v.Findings)
	}
	v.Summary = strings.TrimSpace(v.Summary)
	if v.Findings == nil {
		v.Findings = []string{}
	}
	return v, true
}

// normalizePatrolSeverity 归一巡检严重度;未知 → low(护栏,不因模型乱填触发高险处置)。
func normalizePatrolSeverity(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "low", "medium", "high":
		return strings.ToLower(strings.TrimSpace(s))
	}
	return "low"
}

// normalizePatrolAction 归一处置动作;未知 → none(护栏,链拉仅在显式 fix 且 severity=high)。
func normalizePatrolAction(a string) string {
	switch strings.ToLower(strings.TrimSpace(a)) {
	case "fix", "none":
		return strings.ToLower(strings.TrimSpace(a))
	}
	return "none"
}

// ---- triage 判读 ----

// parseDisposition 提取 issue 处置 + 附注。JSON 主:`{"disposition":"direct_work|ask|skip|merge","note":".."}`;
// 旧标记兜底:"DISPOSITION[:=] <disposition>[:<note>]"。
func parseDisposition(out string) (string, string) {
	var m map[string]json.RawMessage
	if decodeJudgeJSON(out, &m) {
		if raw, ok := m["disposition"]; ok {
			var d string
			if err := json.Unmarshal(raw, &d); err == nil {
				if norm := normalizeDisposition(d); norm != "" {
					var note string
					if n, ok := m["note"]; ok {
						_ = json.Unmarshal(n, &note)
					}
					return norm, strings.TrimSpace(note)
				}
				return "", ""
			}
		}
	}
	val, note := findKeyVal(out, "DISPOSITION")
	if val == "" {
		return "", ""
	}
	return normalizeDisposition(val), note
}

// normalizeDisposition 归一处置;未知 → ""。
func normalizeDisposition(d string) string {
	v := strings.ToLower(strings.TrimSpace(d))
	switch v {
	case engDispDirectWork, engDispAsk, engDispSkip, engDispMerge:
		return v
	}
	return ""
}

// ---- 旧标记通用定位 ----

// findKeyVal 在输出中找形如 "<key>[:= ]<value>[:<note>]" 的首行标记(全小写匹配)。
// 仅兜底用;value 取标记后首个 ':' 或空白前的 token,note 取其后冒号内容。
func findKeyVal(out, key string) (value, note string) {
	low := strings.ToLower(out)
	k := strings.ToLower(key)
	for _, line := range strings.Split(low, "\n") {
		i := strings.Index(line, k)
		if i < 0 {
			continue
		}
		rest := strings.TrimLeft(line[i+len(k):], " \t")
		rest = strings.TrimPrefix(rest, ":")
		rest = strings.TrimPrefix(rest, "=")
		rest = strings.TrimSpace(rest)
		if rest == "" {
			continue
		}
		if c := strings.Index(rest, ":"); c >= 0 {
			val := strings.TrimSpace(rest[:c])
			return val, strings.TrimSpace(rest[c+1:])
		}
		if sp := strings.IndexAny(rest, " \t"); sp >= 0 {
			return strings.TrimSpace(rest[:sp]), strings.TrimSpace(rest[sp+1:])
		}
		return rest, ""
	}
	return "", ""
}
