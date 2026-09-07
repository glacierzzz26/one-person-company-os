package service

// Phase 9.3 — 通知按公司 / 摘要按公司 fan-out(契约 runtime-knobs-web.md §五 B1-B2)。
// 通知源 = 公司机密 feishu_webhook(file:// 离线邮箱 test-double),不再读 env(决策②)。
// secrets 读走主密钥 holder:注入后 t.Cleanup 复位。

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/glacierzzz26/one-person-company-os/internal/approval"
	"github.com/glacierzzz26/one-person-company-os/internal/settings"
	"github.com/glacierzzz26/one-person-company-os/internal/storage/repository"
	"github.com/glacierzzz26/one-person-company-os/internal/task"
	"github.com/google/uuid"
)

// seedMailbox 建一个离线通知邮箱(不存在;写入了才有内容)。
func seedMailbox(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "mailbox.txt")
}

// seedDigestTask 落一条 digest 统计用的任务(created/updated = now → 必落在 24h 窗口)。
func seedDigestTask(t *testing.T, st *repository.Store, compID, status string) task.Task {
	t.Helper()
	now := time.Now().Unix()
	tk, err := st.CreateTask(context.Background(), task.Task{
		ID: uuid.NewString(), CompanyID: compID, Title: "digest-seeded task",
		ToolName: "shell", Status: status, Risk: "low", QStatus: status,
		CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("seed digest task: %v", err)
	}
	return tk
}

// seedDecidedApproval 落一条已决审批(CreateApproval 只插 pending,状态/decided_at 走 UpdateApproval)。
func seedDecidedApproval(t *testing.T, st *repository.Store, taskID, risk string, decidedAt int64) {
	t.Helper()
	a, err := st.CreateApproval(context.Background(), approval.Approval{
		ID: uuid.NewString(), TaskID: taskID, Risk: risk, RequestedBy: "intake", CreatedAt: time.Now().Unix(),
	})
	if err != nil {
		t.Fatalf("seed approval: %v", err)
	}
	if _, err := st.UpdateApproval(context.Background(), a.ID, "approved", "human:console", "", &decidedAt); err != nil {
		t.Fatalf("approve seeded approval: %v", err)
	}
}

// B1 审批即时通知按公司路由:task 归属公司配 feishu_webhook(file:// 邮箱)→ 邮箱出现告警文本;
// 未配公司 → 静默(无文件、无错误);task 无公司 → 静默。
func TestB1ApprovalNotifyPerCompany(t *testing.T) {
	svc, st := newSvc(t)
	ctx := context.Background()
	settings.UseMasterKey(key32())
	t.Cleanup(func() { settings.UseMasterKey(nil) })

	compSet := seedCompanyID(t, st)
	compNone := seedCompanyID(t, st)
	mailbox := seedMailbox(t)
	if err := svc.SetSecretCurrent(ctx, compSet, SecretFeishuWebhook, "file://"+mailbox); err != nil {
		t.Fatalf("SetSecretCurrent feishu_webhook: %v", err)
	}

	// 配了 webhook 的公司:熔断审批 → 邮箱出现告警。
	tk := task.Task{ID: uuid.NewString(), CompanyID: compSet, Title: "flaky auth refactor", Risk: "high", RoundNo: 2, ConflictCount: 3}
	svc.notifyApproval(ctx, tk, "appr1234", "engineering fuse")
	content, err := os.ReadFile(mailbox)
	if err != nil {
		t.Fatalf("read mailbox (want notify written): %v", err)
	}
	text := string(content)
	for _, want := range []string{"【研发熔断】", "flaky auth refactor", "round=2 conflict=3", "os approval list → os approval approve|reject|changes appr1234"} {
		if !strings.Contains(text, want) {
			t.Errorf("mailbox missing %q:\n%s", want, text)
		}
	}

	// 未配 webhook 的公司 → 静默(notifyCompany 返回 nil,不落文件不报错)。
	mailboxNone := seedMailbox(t)
	tkNone := task.Task{ID: uuid.NewString(), CompanyID: compNone, Title: "no sink", Risk: "medium"}
	svc.notifyApproval(ctx, tkNone, "appr-none", "planner ask")
	if _, err := os.Stat(mailboxNone); !os.IsNotExist(err) {
		t.Fatalf("unconfigured company should write nothing (mailbox exists?)")
	}

	// 非熔断等待审批(ask)→ 文本不带熔断头。
	tkAsk := task.Task{ID: uuid.NewString(), CompanyID: compSet, Title: "approval ask", Risk: "high"}
	svc.notifyApproval(ctx, tkAsk, "appr-ask1", "planner ask")
	content, err = os.ReadFile(mailbox)
	if err != nil {
		t.Fatalf("read mailbox after ask: %v", err)
	}
	if !strings.Contains(string(content), "【待审批】") {
		t.Errorf("ask notify should append a 待审批-header message:\n%s", content)
	}

	// task 无公司(CompanyID="")→ 静默,不触碰任何通道。
	svc.notifyApproval(ctx, task.Task{ID: uuid.NewString(), Title: "no company"}, "appr-orphan", "engineering fuse")
}

// B2 SendDailyDigest 按公司 fan-out:公司 A/B 各配 feishu(独立 file:// 邮箱),A 报告只含 A 数据
// (任务 2 完成 + 1 通过审批),B 只含 B(1 完成,0 通过——A 的审批不进 B);公司 C 未配 → 跳过;
// 无公司配 webhook 的公司一律静默,函数返回 nil。
func TestB2DigestFanOutPerCompany(t *testing.T) {
	svc, st := newSvc(t)
	ctx := context.Background()
	settings.UseMasterKey(key32())
	t.Cleanup(func() { settings.UseMasterKey(nil) })

	compA := seedCompanyID(t, st)
	compB := seedCompanyID(t, st)
	_ = seedCompanyID(t, st) // compC:存在但未配 feishu_webhook → fan-out 跳过
	mailA, mailB, mailC := seedMailbox(t), seedMailbox(t), seedMailbox(t)
	for _, s := range []struct{ comp, mbox string }{
		{compA, mailA}, {compB, mailB},
	} {
		if err := svc.SetSecretCurrent(ctx, s.comp, SecretFeishuWebhook, "file://"+s.mbox); err != nil {
			t.Fatalf("SetSecretCurrent feishu_webhook %s: %v", short8(s.comp), err)
		}
	}

	// A:2 条完成;其中 1 条审批通过(decided now,窗口内)。
	now := time.Now().Unix()
	taskA := seedDigestTask(t, st, compA, "completed")
	seedDigestTask(t, st, compA, "completed")
	seedDecidedApproval(t, st, taskA.ID, "high", now)
	// B:1 条完成;其审批 decided 在窗口外(48h 前)→ 不计。
	taskB := seedDigestTask(t, st, compB, "completed")
	seedDecidedApproval(t, st, taskB.ID, "medium", now-48*3600)
	// C:不配 webhook → 跳过。

	if err := svc.SendDailyDigest(ctx); err != nil {
		t.Fatalf("SendDailyDigest: %v", err)
	}

	read := func(p string) string {
		t.Helper()
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("read mailbox %s: %v", p, err)
		}
		return string(b)
	}
	reportA := read(mailA)
	reportB := read(mailB)
	for _, want := range []string{"完成 2 |", "通过 1 / 驳回 0 / 打回 0", "os overview --company " + compA} {
		if !strings.Contains(reportA, want) {
			t.Errorf("report A missing %q:\n%s", want, reportA)
		}
	}
	if strings.Contains(reportA, compB) {
		t.Errorf("report A leaked company B id:\n%s", reportA)
	}
	for _, want := range []string{"完成 1 |", "通过 0 / 驳回 0 / 打回 0", "os overview --company " + compB} {
		if !strings.Contains(reportB, want) {
			t.Errorf("report B missing %q:\n%s", want, reportB)
		}
	}
	if strings.Contains(reportB, compA) {
		t.Errorf("report B leaked company A id:\n%s", reportB)
	}
	// C 未配 webhook → 邮箱不存在。
	if _, err := os.Stat(mailC); !os.IsNotExist(err) {
		t.Fatal("company C (no feishu_webhook) should not receive a digest mailbox")
	}
}
