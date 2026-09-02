package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/glacierzzz26/one-person-company-os/internal/execution"
	"github.com/glacierzzz26/one-person-company-os/internal/task"
	"github.com/google/uuid"
)

// Engineering Driver(研发执行驱动,Phase 6.2)。
// engineering 家族 Task 走此分支:单次认领内循环 写→测→审;测试失败 = 免费返工
// (同 writer 端点重写,round 不变、conflict 不累计);评审 needs_changes → conflict+1
// 同任务返工;conflict 达熔断阈值 → 转 waiting_approval 交人工;approve 后续跑
// (conflict 归零、round 推进,decision 由 DecideApproval 自动落库)。
// 这是加分支不改语义:0-5 全部 tool 路径行为不变(design §11)。

const (
	engFuseMax       = 3 // reviewer 驳回熔断阈值
	engMaxFreeRework = 3 // 单轮内测试免费返工上限(超出按不可恢复失败处理)
)

// isEngineeringTask 判定 Task 是否命中工程家族(Engineering Driver 分发电)。
func isEngineeringTask(t task.Task) bool {
	return t.ToolName == "engineering"
}

// runEngineering 是 Engineering Driver 入口(worker 领取 engineering task 后调用)。
// 认领即同步跑完全部回合,直至:完成任务 / 熔断转审批(释放租约) / 不可恢复失败。
func (s *Service) runEngineering(ctx context.Context, workerID string, t task.Task) error {
	// 前置审批(risk=high 或 approval policy),与 runClaimed 同语义。
	if need, reason, err := s.needsApproval(ctx, t); err != nil {
		return err
	} else if need {
		return s.requestApproval(ctx, t, reason)
	}

	if _, err := s.store.MarkTaskRunning(ctx, t.ID); err != nil {
		return err
	}
	if _, err := s.audit(ctx, "task", t.ID, "eng_start", taskActor(t), "engineering driver round loop"); err != nil {
		return err
	}

	runCtx, cancel := runContext(ctx, t.TimeoutSec)
	defer cancel()

	round := t.RoundNo
	conflict := t.ConflictCount
	humanOverride := false

	// 熔断后人工 approve 续跑:conflict 已达阈值 → 人工已介入,归零并推进回合重试。
	if conflict >= engFuseMax {
		round++
		conflict = 0
		humanOverride = true
		if _, err := s.store.SetTaskRound(ctx, t.ID, round, conflict); err != nil {
			return err
		}
		if _, err := s.audit(ctx, "task", t.ID, "eng_resume", taskActor(t),
			fmt.Sprintf("human approved after fuse; round=%d conflict reset", round)); err != nil {
			return err
		}
	}

	// 6.4 planner 拆解序曲(熔断续跑 humanOverride=true 不再拆):整包请求先过 planner —
	// direct → 落入下方回合机;split(≤8) → 建子任务同步驱动聚合父任务;
	// ask(>8/边界不清) → 人工审批(无 bypass)。已拆/已批准 → 续跑或按原样执行。
	if !humanOverride {
		handled, perr := s.planEngineering(ctx, runCtx, workerID, t)
		if perr != nil {
			return s.engFail(ctx, t, "plan", runCtx, perr)
		}
		if handled {
			return nil
		}
	}

	for {
		// writer 产出(每轮开头;test 免费返工在本轮内复用同一 writer 端点)
		diff, err := s.runEngPhase(runCtx, workerID, t, engRoleWriter, round, conflict, humanOverride, 0,
			engWriterPrompt(t, round, conflict))
		if err != nil {
			return s.engFail(ctx, t, "writer", runCtx, err)
		}

		// test:失败 → 免费返工(同轮重写,round_no 不变、conflict 不累计)
		testOut, err := s.runEngPhase(runCtx, workerID, t, engRoleTest, round, conflict, humanOverride, 0,
			engTestPrompt(t, round, diff))
		if err != nil {
			return s.engFail(ctx, t, "test", runCtx, err)
		}
		freeRework := 0
		for !parseTestPass(testOut) {
			freeRework++
			if freeRework > engMaxFreeRework {
				return s.engFail(ctx, t, "test", runCtx,
					fmt.Errorf("test failed after %d free reworks: %s", engMaxFreeRework, firstLine(testOut)))
			}
			diff, err = s.runEngPhase(runCtx, workerID, t, engRoleWriter, round, conflict, humanOverride, freeRework,
				engWriterPrompt(t, round, conflict))
			if err != nil {
				return s.engFail(ctx, t, "writer", runCtx, err)
			}
			testOut, err = s.runEngPhase(runCtx, workerID, t, engRoleTest, round, conflict, humanOverride, freeRework,
				engTestPrompt(t, round, diff))
			if err != nil {
				return s.engFail(ctx, t, "test", runCtx, err)
			}
		}

		// review
		reviewOut, err := s.runEngPhase(runCtx, workerID, t, engRoleReview, round, conflict, humanOverride, 0,
			engReviewPrompt(t, round, conflict, humanOverride, diff))
		if err != nil {
			return s.engFail(ctx, t, "review", runCtx, err)
		}
		switch parseReviewVerdict(reviewOut) {
		case "":
			return s.engFail(ctx, t, "review", runCtx,
				fmt.Errorf("cannot parse review verdict from output: %s", firstLine(reviewOut)))
		case "approve":
			if _, err := s.store.CompleteTask(ctx, t.ID, diff); err != nil {
				return err
			}
			if _, err := s.audit(ctx, "task", t.ID, "eng_complete", taskActor(t),
				fmt.Sprintf("round=%d conflict=%d", round, conflict)); err != nil {
				return err
			}
			return nil
		}

		// needs_changes → conflict+1
		conflict++
		if _, err := s.store.SetTaskRound(ctx, t.ID, round, conflict); err != nil {
			return err
		}
		if conflict >= engFuseMax {
			// 熔断 → waiting_approval(人工决定;decision 由 DecideApproval 自动落库)。
			if _, err := s.audit(ctx, "task", t.ID, "eng_fuse", taskActor(t),
				fmt.Sprintf("reviewer rejected %d times; fuse at round=%d", conflict, round)); err != nil {
				return err
			}
			return s.requestApproval(ctx, t, fmt.Sprintf("engineering fuse: reviewer rejected %d times", conflict))
		}
		// 同任务返工:round 推进,下轮从新 writer 开始。
		round++
		humanOverride = false
		if _, err := s.store.SetTaskRound(ctx, t.ID, round, conflict); err != nil {
			return err
		}
		if _, err := s.audit(ctx, "task", t.ID, "eng_rework", taskActor(t),
			fmt.Sprintf("conflict=%d rework; round=%d", conflict, round)); err != nil {
			return err
		}
	}
}

// runEngPhase 跑一个工程阶段:创建一条 execution(结果=该阶段模型输出),成功/失败均落库。
func (s *Service) runEngPhase(ctx context.Context, workerID string, t task.Task, role string,
	round, conflict int64, humanOverride bool, retry int, prompt string) (string, error) {
	execID := uuid.NewString()
	now := time.Now().Unix()
	if _, err := s.store.CreateExecution(ctx, execution.Execution{
		ID: execID, TaskID: t.ID, WorkerID: workerID, Attempt: t.Attempt,
		Status: "running", StartedAt: now, CreatedAt: now,
	}); err != nil {
		return "", err
	}
	out, err := s.engCall(ctx, t, engCallCtx{
		role: role, round: round, conflict: conflict,
		humanOverride: humanOverride, retry: retry, prompt: prompt,
	})
	finishAt := time.Now().Unix()
	if err != nil {
		if _, ferr := s.store.FinishExecution(ctx, execID, "failed", &finishAt, "",
			fmt.Sprintf("eng_%s round=%d: %v", role, round, err)); ferr != nil {
			return "", ferr
		}
		return "", err
	}
	if _, err := s.store.FinishExecution(ctx, execID, "completed", &finishAt,
		fmt.Sprintf("eng_%s round=%d conflict=%d", role, round, conflict)+"\n"+out, ""); err != nil {
		return "", err
	}
	return out, nil
}

// engFail 处理不可恢复的工程执行失败:按 attempt/max_attempts 语义 requeue 或 fail。
func (s *Service) engFail(ctx context.Context, t task.Task, phase string, runCtx context.Context, cause error) error {
	msg := phase + ": " + cause.Error()
	if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		msg = phase + ": timeout"
	}
	if t.Attempt < t.MaxAttempts-1 {
		_, err := s.store.RequeueTask(ctx, t.ID, msg)
		return err
	}
	_, err := s.store.FailTask(ctx, t.ID, msg)
	return err
}

// ---- prompt 构造 ----

// 注意:prompt 内需含 ```diff``` 围栏提示,故用双引号拼接而非 raw string。
func engWriterPrompt(t task.Task, round, conflict int64) string {
	return fmt.Sprintf("You are the coding engineer (writer) for task %q.\n"+
		"Description: %s\n"+
		"Context: round=%d reviewer_conflicts=%d\n"+
		"Produce ONLY a single fenced diff block (```diff ... ```) fixing the described problem.\n"+
		"No prose, no explanations outside the fence.",
		t.Title, strings.TrimSpace(t.Description), round, conflict)
}

func engTestPrompt(t task.Task, round int64, diff string) string {
	return fmt.Sprintf("You are QA. A writer produced this diff for task %q (round=%d).\n"+
		"Diff:\n%s\n"+
		"Judge whether the change is acceptable and would pass tests.\n"+
		"Reply on a single line: \"TEST OK\" or \"TEST FAIL:<reason>\".",
		t.Title, round, diff)
}

func engReviewPrompt(t task.Task, round, conflict int64, humanOverride bool, diff string) string {
	note := ""
	if round > 0 || conflict > 0 {
		note = fmt.Sprintf("\nHistory: round=%d, reviewer disagreements so far=%d.", round, conflict)
	}
	if humanOverride {
		note += "\nA human approved after the fuse; if the diff now resolves the concerns, approve."
	}
	return fmt.Sprintf("You are a senior reviewer. A writer produced this diff for task %q.%s\n"+
		"Diff:\n%s\n"+
		"Reply on a single line: \"VERDICT: approve\" or \"VERDICT: needs_changes:<reason>\".",
		t.Title, note, diff)
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
