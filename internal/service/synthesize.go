package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/glacierzzz26/one-person-company-os/internal/pipeline"
	"github.com/glacierzzz26/one-person-company-os/internal/plan"
	"github.com/glacierzzz26/one-person-company-os/internal/task"
	"github.com/google/uuid"
)

// Phase 10.4 — 高智运行时合成驱动(契约 docs/phase10/design/run-synthesis.md §3.3/§3.5)。
// engineering run + 流水线 plan_policy==synthesize 的**新平行驱动 runSynthesized**:
//   - 首次认领:frontier(review 槽)对意图合成 JSON 阶段计划 → 校验 → 落同一 task_plan/phase 账本
//     (materialized grow→upfront + phases 全铺 pending)→ 合成失败/不可解析 → audit eng_synth_fail
//     → **降级回 runEngineering**(计划保持 grow,自适应默认语义;零 body 改)。
//   - 先审后干:合成先于 needsApproval(requestApproval 时完整 upfront 计划已落账本);approve 复认领
//     已 upfront → 跳过合成 → 逐相位执行。do→accept 相邻成对;accept fail → 免费返工 ≤ synthMaxRework
//     (同一 do 相位重委派);仍 fail → do/accept 相位 fail → engFail(evidence 落账本)。
//   - dispose(os|judge|manual)= 轻收尾;runEngineering/runPatrol/grow 零 body 改。
// 共享仅账本 helper(lgStart/lgFinish/loadRunLedger/porcelain)与既有 engFail/审批/委派原语。

const (
	// synthPhaseCap 合成计划相位上限(越界 → 校验失败降级)。
	synthPhaseCap = 12
	// synthMaxRework 单 do/accept 对的免费返工上限(契约 §3.5:accept fail → ≤1 次重委派)。
	synthMaxRework = 1
	// synthNote 标记行前缀(phase.note 只存引用/摘要;acceptance/output 单行,供机械/判读与 Web 展示)。
	synthAcceptPrefix = "acceptance: "
	synthOutputPrefix = "output: "
)

// errSynthTerminal 相位终态哨兵:engFail 已把任务置 fail/requeue(engFail 成功返回 nil = "已处理"),
// 驱动循环据哨兵**停止**(不再跑后续相位、绝不补 CompleteTask 复活任务)。
var errSynthTerminal = errors.New("synthesized phase ended terminal (task requeued or failed)")

// engFailTerminal = engFail + 返回哨兵。engFail 失败(存储错误)→ 原样上抛;成功 → 哨兵。
func (s *Service) engFailTerminal(ctx context.Context, t task.Task, phase string, runCtx context.Context, cause error) error {
	if err := s.engFail(ctx, t, phase, runCtx, cause); err != nil {
		return err
	}
	return errSynthTerminal
}

// ---- frontier 合成计划(JSON 形态;§3.4) ----

type synthPhaseSpec struct {
	Kind       string `json:"kind"`       // do | accept | dispose
	Title      string `json:"title"`      // 人类可读阶段目标
	Allocator  string `json:"allocator"`  // do→delegate;accept→os|judge;dispose→os|judge|manual
	Acceptance string `json:"acceptance"` // 验收判据(单行;供 OS 机械允许清单 / 判读模型)
	Output     string `json:"output"`     // 期望产出相对路径(单行;os accept 必填)
}

type synthPlanSpec struct {
	Summary string           `json:"summary"`
	Phases  []synthPhaseSpec `json:"phases"`
}

// parseSynthPlan 把模型原文解成 JSON 阶段计划。不可解析 → 错误(调用方降级)。
func parseSynthPlan(out string) (synthPlanSpec, error) {
	var spec synthPlanSpec
	if !decodeJudgeJSON(out, &spec) {
		return spec, fmt.Errorf("cannot parse synthesized plan JSON from model output: %s", firstLine(out))
	}
	if len(spec.Phases) == 0 {
		return spec, fmt.Errorf("synthesized plan has no phases")
	}
	return spec, nil
}

// validateSynthPlan 校验计划语义(契约 §3.3);违规 → 错误(降级,不落半成品账本)。
//   - 相位 ≤ synthPhaseCap;至少 1 do + 1 accept;
//   - do→allocator=delegate;accept→os|judge 且 acceptance 非空;dispose→os|judge|manual;
//   - accept(os)→output 必填(允许清单落点);do 紧随 accept / accept 紧随 do(成对,返工语义唯一);
//   - acceptance/output/title 单行、output 相对路径白名单(无 .. / 空白 / 前导 /)。
func validateSynthPlan(spec synthPlanSpec) error {
	phases := spec.Phases
	if len(phases) > synthPhaseCap {
		return fmt.Errorf("synthesized plan has %d phases (cap %d)", len(phases), synthPhaseCap)
	}
	nDo, nAccept := 0, 0
	for i, sp := range phases {
		title := strings.TrimSpace(sp.Title)
		if title == "" || strings.ContainsRune(title, '\n') {
			return fmt.Errorf("phase %d: title must be a non-empty single line", i+1)
		}
		if strings.ContainsRune(sp.Acceptance, '\n') || strings.ContainsRune(sp.Output, '\n') {
			return fmt.Errorf("phase %d: acceptance/output must be single-line", i+1)
		}
		switch sp.Kind {
		case plan.PhaseKindDo:
			nDo++
			if sp.Allocator != plan.AllocatorDelegate {
				return fmt.Errorf("phase %d (do): allocator must be %q (got %q)", i+1, plan.AllocatorDelegate, sp.Allocator)
			}
			// do 必须紧跟其 accept(返工对象唯一;dispose 不插在 do/accept 之间)
			if i+1 >= len(phases) || phases[i+1].Kind != plan.PhaseKindAccept {
				return fmt.Errorf("phase %d (do): must be immediately followed by an accept phase", i+1)
			}
		case plan.PhaseKindAccept:
			nAccept++
			if i == 0 || phases[i-1].Kind != plan.PhaseKindDo {
				return fmt.Errorf("phase %d (accept): must directly follow its do phase", i+1)
			}
			if strings.TrimSpace(sp.Acceptance) == "" {
				return fmt.Errorf("phase %d (accept): acceptance criteria required", i+1)
			}
			switch sp.Allocator {
			case plan.AllocatorOS:
				if strings.TrimSpace(sp.Output) == "" || !validOutputRel(sp.Output) {
					return fmt.Errorf("phase %d (accept os): a valid relative output path is required (allowed list fallback)", i+1)
				}
			case plan.AllocatorJudge:
				// judge 判读:output 可选(判读喂 do 的 diff/产出)
			default:
				return fmt.Errorf("phase %d (accept): allocator must be os|judge (got %q)", i+1, sp.Allocator)
			}
		case plan.PhaseKindDispose:
			switch sp.Allocator {
			case plan.AllocatorOS, plan.AllocatorJudge, plan.AllocatorManual:
			default:
				return fmt.Errorf("phase %d (dispose): allocator must be os|judge|manual (got %q)", i+1, sp.Allocator)
			}
		default:
			return fmt.Errorf("phase %d: unknown kind %q (do|accept|dispose)", i+1, sp.Kind)
		}
	}
	if nDo == 0 || nAccept == 0 {
		return fmt.Errorf("synthesized plan must contain at least one do and one accept phase")
	}
	return nil
}

// validOutputRel 产出相对路径白名单(机械/文件存在性检查的确定性前提):
// 非空、非绝对、无 ..、无空白与反斜杠,仅 [A-Za-z0-9._/-]。
func validOutputRel(p string) bool {
	if p == "" || strings.HasPrefix(p, "/") || strings.Contains(p, "..") ||
		strings.ContainsAny(p, " \t\n\\") {
		return false
	}
	for _, r := range p {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') ||
			strings.ContainsRune("./-_", r)) {
			return false
		}
	}
	return true
}

// synthPhaseNote 把阶段字段落 phase.note(标记行;Web/机械/判读按行前缀读回)。
func synthPhaseNote(sp synthPhaseSpec) string {
	parts := []string{}
	if a := strings.TrimSpace(sp.Acceptance); a != "" {
		parts = append(parts, synthAcceptPrefix+a)
	}
	if o := strings.TrimSpace(sp.Output); o != "" {
		parts = append(parts, synthOutputPrefix+o)
	}
	return strings.Join(parts, "\n")
}

// phaseAcceptance 读回 phase.note 的 acceptance 判据(单行)。
func phaseAcceptance(ph plan.RunPlanPhase) string {
	for _, line := range strings.Split(ph.Note, "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), synthAcceptPrefix); ok {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// phaseOutputs 读回 phase.note 的全部 output 相对路径(单行逐条)。
func phaseOutputs(ph plan.RunPlanPhase) []string { return outputsFromNote(ph.Note) }

// ---- runSynthesized 入口(§3.3) ----

// runSynthesized 合成 run 认领驱动。首次认领合成计划(先于审批门);已合成(approve 续跑)→ 直接执行。
func (s *Service) runSynthesized(ctx context.Context, workerID string, t task.Task, pl pipeline.Pipeline) error {
	rp, ok, err := s.store.GetTaskPlanByTask(ctx, t.ID)
	if err != nil {
		return err
	}
	if !ok {
		// 非流水线 run / 历史 run:无计划 → 兜底 grow 语义(理论不进此分支)。
		return s.runEngineering(ctx, workerID, t)
	}
	if rp.Materialized == plan.MaterializedGrow {
		degraded, err := s.synthesizeOnce(ctx, t, rp)
		if err != nil {
			return err
		}
		if degraded {
			return s.runEngineering(ctx, workerID, t)
		}
	}
	// 先审后干:合成(或已有 upfront 计划)后过审批门;approve 复认领 HasApprovedApproval>0 放行。
	if need, reason, err := s.needsApproval(ctx, t); err != nil {
		return err
	} else if need {
		return s.requestApproval(ctx, t, reason)
	}
	if _, err := s.store.MarkTaskRunning(ctx, t.ID); err != nil {
		return err
	}
	if _, err := s.audit(ctx, "task", t.ID, "eng_start", taskActor(t), "synthesized plan driver"); err != nil {
		return err
	}
	runCtx, cancel := runContext(ctx, t.TimeoutSec)
	defer cancel()
	if !s.engineScripted(ctx, t.CompanyID) {
		if err := s.delegateBaseline(runCtx, t); err != nil {
			return s.engFail(ctx, t, "synth", runCtx, err)
		}
	}
	lg, err := s.loadRunLedger(ctx, t.ID)
	if err != nil {
		return err
	}
	if lg == nil {
		return s.runEngineering(ctx, workerID, t) // 防御:计划被删(不应发生)→ 兜底
	}
	return s.runSynthPhases(ctx, runCtx, workerID, t, lg)
}

// synthesizeOnce 首次认领合成(§3.3 step 2):frontier 调用 → 解析/校验 → 落 upfront 账本。
// 返回 degraded=true = 合成失败/不可解析/校验不过 → 计划保持 grow,调用方降级 runEngineering;
// degraded=false = 合成已落账本(后续先审后干)。
func (s *Service) synthesizeOnce(ctx context.Context, t task.Task, rp plan.RunPlan) (bool, error) {
	fail := func(reason string) (bool, error) {
		if _, err := s.audit(ctx, "task", t.ID, "eng_synth_fail", taskActor(t), truncate(reason, 300)); err != nil {
			return false, err
		}
		return true, nil
	}
	out, serr := s.synthCall(ctx, t)
	if serr != nil {
		return fail(serr.Error())
	}
	spec, perr := parseSynthPlan(out)
	if perr == nil {
		perr = validateSynthPlan(spec)
	}
	if perr != nil {
		return fail(perr.Error())
	}
	if err := s.materializeSynthPlan(ctx, rp, spec); err != nil {
		return false, err
	}
	if _, err := s.audit(ctx, "task", t.ID, "eng_synth", taskActor(t),
		fmt.Sprintf("phases=%d %s", len(spec.Phases), truncate(strings.TrimSpace(spec.Summary), 200))); err != nil {
		return false, err
	}
	return false, nil
}

// synthCall 合成模型调用:review 槽(frontier,承 8.4 档位;engEndpointFor review 语义)。
// scripted → OS_SCRIPT_SYNTH(envSeam 门内,产品不可达):空 → 错误(降级)。任何调用错误由调用方降级。
func (s *Service) synthCall(ctx context.Context, t task.Task) (string, error) {
	if s.engineScripted(ctx, t.CompanyID) {
		raw := strings.TrimSpace(os.Getenv("OS_SCRIPT_SYNTH"))
		if raw == "" {
			return "", fmt.Errorf("scripted synthesis not configured (OS_SCRIPT_SYNTH empty) → degrade to adaptive")
		}
		return raw, nil
	}
	epID := s.engEndpointFor(t, engRoleReview)
	if epID == "" {
		return "", fmt.Errorf("no reviewer endpoint for frontier synthesis (policy=synthesize; bind a frontier reviewer endpoint)")
	}
	e, err := s.store.GetEndpoint(ctx, epID)
	if err != nil {
		return "", fmt.Errorf("synthesis reviewer endpoint: %w", err)
	}
	if e.Status != "active" {
		return "", fmt.Errorf("synthesis reviewer endpoint %q status=%s", e.Name, e.Status)
	}
	return s.modelCall(ctx, e, synthPrompt(ctx, t))
}

// synthPrompt 合成指令:意图 + 委派边界 + workspace 只读摘要 + 输出 JSON 契约。
func synthPrompt(ctx context.Context, t task.Task) string {
	var b strings.Builder
	b.WriteString("You are the frontier planner inside a one-person-company OS. Produce a bounded, reviewable execution plan\n")
	b.WriteString("for a coding task that will run inside one git workspace. You do NOT write code yourself — you synthesize the plan.\n\n")
	fmt.Fprintf(&b, "Task: %s\n", t.Title)
	if desc := strings.TrimSpace(t.Description); desc != "" {
		fmt.Fprintf(&b, "Intent: %s\n", desc)
	}
	if ws := strings.TrimSpace(t.WorkspacePath); ws != "" {
		fmt.Fprintf(&b, "\nWorkspace digest (read-only; respect existing state):\n")
		if wsIsGit(ctx, ws) {
			if out, err := gitDirCmd(ctx, ws, "log", "-1", "--oneline"); err == nil && strings.TrimSpace(out) != "" {
				fmt.Fprintf(&b, "- git HEAD: %s\n", strings.TrimSpace(out))
			}
		}
		if ents, err := os.ReadDir(ws); err == nil {
			var names []string
			for i, e := range ents {
				if i >= 15 {
					names = append(names, "...")
					break
				}
				nm := e.Name() + "/"
				if !e.IsDir() {
					nm = e.Name()
				}
				names = append(names, nm)
			}
			sort.Strings(names)
			fmt.Fprintf(&b, "- workspace top-level: %s\n", strings.Join(names, " "))
		} else {
			b.WriteString("- (workspace digest unavailable)\n")
		}
	}
	b.WriteString("\nRules:\n- Break the intent into ordered phases: each 'do' immediately followed by its 'accept' (no other phase in between).\n")
	b.WriteString("- do: the OS delegates one coding agent into the workspace; allocator=\"delegate\".\n")
	b.WriteString("- accept: how the OS verifies that do passed. If verifiable by existence of a declared output file and no workspace residue,\n")
	b.WriteString("  allocator=\"os\" and set output to the relative artifact path. If it needs semantic judgment over the diff/report,\n")
	b.WriteString("  allocator=\"judge\". Every accept needs a one-line acceptance criteria.\n")
	b.WriteString("- dispose (optional, trailing): allocator os (final hygiene) / judge (advisory) / manual (record for a human).\n")
	b.WriteString("- Keep phases few and sequential (at most 12). Cover risk; do not include budgeting or cost.\n")
	b.WriteString("- acceptance and output must be single line; output is a relative path (letters/digits/._-/), never absolute, no spaces, no '..'.\n")
	b.WriteString("- The agent (not you) runs tests/builds inside the workspace; you must not require arbitrary host commands from the OS.\n\n")
	b.WriteString("Reply with EXACTLY ONE JSON object, no prose, no code fence, shape:\n")
	b.WriteString("  {\"summary\":\"<one-line plan summary>\",\"phases\":[\n")
	b.WriteString("    {\"kind\":\"do\",\"title\":\"...\",\"allocator\":\"delegate\",\"acceptance\":\"<what will verify it>\",\"output\":\"<rel path or leave empty>\"},\n")
	b.WriteString("    {\"kind\":\"accept\",\"title\":\"...\",\"allocator\":\"judge\"|\"os\",\"acceptance\":\"<one-line criteria>\",\"output\":\"<rel path required only when os>\"},\n")
	b.WriteString("    {\"kind\":\"dispose\",\"title\":\"...\",\"allocator\":\"os\"|\"judge\"|\"manual\",\"acceptance\":\"\"}\n")
	b.WriteString("  ]}\n")
	return b.String()
}

// materializeSynthPlan 合成落账本:grow → upfront + 全铺 pending 行(UNIQUE(plan_id,seq) 按序 1..N)。
// do 相位 note 额外合并其紧邻 accept 声明的 output(验收落点 = do 委派简报要产出的工件):
// do/accept 自成工作单元,委派与 scripted 只依赖 do 相位自身即可写出验收要查的产出。
func (s *Service) materializeSynthPlan(ctx context.Context, rp plan.RunPlan, spec synthPlanSpec) error {
	if err := s.store.SetPlanMaterialized(ctx, rp.ID, plan.MaterializedUpfront); err != nil {
		return err
	}
	for i, sp := range spec.Phases {
		note := synthPhaseNote(sp)
		if sp.Kind == plan.PhaseKindDo && i+1 < len(spec.Phases) && spec.Phases[i+1].Kind == plan.PhaseKindAccept {
			note = mergeDoOutputs(note, synthPhaseNote(spec.Phases[i+1]))
		}
		ph := plan.RunPlanPhase{
			ID: uuid.NewString(), PlanID: rp.ID, Seq: int64(i + 1),
			Kind: sp.Kind, Title: firstLine(strings.TrimSpace(sp.Title)),
			Allocator: sp.Allocator, Status: plan.PhaseStatusPending,
			Note: note,
		}
		if _, err := s.store.CreatePlanPhase(ctx, ph); err != nil {
			return err
		}
	}
	return s.store.SetPlanUpdated(ctx, rp.ID, time.Now().Unix())
}

// mergeDoOutputs 把相邻 accept 注记里的 output 行并入 do 注记(去重,保持相对路径逐行契约)。
func mergeDoOutputs(doNote, accNote string) string {
	if strings.TrimSpace(accNote) == "" {
		return doNote
	}
	seen := map[string]bool{}
	for _, rel := range outputsFromNote(doNote) {
		seen[rel] = true
	}
	var lines []string
	if strings.TrimSpace(doNote) != "" {
		lines = append(lines, strings.Split(doNote, "\n")...)
	}
	for _, rel := range outputsFromNote(accNote) {
		if !seen[rel] {
			seen[rel] = true
			lines = append(lines, synthOutputPrefix+rel)
		}
	}
	return strings.Join(lines, "\n")
}

// outputsFromNote 抽出注记中的 output 相对路径(机械存在性/写入面复用)。
func outputsFromNote(note string) []string {
	var out []string
	for _, line := range strings.Split(note, "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), synthOutputPrefix); ok {
			if p := strings.TrimSpace(v); p != "" {
				out = append(out, p)
			}
		}
	}
	return out
}

// ---- 逐相位驱动(§3.5;do/accept 成对 + dispose 收尾) ----

func sortedPhases(lg *runLedger) []plan.RunPlanPhase {
	var seqs []int64
	for seq := range lg.phases {
		seqs = append(seqs, seq)
	}
	sort.Slice(seqs, func(i, j int) bool { return seqs[i] < seqs[j] })
	out := make([]plan.RunPlanPhase, 0, len(seqs))
	for _, seq := range seqs {
		out = append(out, lg.phases[seq])
	}
	return out
}

// runSynthPhases 按账本相位序驱动;全部相位 ok → CompleteTask + audit eng_complete。
// do/accept 相位对共享单 do 运行(accept 不 ok → 免费返工 ≤1 → 仍不 ok → 相位 fail → engFail)。
func (s *Service) runSynthPhases(ctx, runCtx context.Context, workerID string, t task.Task, lg *runLedger) error {
	rows := sortedPhases(lg)
	total := len(rows)
	if total == 0 {
		// 合成后应非空(validate ≥1 do);防御空计划 → 直接终败不留静默 complete。
		return s.engFail(ctx, t, "synth", runCtx, fmt.Errorf("synthesized plan is empty"))
	}
	for i := 0; i < len(rows); {
		ph := rows[i]
		switch ph.Kind {
		case plan.PhaseKindDo:
			if i+1 >= len(rows) || rows[i+1].Kind != plan.PhaseKindAccept {
				return s.engFail(ctx, t, "synth", runCtx,
					fmt.Errorf("synthesized do phase seq=%d has no following accept (malformed ledger)", ph.Seq))
			}
			acc := rows[i+1]
			if ph.Status == plan.PhaseStatusOK && acc.Status == plan.PhaseStatusOK {
				i += 2 // 已完成对(requeue 续跑/崩溃恢复)→ 跳过不重委派
				continue
			}
			if err := s.runSynthUnit(ctx, runCtx, workerID, t, lg, ph, acc); err != nil {
				if errors.Is(err, errSynthTerminal) {
					return nil // engFail 已把任务置 fail/requeue → 停,绝不补 CompleteTask
				}
				return err
			}
			i += 2
		case plan.PhaseKindAccept:
			// 悬空 accept(validate 应拒)→ 防御终败。
			return s.engFail(ctx, t, "synth", runCtx,
				fmt.Errorf("synthesized accept phase seq=%d without preceding do (malformed ledger)", ph.Seq))
		case plan.PhaseKindDispose:
			if err := s.runSynthDispose(ctx, runCtx, t, lg, ph); err != nil {
				if errors.Is(err, errSynthTerminal) {
					return nil // 同上:engFail 已落任务态,停
				}
				return err
			}
			i++
		default:
			return s.engFail(ctx, t, "synth", runCtx, fmt.Errorf("unknown synthesized phase kind %q", ph.Kind))
		}
	}
	result := fmt.Sprintf("synth: %d phases executed", total)
	if _, err := s.store.CompleteTask(ctx, t.ID, result); err != nil {
		return err
	}
	if _, err := s.audit(ctx, "task", t.ID, "eng_complete", taskActor(t),
		fmt.Sprintf("synthesized plan driver: %d phases", total)); err != nil {
		return err
	}
	return nil
}

// runSynthUnit 执行一个 do→accept 对。do 委派产出;accept 判读(os 机械 / judge 模型);
// accept fail → 免费返工 ≤ synthMaxRework(同一 do 重委派,note 带上一判读原因);仍 fail → 相位终败 engFail。
func (s *Service) runSynthUnit(ctx, runCtx context.Context, workerID string, t task.Task, lg *runLedger, do, acc plan.RunPlanPhase) error {
	if err := s.lgStart(ctx, lg, do.Seq); err != nil {
		return err
	}
	rework := 0
	var hint string
	for {
		diff, derr := s.runSynthDo(ctx, runCtx, t, do, hint)
		if derr != nil {
			_ = s.lgFinish(ctx, lg, do.Seq, plan.PhaseStatusFail, "", derr.Error())
			return s.engFailTerminal(ctx, t, fmt.Sprintf("synth do seq=%d", do.Seq), runCtx, derr)
		}
		if err := s.lgStart(ctx, lg, acc.Seq); err != nil { // 首次评估置 running;返工 no-op
			return err
		}
		pass, reason, aerr := s.runSynthAccept(ctx, runCtx, t, acc, do, diff, rework)
		if aerr != nil {
			_ = s.lgFinish(ctx, lg, acc.Seq, plan.PhaseStatusFail, "", aerr.Error())
			_ = s.lgFinish(ctx, lg, do.Seq, plan.PhaseStatusFail, "", "acceptance unavailable: "+truncate(aerr.Error(), 120))
			return s.engFailTerminal(ctx, t, fmt.Sprintf("synth accept seq=%d", acc.Seq), runCtx, aerr)
		}
		if pass {
			if err := s.lgFinish(ctx, lg, acc.Seq, plan.PhaseStatusOK, truncate(reason, 120), ""); err != nil {
				return err
			}
			if err := s.lgFinish(ctx, lg, do.Seq, plan.PhaseStatusOK, synthDoEvidence(diff), ""); err != nil {
				return err
			}
			return nil
		}
		if rework < synthMaxRework {
			rework++
			hint = reason
			continue
		}
		// 返工用尽 → do/accept 相位终败 → 任务 engFail(evidence 落账本)。
		_ = s.lgFinish(ctx, lg, acc.Seq, plan.PhaseStatusFail, truncate(reason, 120), "")
		_ = s.lgFinish(ctx, lg, do.Seq, plan.PhaseStatusFail, "", "acceptance failed after rework: "+truncate(firstLine(reason), 120))
		return s.engFailTerminal(ctx, t, fmt.Sprintf("synth accept seq=%d", acc.Seq), runCtx,
			fmt.Errorf("acceptance failed after %d reworks: %s", rework, truncate(reason, 200)))
	}
}

// runSynthDo 执行一次 do 委派(live = 逐委派 delegate/commit,phase-scoped net diff;scripted = 写 fixture + commit)。
func (s *Service) runSynthDo(ctx, runCtx context.Context, t task.Task, do plan.RunPlanPhase, hint string) (string, error) {
	if s.engineScripted(ctx, t.CompanyID) {
		return s.scriptedSynthPhase(ctx, t, do)
	}
	return s.delegateSynthPhase(ctx, t, do, hint)
}

// delegateSynthPhase live 单相位委派(§3.5 do):锚 baseline ref → phase pre-HEAD 快照 →
// 委派(claude,简报 = 任务护栏 + 相位 title/输出契约/acceptance)→ add -A → diff --cached pre(相位净 diff)→
// audit eng_delegate(带 phase seq)→ commitDelegation(每次委派后工作树归 clean)。
func (s *Service) delegateSynthPhase(ctx context.Context, t task.Task, do plan.RunPlanPhase, hint string) (string, error) {
	ws := t.WorkspacePath
	if ws == "" || !wsIsGit(ctx, ws) {
		return "", fmt.Errorf("synthesized do phase seq=%d requires a git workspace (task workspace=%q is not a git repo)", do.Seq, ws)
	}
	if err := ensureBaseline(ctx, ws, t.ID); err != nil {
		return "", fmt.Errorf("ensure baseline: %w", err)
	}
	family, err := s.agentCLI(ctx, t.CompanyID)
	if err != nil {
		return "", fmt.Errorf("resolve agent cli: %w", err)
	}
	d, err := s.delegatorFor(family)
	if err != nil {
		return "", err
	}
	preRaw, err := gitDirCmd(ctx, ws, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	pre := strings.TrimSpace(preRaw)
	spec := DelegateSpec{
		Workspace: ws,
		Brief:     synthPhaseBrief(t, do, ws, hint),
		ModelEnv:  s.delegateEnv(ctx, t),
		Timeout:   delegateTimeout(ctx, t, engCallCtx{}),
		Family:    family,
	}
	res, err := d.Delegate(ctx, spec)
	if err != nil {
		return "", fmt.Errorf("delegate (phase seq=%d family=%s): %w", do.Seq, family, err)
	}
	diff, err := phaseCapture(ctx, ws, pre)
	if err != nil {
		return "", fmt.Errorf("capture phase diff: %w", err)
	}
	if strings.TrimSpace(diff) == "" {
		return "", fmt.Errorf("synthesized do phase seq=%d produced no workspace changes", do.Seq)
	}
	report := firstLine(strings.TrimSpace(res.Report))
	if report == "" {
		report = "(no agent self-report)"
	}
	if _, err := s.audit(ctx, "task", t.ID, "eng_delegate", taskActor(t),
		fmt.Sprintf("family=%s ref=%s phase=seq%d ws=%s diff=%dB report=%s", family, pre, do.Seq, ws, len(diff), report)); err != nil {
		return "", err
	}
	if err := commitDelegation(ctx, ws, synthPhaseCommitMsg(t, do.Seq, family)); err != nil {
		return "", err
	}
	return diff, nil
}

// phaseCapture 捕获一次相位自 pre-HEAD 的净 diff(add -A 后 diff --cached pre)。
// 委派前工作树 clean(前一相位已 commit)→ 只含本次相位改动,不累积跨相位。
func phaseCapture(ctx context.Context, ws, pre string) (string, error) {
	if _, err := gitDirCmd(ctx, ws, "add", "-A"); err != nil {
		return "", err
	}
	return gitDirCmd(ctx, ws, "diff", "--cached", pre)
}

// synthPhaseCommitMsg 逐相位委派提交的确定性 message(带 task8 + seq,历史可归属)。
func synthPhaseCommitMsg(t task.Task, seq int64, family string) string {
	return fmt.Sprintf("os-delegate: %s synth phase seq=%d family=%s", short8(t.ID), seq, family)
}

// synthDoEvidence do 相位 ok 的证据(commit/diff 摘要首行)。
func synthDoEvidence(diff string) string {
	if e := truncate(firstLine(diff), 120); e != "" {
		return e
	}
	return "(no readable diff line)"
}

// scriptedSynthPhase scripted 单相位执行(离线确定性):把该相位声明的 output 写 fixture 并 commit
// (可选 OS_SCRIPT_SYNTH_STRAY 额外写一个未跟踪文件 → 测残留);返回 canned diff 供判读/证据。
// 与 live 语义一致:产出 commit 后工作树归 clean,机械 accept 才可确定性验证。
func (s *Service) scriptedSynthPhase(ctx context.Context, t task.Task, do plan.RunPlanPhase) (string, error) {
	ws := t.WorkspacePath
	isGit := wsIsGit(ctx, ws)
	if isGit {
		_ = excludeWorkspaceTools(ctx, ws)
	}
	outputs := phaseOutputs(do)
	for _, rel := range outputs {
		full := filepath.Join(ws, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return "", fmt.Errorf("scripted synthesized output mkdir %s: %w", rel, err)
		}
		if err := os.WriteFile(full, []byte(fmt.Sprintf("# scripted synthesized output for %s\n", do.Title)), 0o644); err != nil {
			return "", fmt.Errorf("scripted synthesized output write %s: %w", rel, err)
		}
	}
	if stray := strings.TrimSpace(os.Getenv("OS_SCRIPT_SYNTH_STRAY")); stray != "" && ws != "" {
		_ = os.WriteFile(filepath.Join(ws, stray), []byte("scripted stray residue\n"), 0o644)
	}
	if isGit && len(outputs) > 0 {
		args := append([]string{"add", "--"}, outputs...)
		if _, err := gitDirCmd(ctx, ws, args...); err == nil {
			_ = commitDelegation(ctx, ws, synthPhaseCommitMsg(t, do.Seq, "scripted"))
		}
	}
	return fmt.Sprintf("PATCH\n```diff\n@@ synthesized do seq=%d title=%s\n+ fix (scripted deterministic phase output)\n```\n", do.Seq, do.Title), nil
}

// synthPhaseBrief 单相位委派简报(任务护栏 + 相位目标 + 输出契约 + acceptance;返工带上一判读原因)。
func synthPhaseBrief(t task.Task, do plan.RunPlanPhase, ws, hint string) string {
	var b strings.Builder
	b.WriteString("You are the OS-delegated coding engineer, working inside one bounded git workspace on one phase of a synthesized plan.\n\n")
	fmt.Fprintf(&b, "Task: %s\n", t.Title)
	if desc := strings.TrimSpace(t.Description); desc != "" {
		fmt.Fprintf(&b, "Description: %s\n", desc)
	}
	fmt.Fprintf(&b, "\nPhase (seq=%d): %s\n", do.Seq, do.Title)
	if acc := phaseAcceptance(do); acc != "" {
		fmt.Fprintf(&b, "Acceptance criteria this phase will be checked against: %s\n", acc)
	}
	if outs := phaseOutputs(do); len(outs) > 0 {
		b.WriteString("Declared output artifact(s) expected under this workspace:\n")
		for _, o := range outs {
			fmt.Fprintf(&b, "- %s\n", o)
		}
	}
	b.WriteString("\nBoundaries:\n")
	fmt.Fprintf(&b, "- Work only inside this workspace: %s. Only touch files directly related to this phase.\n", ws)
	b.WriteString("- Do NOT run: git commit / push / fetch / pull. The OS captures and commits your changes.\n")
	b.WriteString("- After editing, run the relevant tests/build yourself so the change is self-consistent.\n")
	b.WriteString("- Do NOT modify governance files: permission policies, approvals, audit records, or the config that gates them.\n")
	if strings.TrimSpace(hint) != "" {
		fmt.Fprintf(&b, "\nPrevious acceptance failed. Address it: %s\n", strings.TrimSpace(hint))
	}
	b.WriteString("\nEnd report must list: the files you changed; the commands you ran and their outcomes.\n")
	return b.String()
}

// ---- accept(§3.5:OS 机械只读允许清单 / judge 模型判读) ----

// runSynthAccept 对 do 产出做一次验收。allocator:
//   - os → osMechanicalAccept(只读确定性允许清单,见 §3.5 清单;每项失败给 reason);
//   - judge → review 槽判读模型(scripted OS_SCRIPT_ACCEPT),diff/产出喂模型。
//
// 返回 (ok, reason/evidence, internalErr);internalErr = 判读执行不可得(→ engFail),不是 verdict fail。
func (s *Service) runSynthAccept(ctx, runCtx context.Context, t task.Task, acc, do plan.RunPlanPhase, diff string, rework int) (bool, string, error) {
	switch acc.Allocator {
	case plan.AllocatorOS:
		ok, detail := s.osMechanicalAccept(ctx, t.WorkspacePath, acc)
		return ok, detail, nil
	case plan.AllocatorJudge:
		raw, err := s.synthJudgeCall(ctx, t, acc, do, diff, rework)
		if err != nil {
			return false, "", err
		}
		pass, reason := parseSynthAccept(raw)
		if !pass && reason == "" {
			reason = "cannot parse accept verdict: " + firstLine(raw) // 判读不可解析 → 显式不过(不静默放行)
		}
		return pass, reason, nil
	}
	return false, "", fmt.Errorf("accept phase allocator %q not supported (os|judge)", acc.Allocator)
}

// osMechanicalAccept OS 机械验收(只读/确定性允许清单;D3/D4 — 不执行任意项目代码):
//  1. 期望产出文件存在(os.Stat,不读内容);
//  2. 净残留对账:git 工作树 porcelain − 允许项(仅本相位 output 相对路径)→ 无越界意外新文件;
//  3. git diff --check 空白错误干净。
//
// 全过 → ok + 逐条 evidence。任一不过 → (false, reason)。
func (s *Service) osMechanicalAccept(ctx context.Context, ws string, acc plan.RunPlanPhase) (bool, string) {
	outputs := phaseOutputs(acc)
	for _, rel := range outputs {
		if _, err := os.Stat(filepath.Join(ws, rel)); err != nil {
			return false, "expected output missing: " + rel
		}
	}
	isGit := wsIsGit(ctx, ws)
	if isGit {
		ps, err := gitPorcelainSet(ctx, ws)
		if err == nil && len(ps) > 0 {
			allowed := map[string]bool{}
			for _, rel := range outputs {
				allowed[rel] = true
			}
			var rem []string
			for line := range ps {
				if !allowed[porcelainPath(line)] {
					rem = append(rem, line)
				}
			}
			if len(rem) > 0 {
				sort.Strings(rem)
				return false, "worktree residue beyond declared outputs: " + strings.Join(rem, "; ")
			}
		}
		// 未提交残留空白错误(防御性;do 相位已 commit,正常为空)。git diff --check 有空白错误 → 非零退出,
		// 因此 err != nil 即"查出问题"(gitDirCmd 把 stderr 合并进错误详情)。
		if _, err := gitDirCmd(ctx, ws, "diff", "--check"); err != nil {
			return false, "git diff --check: " + firstLine(err.Error())
		}
		// 已提交相位自身空白检查:本相位 = 最近一次提交(do 已 commit;agent 无权 commit)。
		// 浅历史(仓库首个提交无 HEAD^)→ rev-parse --verify HEAD^ 失败 → 跳过,不误伤。
		if _, err := gitDirCmd(ctx, ws, "rev-parse", "--verify", "HEAD^"); err == nil {
			if _, err := gitDirCmd(ctx, ws, "diff", "--check", "HEAD^", "HEAD"); err != nil {
				return false, "git diff --check (phase commit): " + firstLine(err.Error())
			}
		}
	}
	detail := "os mechanical: outputs present"
	if isGit {
		detail = "os mechanical: outputs present; worktree residue=0; diff --check clean"
	}
	return true, detail
}

// porcelainPath 从 porcelain 行("XY path" / "?? path")抽相对路径;无法识别 → 整行(保守,归残留)。
func porcelainPath(line string) string {
	line = strings.TrimSpace(line)
	if len(line) >= 4 && line[2] == ' ' {
		return strings.TrimSpace(line[3:])
	}
	return line
}

// synthJudgeCall judge accept 判读调用(review 槽 frontier,scripted 走 OS_SCRIPT_ACCEPT seam)。
// rework = 本 accept 前已失败的判读次数(0 起;fail-once 据此首次不过)。
func (s *Service) synthJudgeCall(ctx context.Context, t task.Task, acc, do plan.RunPlanPhase, diff string, rework int) (string, error) {
	if s.engineScripted(ctx, t.CompanyID) {
		return synthAcceptScripted(rework), nil
	}
	epID := s.engEndpointFor(t, engRoleReview)
	if epID == "" {
		return "", fmt.Errorf("synthesized accept phase seq=%d requires a reviewer endpoint", acc.Seq)
	}
	e, err := s.store.GetEndpoint(ctx, epID)
	if err != nil {
		return "", fmt.Errorf("synthesized accept reviewer endpoint: %w", err)
	}
	if e.Status != "active" {
		return "", fmt.Errorf("synthesized accept reviewer endpoint %q status=%s", e.Name, e.Status)
	}
	return s.modelCall(ctx, e, synthAcceptPrompt(t, acc, do, diff))
}

// synthAcceptPrompt judge accept 指令(判读 do 相位产出是否过 acceptance 判据)。
func synthAcceptPrompt(t task.Task, acc, do plan.RunPlanPhase, diff string) string {
	return fmt.Sprintf("You are the acceptance judge for one phase of a synthesized plan.\n"+
		"Task: %q\nDo phase: %s\nAcceptance criteria: %s\n"+
		"Output artifact(s): %s\n"+
		"Do phase diff:\n%s\n"+
		"Judge whether the do output satisfies the acceptance criteria.\n"+
		"Reply with EXACTLY ONE JSON object, no prose, no code fence:\n"+
		"  {\"pass\":true,\"reason\":\"<what you verified>\"} — acceptable\n"+
		"  {\"pass\":false,\"reason\":\"<precise defect the do phase must fix>\"} — not acceptable",
		t.Title, do.Title, phaseAcceptance(acc), strings.Join(phaseOutputs(acc), ", "), diff)
}

// synthAcceptScripted scripted judge accept(OS_SCRIPT_ACCEPT;envSeam 门内,产品不可达):
// ""|pass → 过;fail → 恒不过(测返工上限);fail-once → 首判不过、返工后过(测免费返工放行)。
func synthAcceptScripted(rework int) string {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("OS_SCRIPT_ACCEPT"))) {
	case "fail":
		return `{"pass":false,"reason":"scripted acceptance permanent failure"}`
	case "fail-once":
		if rework == 0 {
			return `{"pass":false,"reason":"scripted acceptance flake (first attempt)"}`
		}
		return `{"pass":true,"reason":"scripted acceptance pass after rework"}`
	default:
		return `{"pass":true,"reason":"scripted accept"}`
	}
}

// parseSynthAccept 提取 accept 判读结果。JSON 主契约 {"pass":bool,"reason":".."};
// 无 pass 键/不可解析 → (false, "")(调用方不静默放行)。
func parseSynthAccept(out string) (pass bool, reason string) {
	var m map[string]json.RawMessage
	if !decodeJudgeJSON(out, &m) {
		return false, ""
	}
	raw, ok := m["pass"]
	if !ok {
		return false, ""
	}
	if err := json.Unmarshal(raw, &pass); err != nil {
		return false, ""
	}
	if r, ok := m["reason"]; ok {
		_ = json.Unmarshal(r, &reason)
	}
	return pass, strings.TrimSpace(reason)
}

// synthDisposeJudge dispose 处置判读(judge):判读当前工作树/产物状态是否仍需收尾动作。
// scripted → 确定性 clean(处置不读 seam,恒净);live → review 槽模型判读,喂 acceptance 处置意图。
// 返回 (clean, reason, internalErr);判读不可解析 → (false, "...", nil) 不静默放行(记 note 留人)。
func (s *Service) synthDisposeJudge(ctx context.Context, t task.Task, ph plan.RunPlanPhase) (bool, string, error) {
	if s.engineScripted(ctx, t.CompanyID) {
		return true, "scripted dispose clean", nil
	}
	epID := s.engEndpointFor(t, engRoleReview)
	if epID == "" {
		return false, "", fmt.Errorf("synthesized dispose phase seq=%d requires a reviewer endpoint", ph.Seq)
	}
	e, err := s.store.GetEndpoint(ctx, epID)
	if err != nil {
		return false, "", fmt.Errorf("synthesized dispose reviewer endpoint: %w", err)
	}
	if e.Status != "active" {
		return false, "", fmt.Errorf("synthesized dispose reviewer endpoint %q status=%s", e.Name, e.Status)
	}
	raw, err := s.modelCall(ctx, e, synthDisposePrompt(t, ph))
	if err != nil {
		return false, "", err
	}
	var m map[string]json.RawMessage
	if !decodeJudgeJSON(raw, &m) {
		return false, "dispose judge: cannot parse verdict — review " + firstLine(raw), nil
	}
	clean := true
	if b, ok := m["clean"]; ok {
		_ = json.Unmarshal(b, &clean)
	}
	reason := "dispose judge: no cleanup advised"
	if r, ok := m["reason"]; ok {
		_ = json.Unmarshal(r, &reason)
	}
	return clean, strings.TrimSpace(reason), nil
}

// synthDisposePrompt dispose 判读指令(判读对象 = 任务工作树收尾状态,非 do diff)。
func synthDisposePrompt(t task.Task, ph plan.RunPlanPhase) string {
	return fmt.Sprintf("You are the disposal judge for one phase of a synthesized plan.\n"+
		"Task: %q\nDispose phase: %s\n"+strings.TrimSpace(phaseAcceptance(ph))+"\n"+
		"The do/accept units of this plan have finished; the OS will leave the workspace as-is.\n"+
		"Decide whether any cleanup action is still warranted (e.g. stray scratch files, credentials, build droppings).\n"+
		"Reply with EXACTLY ONE JSON object, no prose, no code fence:\n"+
		"  {\"clean\":true,\"reason\":\"<nothing needed>\"} — no further action\n"+
		"  {\"clean\":false,\"reason\":\"<precise cleanup a human should do>\"} — leave a disposition note",
		t.Title, ph.Title)
}

// ---- dispose(§3.5 轻收尾;ok 收尾步,不做任意自动动作) ----

// runSynthDispose 处置相位:
//   - os → git 层收尾:工作树无残留即 ok;残留 → dispose fail → 任务 engFail(不吞不扫,留人复核);
//   - judge → 模型判读是否需收尾动作(scripted 恒 clean);需收尾 → note 记录人工跟进(不自动执行);
//   - manual → 记录人工处置项(note;不自动动作)。
func (s *Service) runSynthDispose(ctx, runCtx context.Context, t task.Task, lg *runLedger, ph plan.RunPlanPhase) error {
	switch ph.Status {
	case plan.PhaseStatusOK, plan.PhaseStatusFail, plan.PhaseStatusSkipped:
		return nil // 已处置(requeue/恢复)→ 不重复
	}
	if err := s.lgStart(ctx, lg, ph.Seq); err != nil {
		return err
	}
	acc := phaseAcceptance(ph)
	switch ph.Allocator {
	case plan.AllocatorManual:
		note := strings.TrimSpace(acc)
		if note == "" {
			note = "manual disposition (OS records; no auto action)"
		}
		return s.lgFinish(ctx, lg, ph.Seq, plan.PhaseStatusOK, "manual disposition", note)
	case plan.AllocatorOS:
		if wsIsGit(ctx, t.WorkspacePath) {
			ps, err := gitPorcelainSet(ctx, t.WorkspacePath)
			if err != nil {
				return err
			}
			if len(ps) > 0 {
				keys := make([]string, 0, len(ps))
				for line := range ps {
					keys = append(keys, line)
				}
				sort.Strings(keys)
				_ = s.lgFinish(ctx, lg, ph.Seq, plan.PhaseStatusFail, strings.Join(keys, "; "),
					"os dispose residue — task workspace not clean at end")
				return s.engFailTerminal(ctx, t, "synth dispose", runCtx,
					fmt.Errorf("os dispose: workspace residue: %s", strings.Join(keys, "; ")))
			}
		}
		return s.lgFinish(ctx, lg, ph.Seq, plan.PhaseStatusOK, "os dispose: worktree clean", "")
	case plan.AllocatorJudge:
		clean, reason, err := s.synthDisposeJudge(ctx, t, ph)
		if err != nil {
			_ = s.lgFinish(ctx, lg, ph.Seq, plan.PhaseStatusFail, "", err.Error())
			return s.engFailTerminal(ctx, t, "synth dispose", runCtx, err)
		}
		if clean {
			return s.lgFinish(ctx, lg, ph.Seq, plan.PhaseStatusOK, truncate(reason, 120), "")
		}
		note := "dispose judge: cleanup advised — " + truncate(strings.TrimSpace(reason), 200)
		if acc != "" {
			note = "dispose judge: " + acc + " — " + truncate(strings.TrimSpace(reason), 120)
		}
		return s.lgFinish(ctx, lg, ph.Seq, plan.PhaseStatusOK, "dispose judge: cleanup advised", note)
	}
	return s.engFailTerminal(ctx, t, "synth dispose", runCtx,
		fmt.Errorf("dispose phase allocator %q not supported (os|judge|manual)", ph.Allocator))
}
