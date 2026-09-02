package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/glacierzzz26/one-person-company-os/internal/task"
)

// Planner 拆解(Phase 6.4)。Engine Driver 在整包工程请求认领后先过一次 planner:
//
//	direct → 请求本身就是原子项,落入既有 写→测→审 回合机(行为同 6.2)。
//	split  → 拆成 ≤8 条真实子任务(挂 parent_task_id),Driver 同步驱动全部完成后聚合父任务。
//	ask    → 请求需 >8 或边界不清 → 转人工审批(requestApproval),无 bypass 开关。
//
// 决策模型:OS_ENGINE_MODE=scripted → OS_SCRIPT_PLAN 确定性(离线冒烟);
// live → 公司 planner/pool 端点真实提问(fail-closed)。输出 JSON 契约:
//
//	{"action":"direct"}
//	{"action":"split","subtasks":[{"title":"...","description":"..."}, ...]}  // 1..8
//	{"action":"ask","reason":"..."}
//
// 拆解深度有界:planner 只对顶层请求(parent_task_id 为空)做拆解,子任务直接回合机 ——
// 防止递归拆解失控(design §10 6.4:子任务即执行单元)。
const engPlanCap = 8 // 自动拆解上限;超过 → ask_human

const (
	planActionDirect = "direct"
	planActionSplit  = "split"
	planActionAsk    = "ask"
)

type engPlan struct {
	action   string
	subtasks []engSubtask
	reason   string
}

type engSubtask struct {
	Title       string
	Description string
}

// planCall 对一次整包工程请求作出拆解决策。返回 engPlan,action ∈ direct/split/ask。
func (s *Service) planCall(ctx context.Context, t task.Task) (engPlan, error) {
	if strings.EqualFold(os.Getenv("OS_ENGINE_MODE"), "scripted") {
		return engScriptedPlan(t), nil
	}
	e, err := s.intakeEndpoint(ctx, t.CompanyID)
	if err != nil {
		return engPlan{}, err
	}
	out, err := s.modelCall(ctx, e, planPrompt(t))
	if err != nil {
		return engPlan{}, fmt.Errorf("planner model: %w", err)
	}
	plan, err := parsePlan(out)
	if err != nil {
		return engPlan{}, fmt.Errorf("cannot parse planner output: %w (output: %s)", err, firstLine(out))
	}
	return plan, nil
}

func planPrompt(t task.Task) string {
	return fmt.Sprintf("You are the R&D planner for a one-person company.\n"+
		"An engineering request has arrived and must be turned into implementable work:\n"+
		"Title: %s\nDescription: %s\n\n"+
		"Decide how to deliver it. Reply with EXACTLY ONE JSON object, no prose, no code fence:\n"+
		"  {\"action\":\"direct\"}                                  — well-scoped atomic item\n"+
		"  {\"action\":\"split\",\"subtasks\":[{\"title\":\"...\",\"description\":\"...\"}, ...]}  — decomposes into 1..%d concrete subtasks\n"+
		"  {\"action\":\"ask\",\"reason\":\"...\"}                   — needs more than %d subtasks, or too vague / needs a human decision\n"+
		"Rules: subtask titles non-empty and actionable; subtasks must be the real work units that a coding engineer executes one by one.",
		t.Title, strings.TrimSpace(t.Description), engPlanCap, engPlanCap)
}

// engScriptedPlan 是离线确定性 planner:OS_SCRIPT_PLAN
// "" / direct(默认) | split[:N](默认 3,N>cap → ask) | ask[:reason]。
func engScriptedPlan(t task.Task) engPlan {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("OS_SCRIPT_PLAN")))
	switch {
	case v == "", v == planActionDirect:
		return engPlan{action: planActionDirect}
	case v == planActionAsk:
		return engPlan{action: planActionAsk, reason: "planner: human decision requested (scripted)"}
	case strings.HasPrefix(v, "ask:"):
		return engPlan{action: planActionAsk, reason: strings.TrimSpace(v[len("ask:"):])}
	case v == planActionSplit, strings.HasPrefix(v, "split:"):
		n := 3
		if strings.HasPrefix(v, "split:") {
			if k, err := parsePlanN(v[len("split:"):]); err == nil {
				n = k
			}
		}
		if n > engPlanCap {
			return engPlan{action: planActionAsk,
				reason: fmt.Sprintf("planner: request needs %d subtasks (> cap %d): human decide scope", n, engPlanCap)}
		}
		if n < 1 {
			n = 1
		}
		subs := make([]engSubtask, 0, n)
		for i := 1; i <= n; i++ {
			subs = append(subs, engSubtask{
				Title:       fmt.Sprintf("%s [part %d/%d]", strings.TrimSpace(t.Title), i, n),
				Description: fmt.Sprintf("Subtask %d of %d for %q: %s", i, n, t.Title, strings.TrimSpace(t.Description)),
			})
		}
		return engPlan{action: planActionSplit, subtasks: subs}
	default:
		return engPlan{action: planActionDirect}
	}
}

// parsePlanN 解析 "N" 整数(OS_SCRIPT_PLAN=split:N 用);非数 → error。
func parsePlanN(v string) (int, error) {
	var n int
	if _, err := fmt.Sscanf(strings.TrimSpace(v), "%d", &n); err != nil {
		return 0, err
	}
	return n, nil
}

var planActionRe = regexp.MustCompile(`(?i)"action"\s*:\s*"(direct|split|ask)"|(?i)\bACTION\s*[:=]\s*(direct|split|ask)\b`)

// parsePlan 容错解析 planner JSON 输出:先去 ``` 围栏,JSON 解析优先;失败再用正则兜底。
// 校验:split 无子任务/子任务缺标题 → 转 ask(不能凭空拆);超出 cap → ask。
func parsePlan(out string) (engPlan, error) {
	raw := strings.TrimSpace(out)
	// 去掉可能的 ```json ... ``` 围栏
	if i := strings.Index(raw, "```"); i >= 0 {
		j := strings.Index(raw[i+3:], "```")
		if j >= 0 {
			raw = strings.TrimSpace(raw[i+3 : i+3+j])
		}
	}
	raw = strings.Trim(raw, "` \t\r\n")
	if raw == "" {
		return engPlan{}, fmt.Errorf("empty planner output")
	}

	var j struct {
		Action   string `json:"action"`
		Subtasks []struct {
			Title       string `json:"title"`
			Description string `json:"description"`
		} `json:"subtasks"`
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal([]byte(raw), &j); err == nil && j.Action != "" {
		subs := make([]engSubtask, 0, len(j.Subtasks))
		for _, x := range j.Subtasks {
			subs = append(subs, engSubtask{Title: strings.TrimSpace(x.Title), Description: strings.TrimSpace(x.Description)})
		}
		return validatePlan(j.Action, j.Reason, subs), nil
	}

	// 正则兜底(仅 action 可判;split 无子任务规格 → 不能自动拆,转 ask)
	m := planActionRe.FindStringSubmatch(raw)
	if m == nil {
		return engPlan{}, fmt.Errorf("no ACTION found")
	}
	action := strings.ToLower(m[1])
	if action == "" {
		action = strings.ToLower(m[2])
	}
	if action == planActionSplit {
		return engPlan{action: planActionAsk,
			reason: "planner: split without concrete subtask list — human decide scope"}, nil
	}
	if action == planActionAsk {
		return engPlan{action: planActionAsk, reason: "planner: human decision requested"}, nil
	}
	return engPlan{action: planActionDirect}, nil
}

// validatePlan 对 action + 子任务做一致性校验,返回规范化 engPlan。
func validatePlan(action, reason string, subs []engSubtask) engPlan {
	switch strings.ToLower(action) {
	case planActionDirect:
		return engPlan{action: planActionDirect}
	case planActionAsk:
		if reason == "" {
			reason = "planner: human decision requested"
		}
		return engPlan{action: planActionAsk, reason: reason}
	case planActionSplit:
		if len(subs) == 0 {
			return engPlan{action: planActionAsk, reason: "planner: split with no subtasks"}
		}
		if len(subs) > engPlanCap {
			return engPlan{action: planActionAsk,
				reason: fmt.Sprintf("planner: request needs %d subtasks (> cap %d): human decide scope", len(subs), engPlanCap)}
		}
		for _, x := range subs {
			if x.Title == "" {
				return engPlan{action: planActionAsk, reason: "planner: subtask missing title — human decide scope"}
			}
		}
		return engPlan{action: planActionSplit, subtasks: subs}
	default:
		return engPlan{action: planActionAsk, reason: fmt.Sprintf("planner: unknown action %q", action)}
	}
}
